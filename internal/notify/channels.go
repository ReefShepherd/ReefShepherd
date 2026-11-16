package notify

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/db/dbgen"
	"github.com/reefshepherd/reefshepherd/internal/httpx"
	"github.com/reefshepherd/reefshepherd/internal/i18n"
	"github.com/reefshepherd/reefshepherd/internal/version"
)

const (
	// maskedSecret is what the API shows in place of a secret config value, and what a client sends back to keep the stored one.
	maskedSecret   = "********"
	maxNameRunes   = 100
	testLinkPath   = "/settings/notifications"
	testEventType  = "test"
	channelSummary = "invalid notification channel"
)

// Channel is a notification channel as the API shows it, with its secrets masked.
type Channel struct {
	ID          int64           `json:"id"`
	Name        string          `json:"name"`
	Kind        string          `json:"kind"`
	Config      json.RawMessage `json:"config"`
	Events      []string        `json:"events"`
	Enabled     bool            `json:"enabled"`
	LastSentAt  *string         `json:"last_sent_at"`
	LastError   string          `json:"last_error"`
	LastErrorAt *string         `json:"last_error_at"`
	CreatedAt   string          `json:"created_at"`
	UpdatedAt   string          `json:"updated_at"`
}

// CreateChannelInput is a new channel; nil Events subscribes it to every event type and nil Enabled switches it on.
type CreateChannelInput struct {
	Name    string
	Kind    string
	Config  json.RawMessage
	Events  []string
	Enabled *bool
}

// UpdateChannelInput changes the fields that are set; a config replaces the stored one except for secrets sent as the mask.
type UpdateChannelInput struct {
	Name    *string
	Config  json.RawMessage
	Events  *[]string
	Enabled *bool
}

// TestResult is the outcome of a test send, which is a result and not a failed request.
type TestResult struct {
	Sent  bool   `json:"sent"`
	Error string `json:"error"`
}

// env is what providers get besides their own configuration.
func (s *Service) env() Env {
	return Env{HTTP: s.HTTP, UserAgent: "ReefShepherd/" + version.Version()}
}

// ListChannels returns every channel with its subscriptions and delivery health, oldest first.
func (s *Service) ListChannels(ctx context.Context) ([]Channel, error) {
	var out []Channel
	err := s.DB.InReadTx(ctx, func(tx *sql.Tx) error {
		q := db.Q(tx)
		rows, err := q.ListChannels(ctx)
		if err != nil {
			return fmt.Errorf("list channels: %w", err)
		}
		subs, err := q.ListSubscriptions(ctx)
		if err != nil {
			return fmt.Errorf("list subscriptions: %w", err)
		}
		out = make([]Channel, len(rows))
		for i, row := range rows {
			if out[i], err = channelView(ctx, q, row, subs); err != nil {
				return err
			}
		}
		return nil
	})
	return out, err
}

// GetChannel returns one channel.
func (s *Service) GetChannel(ctx context.Context, id int64) (Channel, error) {
	var out Channel
	err := s.DB.InReadTx(ctx, func(tx *sql.Tx) error {
		q := db.Q(tx)
		row, err := q.GetChannel(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrChannelNotFound
		}
		if err != nil {
			return fmt.Errorf("read channel: %w", err)
		}
		subs, err := q.ListSubscriptions(ctx)
		if err != nil {
			return fmt.Errorf("list subscriptions: %w", err)
		}
		out, err = channelView(ctx, q, row, subs)
		return err
	})
	return out, err
}

// channelView converts a stored channel to its API form: secrets masked, its events picked from subs, and its delivery health.
func channelView(ctx context.Context, q *dbgen.Queries, row dbgen.NotificationChannel, subs []dbgen.NotificationSubscription) (Channel, error) {
	config, err := maskConfig(row.Kind, row.ConfigJson)
	if err != nil {
		return Channel{}, fmt.Errorf("channel %d: %w", row.ID, err)
	}
	c := Channel{
		ID: row.ID, Name: row.Name, Kind: row.Kind, Config: config, Events: []string{}, Enabled: row.Enabled == 1,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
	for _, sub := range subs {
		if sub.ChannelID == row.ID {
			c.Events = append(c.Events, sub.EventType)
		}
	}
	slices.SortFunc(c.Events, func(a, b string) int { return slices.Index(eventTypes, a) - slices.Index(eventTypes, b) })

	sentAt, err := q.LastSentAt(ctx, row.ID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Channel{}, fmt.Errorf("read last send: %w", err)
	}
	c.LastSentAt = sentAt
	// The channel shows an error only while its latest attempted delivery has not got through.
	last, err := q.LastAttemptedDelivery(ctx, row.ID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Channel{}, fmt.Errorf("read last delivery: %w", err)
	}
	if err == nil && last.Status != "sent" && last.LastError != "" {
		c.LastError, c.LastErrorAt = last.LastError, last.LastAttemptAt
	}
	return c, nil
}

// CreateChannel adds a channel and its subscriptions after the kind has validated the config.
func (s *Service) CreateChannel(ctx context.Context, in CreateChannelInput) (Channel, error) {
	f := httpx.FieldErrors{Message: channelSummary}
	name := cleanName(in.Name, &f)
	kind, known := cleanKind(in.Kind, &f)
	events := cleanEvents(in.Events, in.Events == nil, &f)
	var config []byte
	if known {
		config = s.cleanConfig(kind, in.Config, &f)
	}
	if err := f.Err(); err != nil {
		return Channel{}, err
	}

	now := db.FormatTime(s.now())
	var id int64
	err := s.DB.InTx(ctx, func(tx *sql.Tx) error {
		q := db.Q(tx)
		row, err := q.CreateChannel(ctx, dbgen.CreateChannelParams{
			Name: name, Kind: kind.Name, ConfigJson: string(config), Enabled: db.Flag(in.Enabled == nil || *in.Enabled), CreatedAt: now, UpdatedAt: now,
		})
		if err != nil {
			return fmt.Errorf("create channel: %w", err)
		}
		id = row.ID
		return subscribe(ctx, q, id, events)
	})
	if err != nil {
		return Channel{}, err
	}
	return s.GetChannel(ctx, id)
}

// UpdateChannel changes a channel; the kind cannot change.
func (s *Service) UpdateChannel(ctx context.Context, id int64, in UpdateChannelInput) (Channel, error) {
	err := s.DB.InTx(ctx, func(tx *sql.Tx) error {
		q := db.Q(tx)
		row, err := q.GetChannel(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrChannelNotFound
		}
		if err != nil {
			return fmt.Errorf("read channel: %w", err)
		}

		f := httpx.FieldErrors{Message: channelSummary}
		params := dbgen.UpdateChannelParams{Name: row.Name, ConfigJson: row.ConfigJson, Enabled: row.Enabled, UpdatedAt: db.FormatTime(s.now()), ID: id}
		if in.Name != nil {
			params.Name = cleanName(*in.Name, &f)
		}
		if in.Enabled != nil {
			params.Enabled = db.Flag(*in.Enabled)
		}
		var events []string
		if in.Events != nil {
			events = cleanEvents(*in.Events, false, &f)
		}
		if in.Config != nil {
			kind, ok := lookup(row.Kind)
			if !ok {
				f.Add("config", httpx.CodeInvalid)
			} else if merged, err := keepMaskedSecrets(kind, row.ConfigJson, in.Config); err != nil {
				f.Add("config", httpx.CodeInvalid)
			} else if config := s.cleanConfig(kind, merged, &f); config != nil {
				params.ConfigJson = string(config)
			}
		}
		if err := f.Err(); err != nil {
			return err
		}

		if _, err := q.UpdateChannel(ctx, params); err != nil {
			return fmt.Errorf("update channel: %w", err)
		}
		if in.Events != nil {
			if err := q.ClearSubscriptions(ctx, id); err != nil {
				return fmt.Errorf("clear subscriptions: %w", err)
			}
			return subscribe(ctx, q, id, events)
		}
		return nil
	})
	if err != nil {
		return Channel{}, err
	}
	return s.GetChannel(ctx, id)
}

// DeleteChannel removes a channel with its subscriptions and deliveries.
func (s *Service) DeleteChannel(ctx context.Context, id int64) error {
	n, err := db.Q(s.DB.W).DeleteChannel(ctx, id)
	if err != nil {
		return fmt.Errorf("delete channel: %w", err)
	}
	if n == 0 {
		return ErrChannelNotFound
	}
	return nil
}

// TestChannel sends a test message through the stored channel, enabled or not, and reports how it went.
func (s *Service) TestChannel(ctx context.Context, id int64) (TestResult, error) {
	row, err := db.Q(s.DB.R).GetChannel(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return TestResult{}, ErrChannelNotFound
	}
	if err != nil {
		return TestResult{}, fmt.Errorf("read channel: %w", err)
	}
	instance, err := s.Settings.GetInstanceName(ctx)
	if err != nil {
		return TestResult{}, fmt.Errorf("read instance name: %w", err)
	}
	catalog := i18n.Default()
	msg := Message{
		EventType: testEventType, Severity: "info",
		Title:      catalog.T(i18n.DefaultLocale, "notify_test_title", instance),
		Body:       catalog.T(i18n.DefaultLocale, "notify_test_body"),
		Link:       s.absoluteLink(testLinkPath),
		OccurredAt: s.now(), Instance: instance,
	}
	if _, err := s.sendTo(ctx, row.Kind, row.ConfigJson, msg); err != nil {
		return TestResult{Error: truncate(err.Error(), maxLastError)}, nil
	}
	return TestResult{Sent: true}, nil
}

func subscribe(ctx context.Context, q *dbgen.Queries, channelID int64, events []string) error {
	for _, e := range events {
		if err := q.AddSubscription(ctx, dbgen.AddSubscriptionParams{ChannelID: channelID, EventType: e}); err != nil {
			return fmt.Errorf("subscribe to %s: %w", e, err)
		}
	}
	return nil
}

func cleanName(name string, f *httpx.FieldErrors) string {
	name = strings.TrimSpace(name)
	switch {
	case name == "":
		f.Add("name", httpx.CodeRequired)
	case utf8.RuneCountInString(name) > maxNameRunes:
		f.Add("name", httpx.CodeTooLong)
	}
	return name
}

func cleanKind(name string, f *httpx.FieldErrors) (Kind, bool) {
	if name == "" {
		f.Add("kind", httpx.CodeRequired)
		return Kind{}, false
	}
	kind, ok := lookup(name)
	if !ok {
		f.Add("kind", httpx.CodeInvalid)
	}
	return kind, ok
}

// cleanEvents checks the subscribed event types and returns them without repeats in the listing order; all subscribes to every type.
func cleanEvents(events []string, all bool, f *httpx.FieldErrors) []string {
	if all {
		return slices.Clone(eventTypes)
	}
	if len(events) == 0 {
		f.Add("events", httpx.CodeRequired)
		return nil
	}
	for _, e := range events {
		if !slices.Contains(eventTypes, e) {
			f.Add("events", httpx.CodeInvalid)
			return nil
		}
	}
	return slices.DeleteFunc(slices.Clone(eventTypes), func(e string) bool { return !slices.Contains(events, e) })
}

// cleanConfig has the kind validate raw and returns it in compact form, or nil after recording why it cannot be used.
func (s *Service) cleanConfig(kind Kind, raw json.RawMessage, f *httpx.FieldErrors) []byte {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		f.Add("config", httpx.CodeRequired)
		return nil
	}
	if trimmed[0] != '{' {
		f.Add("config", httpx.CodeInvalid)
		return nil
	}
	if _, err := kind.Build(trimmed, s.env()); err != nil {
		if !f.AddFrom(err) {
			f.Add("config", httpx.CodeInvalid)
		}
		return nil
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, trimmed); err != nil {
		f.Add("config", httpx.CodeInvalid)
		return nil
	}
	return compact.Bytes()
}

// isSet reports whether a config value holds anything, which is what decides that it is shown masked.
func isSet(v json.RawMessage) bool {
	s := string(bytes.TrimSpace(v))
	return s != "" && s != "null" && s != `""`
}

// secretFields returns the config fields of a kind that are masked; a kind this build does not know masks every field.
func secretFields(kindName string, fields map[string]json.RawMessage) []string {
	if kind, ok := lookup(kindName); ok {
		return kind.Secrets
	}
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	return names
}

// maskConfig is the stored config with every non-empty secret field replaced by the mask.
func maskConfig(kindName, stored string) (json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(stored), &fields); err != nil {
		return nil, fmt.Errorf("parse stored config: %w", err)
	}
	for _, name := range secretFields(kindName, fields) {
		if isSet(fields[name]) {
			fields[name] = json.RawMessage(`"` + maskedSecret + `"`)
		}
	}
	masked, err := json.Marshal(fields)
	if err != nil {
		return nil, fmt.Errorf("encode config: %w", err)
	}
	return masked, nil
}

// keepMaskedSecrets returns the config a client sent with each secret it sent as the mask replaced by the stored value, or dropped when none is stored.
func keepMaskedSecrets(kind Kind, stored string, sent json.RawMessage) (json.RawMessage, error) {
	var current, incoming map[string]json.RawMessage
	if err := json.Unmarshal([]byte(stored), &current); err != nil {
		return nil, fmt.Errorf("parse stored config: %w", err)
	}
	if err := json.Unmarshal(sent, &incoming); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	for _, name := range kind.Secrets {
		var value string
		if json.Unmarshal(incoming[name], &value) != nil || value != maskedSecret {
			continue
		}
		if isSet(current[name]) {
			incoming[name] = current[name]
		} else {
			delete(incoming, name)
		}
	}
	return json.Marshal(incoming)
}

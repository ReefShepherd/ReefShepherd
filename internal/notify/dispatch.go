package notify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/db/dbgen"
	"github.com/reefshepherd/reefshepherd/internal/version"
)

const (
	// dispatchBatch is how many due deliveries one tick sends.
	dispatchBatch = 50
	// dispatchTimeout covers a full batch of slow channels.
	dispatchTimeout = 5 * time.Minute
	// maxAttempts is the first send plus the retries; the delivery fails for good after the last one.
	maxAttempts = 4
	// maxLastError is how many characters of a send error a delivery keeps.
	maxLastError = 500
	// maxRetryAfter caps the delay a provider can ask for.
	maxRetryAfter = 24 * time.Hour
)

// retryDelays is the wait after each failed attempt before the next one.
var retryDelays = [maxAttempts - 1]time.Duration{time.Minute, 10 * time.Minute, time.Hour}

// RetryAfterError is implemented by the error of a send that was throttled, so a retry waits at least as long as the provider asked.
type RetryAfterError interface {
	error
	RetryAfter() time.Duration
}

// GoneError is implemented by the error of a send to a destination that no longer exists, so the delivery fails at once instead of being retried.
type GoneError interface {
	error
	Gone() bool
}

// Dispatch sends the pending deliveries that are due at now, up to one batch, and records how each one went at the time it went.
func (s *Service) Dispatch(ctx context.Context, now time.Time) error {
	due := db.FormatTime(now)
	rows, err := db.Q(s.DB.R).ListDueDeliveries(ctx, dbgen.ListDueDeliveriesParams{Now: &due, RowLimit: dispatchBatch})
	if err != nil {
		return fmt.Errorf("list due deliveries: %w", err)
	}
	if len(rows) == 0 {
		return nil
	}
	instance, err := s.Settings.GetInstanceName(ctx)
	if err != nil {
		return fmt.Errorf("read instance name: %w", err)
	}
	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := s.deliver(ctx, instance, row); err != nil {
			return err
		}
	}
	return nil
}

// deliver makes one attempt and writes its outcome; it returns an error only when the outcome could not be recorded or the attempt was cut short.
func (s *Service) deliver(ctx context.Context, instance string, row dbgen.ListDueDeliveriesRow) error {
	var (
		permanent bool
		sendErr   error
	)
	if row.Enabled == 0 {
		permanent, sendErr = true, errors.New("channel is disabled")
	} else {
		permanent, sendErr = s.sendTo(ctx, row.Kind, row.ConfigJson, s.message(row, instance))
	}
	// A cancelled attempt says nothing about the channel, so the delivery stays pending and is tried again.
	if sendErr != nil && ctx.Err() != nil {
		return ctx.Err()
	}

	// The write must complete even when the job's time is nearly up.
	write := db.Q(s.DB.W)
	wctx := context.WithoutCancel(ctx)
	attempts := row.Attempts + 1
	// A slow batch must not date its later attempts, or base their retries, on the tick that started it.
	attemptTime := s.now()
	attemptedAt := db.FormatTime(attemptTime)
	if sendErr == nil {
		if err := write.MarkDeliverySent(wctx, dbgen.MarkDeliverySentParams{Attempts: attempts, AttemptedAt: &attemptedAt, ID: row.DeliveryID}); err != nil {
			return fmt.Errorf("record delivery %d: %w", row.DeliveryID, err)
		}
		return nil
	}

	failure := dbgen.RecordDeliveryFailureParams{Status: "failed", Attempts: attempts, LastError: truncate(sendErr.Error(), maxLastError), LastAttemptAt: &attemptedAt, ID: row.DeliveryID}
	if !permanent && attempts < maxAttempts {
		failure.Status = "pending"
		next := db.FormatTime(attemptTime.Add(retryDelay(attempts, sendErr)))
		failure.NextAttemptAt = &next
	}
	s.Logger.LogAttrs(ctx, slog.LevelWarn, "notification delivery failed",
		slog.Int64("delivery", row.DeliveryID), slog.Int64("channel", row.ChannelID), slog.String("kind", row.Kind),
		slog.Int64("attempts", attempts), slog.String("status", failure.Status), slog.Any("error", sendErr))
	if err := write.RecordDeliveryFailure(wctx, failure); err != nil {
		return fmt.Errorf("record delivery %d: %w", row.DeliveryID, err)
	}
	return nil
}

// retryDelay is how long to wait after the given number of failed attempts: the backoff schedule, or longer when the provider asked for more.
func retryDelay(attempts int64, err error) time.Duration {
	delay := retryDelays[attempts-1]
	var throttled RetryAfterError
	if errors.As(err, &throttled) {
		delay = max(delay, min(throttled.RetryAfter(), maxRetryAfter))
	}
	return delay
}

// message builds what a channel receives for a due delivery.
func (s *Service) message(row dbgen.ListDueDeliveriesRow, instance string) Message {
	msg := Message{
		EventType: row.EventType, Severity: row.Severity, Title: row.Title, Body: row.Body,
		Link: s.absoluteLink(row.Link), Instance: instance,
	}
	if row.TankID != nil {
		msg.TankID = *row.TankID
	}
	if row.TankName != nil {
		msg.TankName = *row.TankName
	}
	// A stored timestamp that cannot be read leaves the zero time rather than holding back the message.
	msg.OccurredAt, _ = db.ParseTime(row.OccurredAt)
	return msg
}

// absoluteLink turns an in-app path into a URL on the public address, or "" when there is no path or no public address.
func (s *Service) absoluteLink(path string) string {
	if path == "" || s.PublicURL == "" {
		return ""
	}
	return strings.TrimRight(s.PublicURL, "/") + path
}

// sendTo builds the channel's sender from its stored config and sends msg with a time limit; permanent reports an error that retrying cannot fix.
func (s *Service) sendTo(ctx context.Context, kindName, config string, msg Message) (permanent bool, err error) {
	kind, ok := lookup(kindName)
	if !ok {
		return true, fmt.Errorf("unknown channel kind %q", kindName)
	}
	permanent, err = s.send(ctx, kind, config, msg)
	if err != nil {
		err = redactSecrets(err, secretValues(kind, config))
	}
	return permanent, err
}

// send builds the sender and sends msg, turning a panic into an error.
func (s *Service) send(ctx context.Context, kind Kind, config string, msg Message) (permanent bool, err error) {
	defer func() {
		if r := recover(); r != nil {
			permanent, err = false, fmt.Errorf("channel panicked: %v", r)
		}
	}()
	notifier, err := kind.Build(json.RawMessage(config), Env{HTTP: s.HTTP, UserAgent: "ReefShepherd/" + version.Version()})
	if err != nil {
		return true, fmt.Errorf("invalid channel configuration: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()
	err = notifier.Send(ctx, msg)
	var gone GoneError
	return errors.As(err, &gone) && gone.Gone(), err
}

// truncate keeps the first n characters of s.
func truncate(s string, n int) string {
	s = strings.ToValidUTF8(s, "�")
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

// minFragment is the shortest URL path segment or query value redacted on its own, so common words such as "api" survive.
const minFragment = 6

// redactedError is a send error whose text hides the channel's secrets; the original stays in the chain so RetryAfter and Gone still work.
type redactedError struct {
	text string
	err  error
}

func (e *redactedError) Error() string { return e.text }
func (e *redactedError) Unwrap() error { return e.err }

// secretValues returns the values of the config fields the kind masks.
func secretValues(kind Kind, config string) []string {
	var fields map[string]json.RawMessage
	if json.Unmarshal([]byte(config), &fields) != nil {
		return nil
	}
	var out []string
	for _, name := range kind.Secrets {
		var v string
		if json.Unmarshal(fields[name], &v) == nil && v != "" {
			out = append(out, v)
		}
	}
	return out
}

// redactionTerms lists every text that must not leave the server: each secret, and for a secret that is a URL also its path, query, password and the long parts of those, each as is and escaped, longest first.
func redactionTerms(secrets []string) []string {
	var parts []string
	for _, v := range secrets {
		parts = append(parts, v)
		u, err := url.Parse(v)
		if err != nil || u.Scheme == "" || u.Host == "" {
			continue
		}
		for _, p := range []string{u.Path, u.EscapedPath()} {
			if len(p) > 1 {
				parts = append(parts, p)
			}
			parts = append(parts, longParts(strings.Split(p, "/"))...)
		}
		if u.RawQuery != "" {
			parts = append(parts, u.RawQuery)
		}
		for _, pair := range strings.Split(u.RawQuery, "&") {
			_, value, _ := strings.Cut(pair, "=")
			parts = append(parts, longParts([]string{value})...)
		}
		for _, values := range u.Query() {
			parts = append(parts, longParts(values)...)
		}
		if password, ok := u.User.Password(); ok {
			parts = append(parts, password)
		}
	}
	var terms []string
	for _, p := range parts {
		terms = append(terms, p, url.QueryEscape(p), url.PathEscape(p))
	}
	terms = slices.DeleteFunc(terms, func(t string) bool { return t == "" })
	slices.Sort(terms)
	terms = slices.Compact(terms)
	slices.SortStableFunc(terms, func(a, b string) int { return len(b) - len(a) })
	return terms
}

// longParts returns the values that are long enough to be redacted on their own.
func longParts(values []string) []string {
	return slices.DeleteFunc(slices.Clone(values), func(v string) bool { return len(v) < minFragment })
}

// redactSecrets returns err with every secret replaced by the mask in its text; the chain behind it is unchanged.
func redactSecrets(err error, secrets []string) error {
	terms := redactionTerms(secrets)
	if len(terms) == 0 {
		return err
	}
	pairs := make([]string, 0, 2*len(terms))
	for _, t := range terms {
		pairs = append(pairs, t, maskedSecret)
	}
	text := err.Error()
	redacted := strings.NewReplacer(pairs...).Replace(text)
	if redacted == text {
		return err
	}
	return &redactedError{text: redacted, err: err}
}

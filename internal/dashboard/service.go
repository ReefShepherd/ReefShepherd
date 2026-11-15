package dashboard

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/db/dbgen"
	"github.com/reefshepherd/reefshepherd/internal/httpx"
	"github.com/reefshepherd/reefshepherd/internal/tanks"
)

const (
	// maxCards and maxOptionsBytes only guard against runaway requests.
	maxCards        = 100
	maxOptionsBytes = 2048

	typeParameter      = "parameter"
	typeParameterChart = "parameter_chart"
	typeTestsDue       = "tests_due"

	sizeNormal = "normal"
	sizeWide   = "wide"
)

// cardType describes one kind of dashboard card; later milestones add entries here.
type cardType struct {
	name string
	// parameter is true when the card shows one parameter and so needs a parameter_id.
	parameter bool
}

var cardTypes = []cardType{
	{typeParameter, true},
	{typeParameterChart, true},
	{typeTestsDue, false},
}

var sizes = []string{sizeNormal, sizeWide}

func lookupType(name string) (cardType, bool) {
	for _, t := range cardTypes {
		if t.name == name {
			return t, true
		}
	}
	return cardType{}, false
}

func typeNames() string {
	names := make([]string, len(cardTypes))
	for i, t := range cardTypes {
		names[i] = t.name
	}
	return strings.Join(names, ", ")
}

// Card is one dashboard card as the API shows it.
type Card struct {
	ID          int64           `json:"id"`
	CardType    string          `json:"card_type"`
	ParameterID *int64          `json:"parameter_id"`
	Position    int64           `json:"position"`
	Size        string          `json:"size"`
	Options     json.RawMessage `json:"options"`
}

// Layout is a user's ordered cards for one tank.
type Layout struct {
	Cards []Card `json:"cards"`
}

// CardInput is one card in a replacement layout; the order of the list decides positions, so Position is accepted only to let a client send back what it read.
type CardInput struct {
	CardType    string          `json:"card_type"`
	ParameterID *int64          `json:"parameter_id"`
	Position    int64           `json:"position"`
	Size        string          `json:"size"`
	Options     json.RawMessage `json:"options"`
}

// Service is the dashboard logic behind the HTTP handlers; its errors are *httpx.APIError when the client caused them.
type Service struct {
	Deps
	now func() time.Time
}

// NewService returns the service for deps.
func NewService(deps Deps) *Service {
	return &Service{Deps: deps, now: time.Now}
}

func (s *Service) stamp() string { return s.now().UTC().Format(db.TimeLayout) }

// Get returns the user's cards for a tank, creating the default layout the first time the user has none.
func (s *Service) Get(ctx context.Context, userID, tankID int64) (Layout, error) {
	q := db.Q(s.DB.R)
	if err := tanks.Require(ctx, s.DB.R, tankID); err != nil {
		return Layout{}, err
	}
	n, err := q.CountDashboardCards(ctx, dbgen.CountDashboardCardsParams{UserID: userID, TankID: tankID})
	if err != nil {
		return Layout{}, fmt.Errorf("count dashboard cards: %w", err)
	}
	if n == 0 {
		// The count is checked again inside the write so two first requests cannot both seed.
		err := s.DB.InTx(ctx, func(tx *sql.Tx) error {
			q := db.Q(tx)
			n, err := q.CountDashboardCards(ctx, dbgen.CountDashboardCardsParams{UserID: userID, TankID: tankID})
			if err != nil {
				return fmt.Errorf("count dashboard cards: %w", err)
			}
			if n > 0 {
				return nil
			}
			return s.seedDefault(ctx, q, userID, tankID)
		})
		if err != nil {
			return Layout{}, err
		}
	}
	return list(ctx, q, userID, tankID)
}

// Reset discards the user's cards for a tank and creates the default layout again.
func (s *Service) Reset(ctx context.Context, userID, tankID int64) (Layout, error) {
	var out Layout
	err := s.DB.InTx(ctx, func(tx *sql.Tx) error {
		q := db.Q(tx)
		if err := tanks.Require(ctx, tx, tankID); err != nil {
			return err
		}
		if _, err := q.DeleteDashboardCards(ctx, dbgen.DeleteDashboardCardsParams{UserID: userID, TankID: tankID}); err != nil {
			return fmt.Errorf("clear dashboard cards: %w", err)
		}
		if err := s.seedDefault(ctx, q, userID, tankID); err != nil {
			return err
		}
		var err error
		out, err = list(ctx, q, userID, tankID)
		return err
	})
	return out, err
}

// Replace swaps the cards the user can see for a tank in one transaction, numbering positions from the order given; hidden cards are kept in their relative order after the shown ones.
func (s *Service) Replace(ctx context.Context, userID, tankID int64, cards []CardInput) (Layout, error) {
	var out Layout
	err := s.DB.InTx(ctx, func(tx *sql.Tx) error {
		q := db.Q(tx)
		if err := tanks.Require(ctx, tx, tankID); err != nil {
			return err
		}
		enabled, err := q.ListEnabledTankParameterIDs(ctx, tankID)
		if err != nil {
			return fmt.Errorf("list enabled parameters: %w", err)
		}
		rows, fields := validate(cards, enabled)
		if len(fields) > 0 {
			return httpx.Validation("invalid dashboard", fields)
		}
		_, hidden, err := loadCards(ctx, q, userID, tankID, enabled)
		if err != nil {
			return err
		}
		if _, err := q.DeleteDashboardCards(ctx, dbgen.DeleteDashboardCardsParams{UserID: userID, TankID: tankID}); err != nil {
			return fmt.Errorf("clear dashboard cards: %w", err)
		}
		now := s.stamp()
		for i, row := range rows {
			row.UserID, row.TankID, row.Position, row.CreatedAt = userID, tankID, int64(i), now
			if err := q.InsertDashboardCard(ctx, row); err != nil {
				return fmt.Errorf("insert dashboard card: %w", err)
			}
		}
		for i, c := range hidden {
			err := q.InsertDashboardCard(ctx, dbgen.InsertDashboardCardParams{
				UserID: userID, TankID: tankID, CardType: c.CardType, ParameterID: c.ParameterID,
				Position: int64(len(rows) + i), Size: c.Size, OptionsJson: c.OptionsJson, CreatedAt: c.CreatedAt,
			})
			if err != nil {
				return fmt.Errorf("keep hidden dashboard card: %w", err)
			}
		}
		out, err = list(ctx, q, userID, tankID)
		return err
	})
	return out, err
}

// seedDefault inserts a tests_due card followed by one parameter card per enabled parameter, in the tank's parameter order.
func (s *Service) seedDefault(ctx context.Context, q *dbgen.Queries, userID, tankID int64) error {
	ids, err := q.ListEnabledTankParameterIDs(ctx, tankID)
	if err != nil {
		return fmt.Errorf("list enabled parameters: %w", err)
	}
	now := s.stamp()
	insert := func(position int, kind string, parameterID *int64) error {
		err := q.InsertDashboardCard(ctx, dbgen.InsertDashboardCardParams{
			UserID: userID, TankID: tankID, CardType: kind, ParameterID: parameterID,
			Position: int64(position), Size: sizeNormal, OptionsJson: "{}", CreatedAt: now,
		})
		if err != nil {
			return fmt.Errorf("insert default dashboard card: %w", err)
		}
		return nil
	}
	if err := insert(0, typeTestsDue, nil); err != nil {
		return err
	}
	for i, id := range ids {
		if err := insert(i+1, typeParameter, &id); err != nil {
			return err
		}
	}
	return nil
}

// loadCards splits the user's cards for a tank into those shown and those hidden because their parameter is not in enabled.
func loadCards(ctx context.Context, q *dbgen.Queries, userID, tankID int64, enabled []int64) (shown, hidden []dbgen.ListDashboardCardsRow, err error) {
	rows, err := q.ListDashboardCards(ctx, dbgen.ListDashboardCardsParams{UserID: userID, TankID: tankID})
	if err != nil {
		return nil, nil, fmt.Errorf("list dashboard cards: %w", err)
	}
	for _, r := range rows {
		if r.ParameterID == nil || slices.Contains(enabled, *r.ParameterID) {
			shown = append(shown, r)
		} else {
			hidden = append(hidden, r)
		}
	}
	return shown, hidden, nil
}

// list returns the cards the user sees, which may be none when every card is hidden.
func list(ctx context.Context, q *dbgen.Queries, userID, tankID int64) (Layout, error) {
	enabled, err := q.ListEnabledTankParameterIDs(ctx, tankID)
	if err != nil {
		return Layout{}, fmt.Errorf("list enabled parameters: %w", err)
	}
	shown, _, err := loadCards(ctx, q, userID, tankID, enabled)
	if err != nil {
		return Layout{}, err
	}
	out := Layout{Cards: make([]Card, len(shown))}
	for i, r := range shown {
		// Positions are counted over the cards shown, so hidden cards never leave a gap.
		out.Cards[i] = Card{
			ID: r.ID, CardType: r.CardType, ParameterID: r.ParameterID, Position: int64(i),
			Size: r.Size, Options: json.RawMessage(r.OptionsJson),
		}
	}
	return out, nil
}

// validate checks a replacement list against the card types and the tank's enabled parameters, returning the rows to insert or the problems keyed by field.
func validate(cards []CardInput, enabled []int64) ([]dbgen.InsertDashboardCardParams, map[string]string) {
	fields := map[string]string{}
	if len(cards) == 0 {
		fields["cards"] = "must have at least one card"
		return nil, fields
	}
	if len(cards) > maxCards {
		fields["cards"] = fmt.Sprintf("must have at most %d cards", maxCards)
		return nil, fields
	}
	rows := make([]dbgen.InsertDashboardCardParams, len(cards))
	type key struct {
		cardType    string
		parameterID int64
	}
	seen := map[key]int{}
	for i, c := range cards {
		field := func(name string) string { return fmt.Sprintf("cards.%d.%s", i, name) }
		row := dbgen.InsertDashboardCardParams{CardType: c.CardType, ParameterID: c.ParameterID, Size: c.Size}

		kind, known := lookupType(c.CardType)
		switch {
		case c.CardType == "":
			fields[field("card_type")] = "is required"
		case !known:
			fields[field("card_type")] = "must be one of " + typeNames()
		case kind.parameter && c.ParameterID == nil:
			fields[field("parameter_id")] = "is required for this card type"
		case kind.parameter && !slices.Contains(enabled, *c.ParameterID):
			fields[field("parameter_id")] = "must be a parameter enabled on this tank"
		case !kind.parameter && c.ParameterID != nil:
			fields[field("parameter_id")] = "must be empty for this card type"
		default:
			k := key{c.CardType, 0}
			if c.ParameterID != nil {
				k.parameterID = *c.ParameterID
			}
			if first, dup := seen[k]; dup {
				fields[fmt.Sprintf("cards.%d", i)] = fmt.Sprintf("duplicates cards.%d", first)
			} else {
				seen[k] = i
			}
		}

		if row.Size == "" {
			row.Size = sizeNormal
		}
		if !slices.Contains(sizes, row.Size) {
			fields[field("size")] = "must be one of " + strings.Join(sizes, ", ")
		}

		opts, err := cleanOptions(c.Options)
		if err != nil {
			fields[field("options")] = err.Error()
		}
		row.OptionsJson = opts
		rows[i] = row
	}
	if len(fields) > 0 {
		return nil, fields
	}
	return rows, nil
}

// cleanOptions accepts a JSON object (or nothing) and returns it compacted, "{}" when empty.
func cleanOptions(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "{}", nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil || obj == nil {
		return "", errors.New("must be a JSON object")
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return "", errors.New("must be a JSON object")
	}
	if buf.Len() > maxOptionsBytes {
		return "", fmt.Errorf("must be at most %d bytes", maxOptionsBytes)
	}
	return buf.String(), nil
}

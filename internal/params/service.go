package params

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/db/dbgen"
	"github.com/reefshepherd/reefshepherd/internal/httpx"
	"github.com/reefshepherd/reefshepherd/internal/units"
)

// Limits on request fields.
const (
	maxNameRunes       = 60
	maxUnitLabelRunes  = 16
	maxNoteRunes       = 2000
	maxTestInterval    = 3650
	maxSortOrder       = 100000
	maxResultsPerEntry = 100
	// maxFuture is how far past the server clock a measurement time may lie, which absorbs clock and time zone slips without letting a typo pin "latest" to the far future.
	maxFuture = 24 * time.Hour
)

// Service is the parameter and result logic behind the HTTP handlers; the errors it returns for a client mistake are field errors and the sentinels below.
type Service struct {
	Deps
	// r serves reads; it is a field so a test can count the statements a read issues.
	r   dbgen.DBTX
	now func() time.Time
}

// NewService returns the service for deps.
func NewService(deps Deps) *Service {
	return &Service{Deps: deps, r: deps.DB.R, now: time.Now}
}

// Result is a stored test result with its value in the parameter's canonical unit.
type Result struct {
	ID          int64
	TankID      int64
	ParameterID int64
	MeasuredAt  time.Time
	Value       float64
	Note        string
	Source      string
	CreatedBy   *int64
}

// Refusals the handlers turn into 404 and 409 responses; the text is the message sent.
var (
	errNoParameter       = errors.New("no such parameter")
	errNoResult          = errors.New("no such result")
	errBuiltinParameter  = errors.New("built-in parameters cannot be deleted, archive it instead")
	errParameterHasData  = errors.New("this parameter has test results, archive it instead")
	errParameterArchived = errors.New("this parameter is archived")
)

func cleanText(s string, maxRunes int) (string, bool) {
	s = strings.TrimSpace(s)
	return s, utf8.RuneCountInString(s) <= maxRunes
}

// parseTimestamp reads an RFC 3339 instant and drops the sub-second part.
func parseTimestamp(s string) (time.Time, bool) {
	t, err := db.ParseTime(s)
	return t.Truncate(time.Second), err == nil
}

// meta says how one parameter is shown: the unit chosen for its family, or its own label and decimals when the family is fixed.
type meta struct {
	family   units.Family
	unit     units.Unit
	label    string
	decimals int
}

// display resolves meta values for one request and reads each family's unit setting at most once.
type display struct {
	s     *Service
	ctx   context.Context
	units map[units.Family]units.Unit
}

func (s *Service) newDisplay(ctx context.Context) *display {
	return &display{s: s, ctx: ctx, units: map[units.Family]units.Unit{}}
}

func (d *display) meta(family, label string, decimals int64) (meta, error) {
	f := units.Family(family)
	if f == units.Fixed {
		return meta{family: f, unit: units.None, label: label, decimals: int(decimals)}, nil
	}
	u, ok := d.units[f]
	if !ok {
		var err error
		if u, err = d.s.Settings.GetUnit(d.ctx, f); err != nil {
			// Not wrapped: an unknown stored unit family is a server fault and must not reach the client as a field error.
			return meta{}, fmt.Errorf("read display unit for %s: %v", f, err) //nolint:errorlint // see above
		}
		d.units[f] = u
	}
	info, ok := units.Info(f, u)
	if !ok {
		return meta{}, fmt.Errorf("unit %q is not in family %q", u, f)
	}
	return meta{family: f, unit: u, label: info.Label, decimals: info.Decimals}, nil
}

// show converts a canonical value to the display unit, rounded to the display precision.
func (m meta) show(canonical float64) float64 {
	return units.Round(units.FromCanonical(m.family, m.unit, canonical), m.decimals)
}

func (m meta) showPtr(canonical *float64) *float64 {
	if canonical == nil {
		return nil
	}
	v := m.show(*canonical)
	return &v
}

// canonical converts a display value to the canonical unit, with a field error code when it cannot.
func (m meta) canonical(v float64) (float64, httpx.Code) {
	c, err := units.ToCanonical(m.family, m.unit, v)
	switch {
	case err == nil:
		return c, ""
	case errors.Is(err, units.ErrNotFinite):
		return 0, httpx.CodeInvalid
	default:
		return 0, httpx.CodeOutOfRange
	}
}

// inRange judges a value the way it is shown, so a reading displayed as the limit never counts as outside it.
func (m meta) inRange(canonical float64, low, high *float64) bool {
	v := m.show(canonical)
	if low != nil && v < m.show(*low) {
		return false
	}
	if high != nil && v > m.show(*high) {
		return false
	}
	return true
}

func (s *Service) location(ctx context.Context) (*time.Location, error) {
	name, err := s.Settings.GetTimezone(ctx)
	if err != nil {
		return nil, fmt.Errorf("read timezone: %w", err)
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("load timezone %q: %w", name, err)
	}
	return loc, nil
}

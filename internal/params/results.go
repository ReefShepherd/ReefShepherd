package params

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/db/dbgen"
	"github.com/reefshepherd/reefshepherd/internal/httpx"
	"github.com/reefshepherd/reefshepherd/internal/tanks"
)

// sourceManual marks results typed in by a user.
const sourceManual = "manual"

// Paging limits for the result list.
const (
	defaultPageSize = 50
	maxPageSize     = 200
)

// ResultView is a test result with its value in display units and its range check against the tank's current range.
type ResultView struct {
	ID            int64   `json:"id"`
	TankID        int64   `json:"tank_id"`
	ParameterID   int64   `json:"parameter_id"`
	ParameterName string  `json:"parameter_name"`
	MeasuredAt    string  `json:"measured_at"`
	Value         float64 `json:"value"`
	Unit          string  `json:"unit"`
	UnitLabel     string  `json:"unit_label"`
	InRange       bool    `json:"in_range"`
	Note          string  `json:"note"`
	Source        string  `json:"source"`
	CreatedBy     *int64  `json:"created_by"`
	CreatedAt     string  `json:"created_at"`
	UpdatedAt     string  `json:"updated_at"`
}

// paramEntry is what a result view needs to know about its parameter in one tank.
type paramEntry struct {
	row       dbgen.ListTankParametersRow
	meta      meta
	low, high *float64
}

// paramIndex finds each parameter's display and range for one tank.
type paramIndex map[int64]paramEntry

func (s *Service) paramIndex(ctx context.Context, q dbgen.DBTX, tankID int64) (paramIndex, error) {
	rows, err := db.Q(q).ListTankParameters(ctx, dbgen.ListTankParametersParams{TankID: tankID, IncludeArchived: 1})
	if err != nil {
		return nil, fmt.Errorf("list tank parameters: %w", err)
	}
	d := s.newDisplay(ctx)
	idx := make(paramIndex, len(rows))
	for _, row := range rows {
		m, err := d.meta(row.UnitFamily, row.UnitLabel, row.Decimals)
		if err != nil {
			return nil, err
		}
		low, high := rangeOf(row)
		idx[row.ParameterID] = paramEntry{row: row, meta: m, low: low, high: high}
	}
	return idx, nil
}

func (idx paramIndex) view(r dbgen.TestResult) (ResultView, error) {
	p, ok := idx[r.ParameterID]
	if !ok {
		return ResultView{}, fmt.Errorf("result %d has unknown parameter %d", r.ID, r.ParameterID)
	}
	return ResultView{
		ID: r.ID, TankID: r.TankID, ParameterID: r.ParameterID, ParameterName: p.row.Name,
		MeasuredAt: r.MeasuredAt, Value: p.meta.show(r.Value), Unit: string(p.meta.unit), UnitLabel: p.meta.label,
		InRange: p.meta.inRange(r.Value, p.low, p.high), Note: r.Note, Source: r.Source, CreatedBy: r.CreatedBy,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}, nil
}

// ResultEntry is one parameter's reading in a multi-parameter entry, with the value in display units.
type ResultEntry struct {
	ParameterID int64
	Value       *float64
	Note        string
}

// CreateResultsInput is a set of readings taken at one time.
type CreateResultsInput struct {
	// MeasuredAt is an RFC 3339 timestamp, the current time when nil.
	MeasuredAt *string
	Results    []ResultEntry
}

// reading is one validated value in the parameter's canonical unit.
type reading struct {
	parameterID int64
	value       float64
	note        string
}

// CreateResults stores all readings of an entry in one transaction and returns them in the order given; the tank and the parameters are checked inside it, so a change made meanwhile cannot slip past.
func (s *Service) CreateResults(ctx context.Context, tankID, userID int64, in CreateResultsInput) ([]ResultView, error) {
	var stored []dbgen.TestResult
	var measured time.Time
	err := s.DB.InTx(ctx, func(tx *sql.Tx) error {
		stored = stored[:0]
		if err := tanks.Require(ctx, tx, tankID); err != nil {
			return err
		}
		q := db.Q(tx)
		fields := fieldErrors{}
		measured = s.checkMeasuredAt(in.MeasuredAt, fields)
		switch {
		case len(in.Results) == 0:
			fields.add("results", "must list at least one result")
		case len(in.Results) > maxResultsPerEntry:
			fields.add("results", fmt.Sprintf("must list at most %d results", maxResultsPerEntry))
			return fields.err("invalid results")
		}
		readings, err := s.checkReadings(ctx, q, in.Results, fields)
		if err != nil {
			return err
		}
		if err := fields.err("invalid results"); err != nil {
			return err
		}
		stamp := s.stamp()
		for _, rd := range readings {
			row := dbgen.TestResult{
				TankID: tankID, ParameterID: rd.parameterID, MeasuredAt: measured.Format(db.TimeLayout), Value: rd.value,
				Note: rd.note, Source: sourceManual, CreatedBy: &userID, CreatedAt: stamp, UpdatedAt: stamp,
			}
			id, err := q.InsertTestResult(ctx, dbgen.InsertTestResultParams{
				TankID: row.TankID, ParameterID: row.ParameterID, MeasuredAt: row.MeasuredAt, Value: row.Value,
				Note: row.Note, Source: row.Source, CreatedBy: row.CreatedBy, CreatedAt: stamp, UpdatedAt: stamp,
			})
			if err != nil {
				return fmt.Errorf("insert result: %w", err)
			}
			row.ID = id
			stored = append(stored, row)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if s.OnResultCreated != nil {
		for _, row := range stored {
			s.OnResultCreated(ctx, Result{
				ID: row.ID, TankID: row.TankID, ParameterID: row.ParameterID, MeasuredAt: measured, Value: row.Value,
				Note: row.Note, Source: row.Source, CreatedBy: row.CreatedBy,
			})
		}
	}

	idx, err := s.paramIndex(ctx, s.r, tankID)
	if err != nil {
		return nil, err
	}
	out := make([]ResultView, 0, len(stored))
	for _, row := range stored {
		v, err := idx.view(row)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

// checkReadings validates the entries against the active parameter definitions, adding every problem to fields, and returns the readings converted to canonical units.
func (s *Service) checkReadings(ctx context.Context, q *dbgen.Queries, entries []ResultEntry, fields fieldErrors) ([]reading, error) {
	defs, err := q.ListParameterDefinitions(ctx, 1)
	if err != nil {
		return nil, fmt.Errorf("list parameter definitions: %w", err)
	}
	byID := make(map[int64]dbgen.ParameterDefinition, len(defs))
	for _, def := range defs {
		byID[def.ID] = def
	}

	d := s.newDisplay(ctx)
	seen := map[int64]bool{}
	readings := make([]reading, 0, len(entries))
	for i, e := range entries {
		field := func(name string) string { return "results." + strconv.Itoa(i) + "." + name }
		def, known := byID[e.ParameterID]
		switch {
		case !known:
			fields.add(field("parameter_id"), "no such parameter")
		case def.ArchivedAt != nil:
			fields.add(field("parameter_id"), "this parameter is archived")
		case seen[e.ParameterID]:
			fields.add(field("parameter_id"), "is listed more than once")
		}
		seen[e.ParameterID] = true
		note, short := cleanText(e.Note, maxNoteRunes)
		if !short {
			fields.add(field("note"), fmt.Sprintf("must be at most %d characters", maxNoteRunes))
		}
		if e.Value == nil {
			fields.add(field("value"), "is required")
		}
		if !known || e.Value == nil {
			continue
		}
		m, err := d.meta(def.UnitFamily, def.UnitLabel, def.Decimals)
		if err != nil {
			return nil, err
		}
		canonical, msg := m.canonical(*e.Value)
		if msg != "" {
			fields.add(field("value"), msg)
			continue
		}
		readings = append(readings, reading{e.ParameterID, canonical, note})
	}
	return readings, nil
}

// checkMeasuredAt parses a measurement time, defaulting to now and refusing one far in the future.
func (s *Service) checkMeasuredAt(raw *string, fields fieldErrors) time.Time {
	now := s.now().UTC().Truncate(time.Second)
	if raw == nil {
		return now
	}
	t, err := parseTimestamp(*raw)
	if err != nil {
		fields.add("measured_at", err.Error())
		return now
	}
	if t.After(now.Add(maxFuture)) {
		fields.add("measured_at", "must not be more than a day in the future")
	}
	return t
}

// UpdateResultInput holds the changes to a result; nil fields are left alone and the value is in display units.
type UpdateResultInput struct {
	MeasuredAt *string
	Value      *float64
	Note       *string
}

// UpdateResult changes a result's time, value or note.
func (s *Service) UpdateResult(ctx context.Context, id int64, in UpdateResultInput) (ResultView, error) {
	var tankID int64
	err := s.DB.InTx(ctx, func(tx *sql.Tx) error {
		q := db.Q(tx)
		cur, err := q.GetTestResult(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			return notFound("result")
		}
		if err != nil {
			return fmt.Errorf("read result: %w", err)
		}
		tankID = cur.TankID
		def, err := q.GetParameterDefinition(ctx, cur.ParameterID)
		if err != nil {
			return fmt.Errorf("read parameter definition: %w", err)
		}
		m, err := s.newDisplay(ctx).meta(def.UnitFamily, def.UnitLabel, def.Decimals)
		if err != nil {
			return err
		}

		fields := fieldErrors{}
		measuredAt, value, note := cur.MeasuredAt, cur.Value, cur.Note
		if in.MeasuredAt != nil {
			measuredAt = s.checkMeasuredAt(in.MeasuredAt, fields).Format(db.TimeLayout)
		}
		if in.Value != nil {
			c, msg := m.canonical(*in.Value)
			if msg != "" {
				fields.add("value", msg)
			}
			value = c
		}
		if in.Note != nil {
			var short bool
			if note, short = cleanText(*in.Note, maxNoteRunes); !short {
				fields.add("note", fmt.Sprintf("must be at most %d characters", maxNoteRunes))
			}
		}
		if err := fields.err("invalid result"); err != nil {
			return err
		}
		if _, err := q.UpdateTestResult(ctx, dbgen.UpdateTestResultParams{MeasuredAt: measuredAt, Value: value, Note: note, UpdatedAt: s.stamp(), ID: id}); err != nil {
			return fmt.Errorf("update result: %w", err)
		}
		return nil
	})
	if err != nil {
		return ResultView{}, err
	}
	row, err := db.Q(s.r).GetTestResult(ctx, id)
	if err != nil {
		return ResultView{}, fmt.Errorf("read result: %w", err)
	}
	idx, err := s.paramIndex(ctx, s.r, tankID)
	if err != nil {
		return ResultView{}, err
	}
	return idx.view(row)
}

// DeleteResult removes a result.
func (s *Service) DeleteResult(ctx context.Context, id int64) error {
	n, err := db.Q(s.DB.W).DeleteTestResult(ctx, id)
	if err != nil {
		return fmt.Errorf("delete result: %w", err)
	}
	if n == 0 {
		return notFound("result")
	}
	return nil
}

// ResultFilter narrows the result list; unset fields match everything.
type ResultFilter struct {
	ParameterID *int64
	From, To    *time.Time
	Cursor      string
	Limit       int
}

// ResultPage is one page of results, newest first.
type ResultPage struct {
	Results []ResultView `json:"results"`
	// NextCursor is passed back as the cursor to get the following page, and is null on the last page.
	NextCursor *string `json:"next_cursor"`
}

// ListResults returns a page of the tank's results, newest first.
func (s *Service) ListResults(ctx context.Context, tankID int64, f ResultFilter) (ResultPage, error) {
	if err := tanks.Require(ctx, s.r, tankID); err != nil {
		return ResultPage{}, err
	}
	limit := f.Limit
	if limit == 0 {
		limit = defaultPageSize
	}
	arg := dbgen.ListTankResultsParams{
		TankID: tankID, ParameterID: f.ParameterID, FromAt: formatTimePtr(f.From), ToAt: formatTimePtr(f.To),
		RowLimit: int64(limit) + 1,
	}
	if f.Cursor != "" {
		at, id, err := decodeCursor(f.Cursor)
		if err != nil {
			return ResultPage{}, httpx.Validation("invalid cursor", map[string]string{"cursor": "is not a cursor from a previous page"})
		}
		arg.CursorAt, arg.CursorID = &at, &id
	}
	rows, err := db.Q(s.r).ListTankResults(ctx, arg)
	if err != nil {
		return ResultPage{}, fmt.Errorf("list results: %w", err)
	}
	page := ResultPage{Results: []ResultView{}}
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		c := encodeCursor(last.MeasuredAt, last.ID)
		page.NextCursor = &c
	}
	idx, err := s.paramIndex(ctx, s.r, tankID)
	if err != nil {
		return ResultPage{}, err
	}
	for _, row := range rows {
		v, err := idx.view(row)
		if err != nil {
			return ResultPage{}, err
		}
		page.Results = append(page.Results, v)
	}
	return page, nil
}

func formatTimePtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.UTC().Format(db.TimeLayout)
	return &s
}

func encodeCursor(measuredAt string, id int64) string {
	return base64.RawURLEncoding.EncodeToString([]byte(measuredAt + "|" + strconv.FormatInt(id, 10)))
}

func decodeCursor(c string) (measuredAt string, id int64, err error) {
	raw, err := base64.RawURLEncoding.DecodeString(c)
	if err != nil {
		return "", 0, err
	}
	at, idText, ok := strings.Cut(string(raw), "|")
	if !ok {
		return "", 0, errors.New("malformed cursor")
	}
	if _, err := time.Parse(time.RFC3339, at); err != nil {
		return "", 0, err
	}
	id, err = strconv.ParseInt(idText, 10, 64)
	if err != nil {
		return "", 0, err
	}
	return at, id, nil
}

// SeriesPoint is one reading on a chart.
type SeriesPoint struct {
	ResultID   int64   `json:"result_id"`
	MeasuredAt string  `json:"measured_at"`
	Value      float64 `json:"value"`
	InRange    bool    `json:"in_range"`
	// Note is what was entered with the reading, empty when nothing was.
	Note string `json:"note"`
}

// SeriesRange is the ideal range in display units, with null for a side that has no limit.
type SeriesRange struct {
	Low  *float64 `json:"low"`
	High *float64 `json:"high"`
}

// Series is the chart data of one parameter in one tank, oldest reading first.
type Series struct {
	TankID      int64         `json:"tank_id"`
	ParameterID int64         `json:"parameter_id"`
	Name        string        `json:"name"`
	Unit        string        `json:"unit"`
	UnitLabel   string        `json:"unit_label"`
	Decimals    int           `json:"decimals"`
	Range       SeriesRange   `json:"range"`
	Points      []SeriesPoint `json:"points"`
}

// GetSeries returns the readings of a parameter in a tank between from and to, both optional and inclusive.
func (s *Service) GetSeries(ctx context.Context, tankID, parameterID int64, from, to *time.Time) (Series, error) {
	if err := tanks.Require(ctx, s.r, tankID); err != nil {
		return Series{}, err
	}
	rows, err := db.Q(s.r).ListTankParameters(ctx, dbgen.ListTankParametersParams{TankID: tankID, ParameterID: &parameterID, IncludeArchived: 1})
	if err != nil {
		return Series{}, fmt.Errorf("read tank parameter: %w", err)
	}
	if len(rows) == 0 {
		return Series{}, notFound("parameter")
	}
	row := rows[0]
	m, err := s.newDisplay(ctx).meta(row.UnitFamily, row.UnitLabel, row.Decimals)
	if err != nil {
		return Series{}, err
	}
	low, high := rangeOf(row)
	pts, err := db.Q(s.r).ListSeriesPoints(ctx, dbgen.ListSeriesPointsParams{
		TankID: tankID, ParameterID: parameterID, FromAt: formatTimePtr(from), ToAt: formatTimePtr(to),
	})
	if err != nil {
		return Series{}, fmt.Errorf("list series points: %w", err)
	}
	out := Series{
		TankID: tankID, ParameterID: parameterID, Name: row.Name, Unit: string(m.unit), UnitLabel: m.label, Decimals: m.decimals,
		Range:  SeriesRange{Low: m.showPtr(low), High: m.showPtr(high)},
		Points: make([]SeriesPoint, 0, len(pts)),
	}
	for _, p := range pts {
		out.Points = append(out.Points, SeriesPoint{ResultID: p.ID, MeasuredAt: p.MeasuredAt, Value: m.show(p.Value), InRange: m.inRange(p.Value, low, high), Note: p.Note})
	}
	return out, nil
}

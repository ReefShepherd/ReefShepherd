package params

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"

	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/db/dbgen"
	"github.com/reefshepherd/reefshepherd/internal/httpx"
	"github.com/reefshepherd/reefshepherd/internal/units"
)

const defaultDecimals = 2

// Definition is a parameter that can be tested, with defaults shown in the display unit.
type Definition struct {
	ID                      int64    `json:"id"`
	PresetKey               *string  `json:"preset_key"`
	Name                    string   `json:"name"`
	UnitFamily              string   `json:"unit_family"`
	Unit                    string   `json:"unit"`
	UnitLabel               string   `json:"unit_label"`
	Decimals                int      `json:"decimals"`
	DefaultLow              *float64 `json:"default_low"`
	DefaultHigh             *float64 `json:"default_high"`
	DefaultTestIntervalDays *int64   `json:"default_test_interval_days"`
	SortOrder               int64    `json:"sort_order"`
	ArchivedAt              *string  `json:"archived_at"`
}

func definitionOf(d *display, def dbgen.ParameterDefinition) (Definition, error) {
	m, err := d.meta(def.UnitFamily, def.UnitLabel, def.Decimals)
	if err != nil {
		return Definition{}, err
	}
	return Definition{
		ID: def.ID, PresetKey: def.PresetKey, Name: def.Name,
		UnitFamily: def.UnitFamily, Unit: string(m.unit), UnitLabel: m.label, Decimals: m.decimals,
		DefaultLow: m.showPtr(def.DefaultLow), DefaultHigh: m.showPtr(def.DefaultHigh),
		DefaultTestIntervalDays: def.DefaultTestIntervalDays, SortOrder: def.SortOrder, ArchivedAt: def.ArchivedAt,
	}, nil
}

// ListDefinitions returns the parameter definitions, archived ones only when asked for.
func (s *Service) ListDefinitions(ctx context.Context, includeArchived bool) ([]Definition, error) {
	rows, err := db.Q(s.r).ListParameterDefinitions(ctx, db.Flag(includeArchived))
	if err != nil {
		return nil, fmt.Errorf("list parameter definitions: %w", err)
	}
	d := s.newDisplay(ctx)
	out := make([]Definition, 0, len(rows))
	for _, row := range rows {
		def, err := definitionOf(d, row)
		if err != nil {
			return nil, err
		}
		out = append(out, def)
	}
	return out, nil
}

func (s *Service) definition(ctx context.Context, q dbgen.DBTX, id int64) (Definition, error) {
	row, err := db.Q(q).GetParameterDefinition(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Definition{}, notFound("parameter")
	}
	if err != nil {
		return Definition{}, fmt.Errorf("read parameter definition: %w", err)
	}
	return definitionOf(s.newDisplay(ctx), row)
}

// CreateDefinitionInput describes a custom parameter, which always uses the fixed unit family.
type CreateDefinitionInput struct {
	Name                    string
	UnitLabel               string
	Decimals                *int
	DefaultLow              *float64
	DefaultHigh             *float64
	DefaultTestIntervalDays *int
}

// CreateDefinition adds a custom parameter.
func (s *Service) CreateDefinition(ctx context.Context, in CreateDefinitionInput) (Definition, error) {
	fields := fieldErrors{}
	name := cleanName(in.Name, fields)
	label := cleanUnitLabel(in.UnitLabel, fields)
	decimals := int64(defaultDecimals)
	if in.Decimals != nil {
		decimals = cleanDecimals(*in.Decimals, fields)
	}
	checkRangePair(in.DefaultLow, in.DefaultHigh, "default_low", "default_high", fields)
	interval := cleanInterval(in.DefaultTestIntervalDays, fields)
	if err := fields.err("invalid parameter"); err != nil {
		return Definition{}, err
	}

	var id int64
	err := s.DB.InTx(ctx, func(tx *sql.Tx) error {
		q := db.Q(tx)
		if err := checkNameFree(ctx, q, name, 0); err != nil {
			return err
		}
		now := db.FormatTime(s.now())
		var err error
		id, err = q.CreateCustomParameter(ctx, dbgen.CreateCustomParameterParams{
			Name: name, UnitLabel: label, Decimals: decimals,
			DefaultLow: in.DefaultLow, DefaultHigh: in.DefaultHigh, DefaultTestIntervalDays: interval,
			CreatedAt: now, UpdatedAt: now,
		})
		if err != nil {
			return fmt.Errorf("create parameter: %w", err)
		}
		return nil
	})
	if err != nil {
		return Definition{}, err
	}
	return s.definition(ctx, s.r, id)
}

// UpdateDefinitionInput holds the changes to a parameter; nil and unset fields are left alone.
type UpdateDefinitionInput struct {
	Name                    *string
	UnitLabel               *string
	Decimals                *int
	DefaultLow              httpx.Nullable[float64]
	DefaultHigh             httpx.Nullable[float64]
	DefaultTestIntervalDays httpx.Nullable[int]
}

// UpdateDefinition changes a parameter's name, defaults and, for the fixed family, its label and decimals.
func (s *Service) UpdateDefinition(ctx context.Context, id int64, in UpdateDefinitionInput) (Definition, error) {
	err := s.DB.InTx(ctx, func(tx *sql.Tx) error {
		q := db.Q(tx)
		cur, err := q.GetParameterDefinition(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			return notFound("parameter")
		}
		if err != nil {
			return fmt.Errorf("read parameter definition: %w", err)
		}
		m, err := s.newDisplay(ctx).meta(cur.UnitFamily, cur.UnitLabel, cur.Decimals)
		if err != nil {
			return err
		}

		fields := fieldErrors{}
		name, label, decimals := cur.Name, cur.UnitLabel, cur.Decimals
		if in.Name != nil {
			name = cleanName(*in.Name, fields)
		}
		if units.Family(cur.UnitFamily) == units.Fixed {
			if in.UnitLabel != nil {
				label = cleanUnitLabel(*in.UnitLabel, fields)
			}
			if in.Decimals != nil {
				decimals = cleanDecimals(*in.Decimals, fields)
			}
		} else {
			if in.UnitLabel != nil {
				fields.add("unit_label", "cannot be changed for a parameter with a unit choice")
			}
			if in.Decimals != nil {
				fields.add("decimals", "cannot be changed for a parameter with a unit choice")
			}
		}
		low := mergeCanonical(m, cur.DefaultLow, in.DefaultLow, "default_low", fields)
		high := mergeCanonical(m, cur.DefaultHigh, in.DefaultHigh, "default_high", fields)
		checkRangePair(low, high, "default_low", "default_high", fields)
		interval := cur.DefaultTestIntervalDays
		if in.DefaultTestIntervalDays.Set {
			interval = cleanInterval(in.DefaultTestIntervalDays.Value, fields)
		}
		if err := fields.err("invalid parameter"); err != nil {
			return err
		}
		if err := checkNameFree(ctx, q, name, id); err != nil {
			return err
		}
		if _, err := q.UpdateParameterDefinition(ctx, dbgen.UpdateParameterDefinitionParams{
			Name: name, UnitLabel: label, Decimals: decimals,
			DefaultLow: low, DefaultHigh: high, DefaultTestIntervalDays: interval,
			UpdatedAt: db.FormatTime(s.now()), ID: id,
		}); err != nil {
			return fmt.Errorf("update parameter definition: %w", err)
		}
		return nil
	})
	if err != nil {
		return Definition{}, err
	}
	return s.definition(ctx, s.r, id)
}

// SetDefinitionArchived archives a parameter, hiding it while keeping its results, or brings it back.
func (s *Service) SetDefinitionArchived(ctx context.Context, id int64, archived bool) (Definition, error) {
	err := s.DB.InTx(ctx, func(tx *sql.Tx) error {
		q := db.Q(tx)
		cur, err := q.GetParameterDefinition(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			return notFound("parameter")
		}
		if err != nil {
			return fmt.Errorf("read parameter definition: %w", err)
		}
		if archived == (cur.ArchivedAt != nil) {
			return nil
		}
		var at *string
		if archived {
			now := db.FormatTime(s.now())
			at = &now
		}
		if _, err := q.SetParameterArchived(ctx, dbgen.SetParameterArchivedParams{ArchivedAt: at, UpdatedAt: db.FormatTime(s.now()), ID: id}); err != nil {
			return fmt.Errorf("set parameter archived: %w", err)
		}
		return nil
	})
	if err != nil {
		return Definition{}, err
	}
	return s.definition(ctx, s.r, id)
}

// DeleteDefinition removes a custom parameter that has no results; anything else can only be archived.
func (s *Service) DeleteDefinition(ctx context.Context, id int64) error {
	return s.DB.InTx(ctx, func(tx *sql.Tx) error {
		q := db.Q(tx)
		n, err := q.DeleteCustomParameter(ctx, id)
		if err != nil {
			return fmt.Errorf("delete parameter: %w", err)
		}
		if n > 0 {
			return nil
		}
		cur, err := q.GetParameterDefinition(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			return notFound("parameter")
		}
		if err != nil {
			return fmt.Errorf("read parameter definition: %w", err)
		}
		if cur.PresetKey != nil {
			return httpx.Conflict("built-in parameters cannot be deleted, archive it instead")
		}
		return httpx.Conflict("this parameter has test results, archive it instead")
	})
}

func cleanName(name string, fields fieldErrors) string {
	name, short := cleanText(name, maxNameRunes)
	switch {
	case name == "":
		fields.add("name", "must not be blank")
	case !short:
		fields.add("name", fmt.Sprintf("must be at most %d characters", maxNameRunes))
	}
	return name
}

func cleanUnitLabel(label string, fields fieldErrors) string {
	label, short := cleanText(label, maxUnitLabelRunes)
	if !short {
		fields.add("unit_label", fmt.Sprintf("must be at most %d characters", maxUnitLabelRunes))
	}
	return label
}

func cleanDecimals(v int, fields fieldErrors) int64 {
	if v < 0 || v > 4 {
		fields.add("decimals", "must be between 0 and 4")
	}
	return int64(v)
}

func cleanInterval(v *int, fields fieldErrors) *int64 {
	if v == nil {
		return nil
	}
	if *v < 1 || *v > maxTestInterval {
		fields.add("default_test_interval_days", fmt.Sprintf("must be between 1 and %d", maxTestInterval))
		return nil
	}
	n := int64(*v)
	return &n
}

// checkRangePair reports a low limit above the high one, or a limit that is not a finite number.
func checkRangePair(low, high *float64, lowField, highField string, fields fieldErrors) {
	for _, p := range []struct {
		v     *float64
		field string
	}{{low, lowField}, {high, highField}} {
		if p.v != nil && (math.IsNaN(*p.v) || math.IsInf(*p.v, 0)) {
			fields.add(p.field, "must be a finite number")
		}
	}
	if low != nil && high != nil && *low > *high {
		fields.add(lowField, "must not be above "+highField)
	}
}

// mergeCanonical applies a display-unit change to a canonical limit: unset keeps it, null clears it, a number replaces it.
func mergeCanonical(m meta, current *float64, change httpx.Nullable[float64], field string, fields fieldErrors) *float64 {
	if !change.Set {
		return current
	}
	if change.Value == nil {
		return nil
	}
	c, msg := m.canonical(*change.Value)
	if msg != "" {
		fields.add(field, msg)
		return current
	}
	return &c
}

func checkNameFree(ctx context.Context, q *dbgen.Queries, name string, selfID int64) error {
	n, err := q.CountParametersNamed(ctx, dbgen.CountParametersNamedParams{Name: name, ID: selfID})
	if err != nil {
		return fmt.Errorf("check parameter name: %w", err)
	}
	if n > 0 {
		return httpx.Validation("invalid parameter", map[string]string{"name": "another parameter already has this name"})
	}
	return nil
}

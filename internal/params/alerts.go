package params

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/tanks"
)

// DueParameter is an enabled, scheduled parameter whose next test has come due.
type DueParameter struct {
	TankID      int64
	TankName    string
	ParameterID int64
	Name        string
	DueAt       time.Time
}

// DueTests lists enabled parameters with a schedule on non-archived tanks whose next test is due at or before now.
func (s *Service) DueTests(ctx context.Context, now time.Time) ([]DueParameter, error) {
	tankRows, err := db.Q(s.r).ListTanks(ctx, db.Flag(false))
	if err != nil {
		return nil, fmt.Errorf("list tanks: %w", err)
	}
	loc, err := s.location(ctx)
	if err != nil {
		return nil, err
	}
	d := s.newDisplay(ctx)
	var out []DueParameter
	for _, tank := range tankRows {
		rows, err := tankParameterRows(ctx, s.r, tank.ID, false)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			item, err := tankParameterOf(d, row, loc, now)
			if err != nil {
				return nil, err
			}
			// NextDue is only set for an enabled parameter with a schedule.
			if item.NextDue == nil {
				continue
			}
			due, err := db.ParseTime(*item.NextDue)
			if err != nil {
				return nil, fmt.Errorf("next test time of parameter %d: %w", row.ParameterID, err)
			}
			if due.After(now) {
				continue
			}
			out = append(out, DueParameter{TankID: tank.ID, TankName: tank.Name, ParameterID: row.ParameterID, Name: row.Name, DueAt: due})
		}
	}
	return out, nil
}

// RangeAlert describes a stored result that lies outside its tank's ideal range, with values as display text.
type RangeAlert struct {
	TankID        int64
	TankName      string
	ParameterID   int64
	ParameterName string
	// Value is the result with its unit, such as "6.5 dKH".
	Value string
	// Low and High are the limits with their unit, empty when that side has no limit.
	Low, High string
	// Below is true when the value is under Low and false when it is over High.
	Below bool
}

// RangeAlert reports whether a stored result is outside its tank's current ideal range, with display-unit text.
func (s *Service) RangeAlert(ctx context.Context, r Result) (RangeAlert, bool, error) {
	tank, err := db.Q(s.r).GetTank(ctx, r.TankID)
	if errors.Is(err, sql.ErrNoRows) {
		return RangeAlert{}, false, tanks.ErrNotFound
	}
	if err != nil {
		return RangeAlert{}, false, fmt.Errorf("read tank: %w", err)
	}
	row, err := tankParameterRow(ctx, s.r, r.TankID, r.ParameterID)
	if err != nil {
		return RangeAlert{}, false, err
	}
	m, err := s.newDisplay(ctx).meta(row.UnitFamily, row.UnitLabel, row.Decimals)
	if err != nil {
		return RangeAlert{}, false, err
	}
	low, high := rangeOf(row)
	if m.inRange(r.Value, low, high) {
		return RangeAlert{}, false, nil
	}
	alert := RangeAlert{
		TankID: tank.ID, TankName: tank.Name, ParameterID: row.ParameterID, ParameterName: row.Name,
		Value: m.text(r.Value), Below: low != nil && m.show(r.Value) < m.show(*low),
	}
	if low != nil {
		alert.Low = m.text(*low)
	}
	if high != nil {
		alert.High = m.text(*high)
	}
	return alert, true, nil
}

// text formats a canonical value as display text with its unit, such as "6.5 dKH".
func (m meta) text(canonical float64) string {
	n := strconv.FormatFloat(m.show(canonical), 'f', m.decimals, 64)
	if m.label == "" {
		return n
	}
	return n + " " + m.label
}

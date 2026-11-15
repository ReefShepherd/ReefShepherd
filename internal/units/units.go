// Package units holds the unit catalog and the conversions between canonical storage units and display units.
package units

import (
	"errors"
	"fmt"
	"math"
	"slices"
)

// Family groups the units of one kind of quantity.
type Family string

// Unit identifies a unit within a family; the value is the stable code stored in settings.
type Unit string

// Unit families.
const (
	Temperature Family = "temperature"
	Salinity    Family = "salinity"
	Alkalinity  Family = "alkalinity"
	Phosphate   Family = "phosphate"
	Volume      Family = "volume"
	Fixed       Family = "fixed"
)

// Units, where None is the single unit of the fixed family, whose label comes from the parameter itself.
const (
	None       Unit = ""
	Celsius    Unit = "c"
	Fahrenheit Unit = "f"
	PPT        Unit = "ppt"
	SG         Unit = "sg"
	DKH        Unit = "dkh"
	MeqPerL    Unit = "meq_l"
	PPMCaCO3   Unit = "ppm_caco3"
	PPM        Unit = "ppm"
	PPB        Unit = "ppb"
	Liter      Unit = "l"
	USGallon   Unit = "us_gal"
	UKGallon   Unit = "uk_gal"
)

var (
	// ErrUnknownUnit is returned when a unit does not belong to the family.
	ErrUnknownUnit = errors.New("unknown unit for family")
	// ErrNotFinite is returned when a value is NaN or infinite.
	ErrNotFinite = errors.New("value is not finite")
	// ErrOutOfRange is returned when a value cannot be expressed in the canonical unit.
	ErrOutOfRange = errors.New("value out of range")
)

// UnitInfo describes one display unit.
type UnitInfo struct {
	Unit     Unit   `json:"unit"`
	Label    string `json:"label"`
	Decimals int    `json:"decimals"`

	// display = canonical*scale + offset for every unit except SG, which uses the density formula.
	scale, offset float64
}

// FamilyInfo describes a family, its canonical unit and its display options.
type FamilyInfo struct {
	Family    Family     `json:"family"`
	Canonical Unit       `json:"canonical"`
	Units     []UnitInfo `json:"units"`
}

func linear(u Unit, label string, decimals int, scale, offset float64) UnitInfo {
	return UnitInfo{Unit: u, Label: label, Decimals: decimals, scale: scale, offset: offset}
}

type familyDef struct {
	family    Family
	canonical Unit
	units     []UnitInfo
}

// families is read-only reference data; callers get copies so they cannot alter it.
var families = [...]familyDef{
	{Temperature, Celsius, []UnitInfo{
		linear(Celsius, "°C", 1, 1, 0),
		linear(Fahrenheit, "°F", 1, 9.0/5.0, 32),
	}},
	{Salinity, PPT, []UnitInfo{
		linear(PPT, "ppt", 1, 1, 0),
		linear(SG, "SG", 3, 1, 0),
	}},
	{Alkalinity, DKH, []UnitInfo{
		linear(DKH, "dKH", 1, 1, 0),
		linear(MeqPerL, "meq/L", 2, 0.3566, 0),
		linear(PPMCaCO3, "ppm CaCO3", 0, 17.848, 0),
	}},
	{Phosphate, PPM, []UnitInfo{
		linear(PPM, "ppm", 2, 1, 0),
		linear(PPB, "ppb", 0, 1000, 0),
	}},
	{Volume, Liter, []UnitInfo{
		linear(Liter, "L", 1, 1, 0),
		linear(USGallon, "US gal", 1, 1/3.785411784, 0),
		linear(UKGallon, "UK gal", 1, 1/4.54609, 0),
	}},
	{Fixed, None, []UnitInfo{
		linear(None, "", 2, 1, 0),
	}},
}

func (d familyDef) info() FamilyInfo {
	return FamilyInfo{Family: d.family, Canonical: d.canonical, Units: slices.Clone(d.units)}
}

// Catalog returns every family with its units, canonical unit first in each list.
func Catalog() []FamilyInfo {
	out := make([]FamilyInfo, len(families))
	for i := range families {
		out[i] = families[i].info()
	}
	return out
}

// Lookup returns the catalog entry for a family.
func Lookup(f Family) (FamilyInfo, bool) {
	for i := range families {
		if families[i].family == f {
			return families[i].info(), true
		}
	}
	return FamilyInfo{}, false
}

// Info returns the catalog entry for a unit; the fixed family accepts any unit label.
func Info(f Family, u Unit) (UnitInfo, bool) {
	for i := range families {
		d := &families[i]
		if d.family != f {
			continue
		}
		if f == Fixed {
			return d.units[0], true
		}
		for _, ui := range d.units {
			if ui.Unit == u {
				return ui, true
			}
		}
		return UnitInfo{}, false
	}
	return UnitInfo{}, false
}

// Precision returns the number of display decimals for a unit, or -1 when the unit is unknown.
func Precision(f Family, u Unit) int {
	ui, ok := Info(f, u)
	if !ok {
		return -1
	}
	return ui.Decimals
}

// Round rounds v to the given number of decimals.
func Round(v float64, decimals int) float64 {
	p := math.Pow(10, float64(decimals))
	return math.Round(v*p) / p
}

// canonicalRange returns the physically meaningful span of a family's canonical unit.
func canonicalRange(f Family) (lo, hi float64) {
	switch f {
	case Temperature:
		return -100, 500
	case Fixed:
		return -math.MaxFloat64, math.MaxFloat64
	default:
		return 0, math.MaxFloat64
	}
}

// ToCanonical converts a value in display unit u to the family's canonical unit, rejecting results outside the family's physical range.
func ToCanonical(f Family, u Unit, v float64) (float64, error) {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, fmt.Errorf("convert %q %q: %w", f, u, ErrNotFinite)
	}
	ui, ok := Info(f, u)
	if !ok {
		return 0, fmt.Errorf("convert %q %q: %w", f, u, ErrUnknownUnit)
	}
	if f == Salinity && u == SG {
		ppt, err := SGToSalinity(v, DefaultSampleC, DefaultReferenceC)
		if err != nil {
			return 0, fmt.Errorf("convert %q %q: %w", f, u, err)
		}
		return ppt, nil
	}
	c := (v - ui.offset) / ui.scale
	if lo, hi := canonicalRange(f); !(c >= lo && c <= hi) {
		return 0, fmt.Errorf("convert %q %q %v: %w", f, u, v, ErrOutOfRange)
	}
	return c, nil
}

// FromCanonical converts a canonical value to display unit u, returning NaN when the unit does not belong to the family.
func FromCanonical(f Family, u Unit, v float64) float64 {
	ui, ok := Info(f, u)
	if !ok {
		return math.NaN()
	}
	if f == Salinity && u == SG {
		return SalinityToSG(v, DefaultSampleC, DefaultReferenceC)
	}
	return v*ui.scale + ui.offset
}

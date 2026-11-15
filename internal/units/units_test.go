package units

import (
	"encoding/json"
	"errors"
	"math"
	"testing"
)

func TestCatalogShape(t *testing.T) {
	cat := Catalog()
	wantFamilies := []Family{Temperature, Salinity, Alkalinity, Phosphate, Volume, Fixed}
	if len(cat) != len(wantFamilies) {
		t.Fatalf("got %d families, want %d", len(cat), len(wantFamilies))
	}
	for i, fi := range cat {
		if fi.Family != wantFamilies[i] {
			t.Errorf("family %d = %q, want %q", i, fi.Family, wantFamilies[i])
		}
		if len(fi.Units) == 0 || fi.Units[0].Unit != fi.Canonical {
			t.Errorf("%s: first unit must be the canonical unit %q, got %+v", fi.Family, fi.Canonical, fi.Units)
		}
		seen := map[Unit]bool{}
		for _, ui := range fi.Units {
			if seen[ui.Unit] {
				t.Errorf("%s: duplicate unit %q", fi.Family, ui.Unit)
			}
			seen[ui.Unit] = true
			if ui.Decimals < 0 || ui.Decimals > 4 {
				t.Errorf("%s/%s: decimals %d out of range", fi.Family, ui.Unit, ui.Decimals)
			}
			if fi.Family != Fixed && ui.Label == "" {
				t.Errorf("%s/%s: empty label", fi.Family, ui.Unit)
			}
		}
	}
}

func TestCatalogDecimals(t *testing.T) {
	tests := []struct {
		f    Family
		u    Unit
		want int
	}{
		{Temperature, Celsius, 1}, {Temperature, Fahrenheit, 1},
		{Salinity, PPT, 1}, {Salinity, SG, 3},
		{Alkalinity, DKH, 1}, {Alkalinity, MeqPerL, 2}, {Alkalinity, PPMCaCO3, 0},
		{Phosphate, PPM, 2}, {Phosphate, PPB, 0},
		{Volume, Liter, 1}, {Volume, USGallon, 1}, {Volume, UKGallon, 1},
		{Fixed, None, 2}, {Fixed, "mV", 2},
		{Salinity, DKH, -1}, {"nope", PPT, -1},
	}
	for _, tc := range tests {
		if got := Precision(tc.f, tc.u); got != tc.want {
			t.Errorf("Precision(%s,%q) = %d, want %d", tc.f, tc.u, got, tc.want)
		}
	}
}

func TestCatalogJSON(t *testing.T) {
	b, err := json.Marshal(Catalog()[1])
	if err != nil {
		t.Fatal(err)
	}
	want := `{"family":"salinity","canonical":"ppt","units":[{"unit":"ppt","label":"ppt","decimals":1},{"unit":"sg","label":"SG","decimals":3}]}`
	if string(b) != want {
		t.Errorf("json = %s\nwant   %s", b, want)
	}
}

func TestCatalogIsFreshCopy(t *testing.T) {
	a := Catalog()
	a[0].Units[0].Label = "mutated"
	if got := Catalog()[0].Units[0].Label; got != "°C" {
		t.Errorf("catalog shared state: label = %q", got)
	}
}

// Known conversions, each checked in both directions.
func TestKnownConversions(t *testing.T) {
	tests := []struct {
		name      string
		f         Family
		u         Unit
		display   float64
		canonical float64
		tol       float64
	}{
		{"0C", Temperature, Fahrenheit, 32, 0, 1e-12},
		{"25C", Temperature, Fahrenheit, 77, 25, 1e-12},
		{"100C", Temperature, Fahrenheit, 212, 100, 1e-12},
		{"-40", Temperature, Fahrenheit, -40, -40, 1e-12},
		{"26.1C", Temperature, Fahrenheit, 78.98, 26.1, 1e-12},
		{"celsius identity", Temperature, Celsius, 25.5, 25.5, 0},
		{"8 dKH meq", Alkalinity, MeqPerL, 8 * 0.3566, 8, 1e-12},
		{"8 dKH ppm", Alkalinity, PPMCaCO3, 8 * 17.848, 8, 1e-12},
		{"1 dKH ppm", Alkalinity, PPMCaCO3, 17.848, 1, 1e-12},
		{"0.03 ppm", Phosphate, PPB, 30, 0.03, 1e-12},
		{"1 ppm", Phosphate, PPB, 1000, 1, 1e-12},
		{"1 US gal", Volume, USGallon, 1, 3.785411784, 1e-12},
		{"1 UK gal", Volume, UKGallon, 1, 4.54609, 1e-12},
		{"100 US gal", Volume, USGallon, 100, 378.5411784, 1e-9},
		{"liter identity", Volume, Liter, 120, 120, 0},
		{"fixed identity", Fixed, "mV", 7.5, 7.5, 0},
		{"fixed no unit", Fixed, None, 8.2, 8.2, 0},
		{"ppt identity", Salinity, PPT, 35, 35, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ToCanonical(tc.f, tc.u, tc.display)
			if err != nil {
				t.Fatal(err)
			}
			if math.Abs(got-tc.canonical) > tc.tol {
				t.Errorf("ToCanonical(%v) = %v, want %v", tc.display, got, tc.canonical)
			}
			back := FromCanonical(tc.f, tc.u, tc.canonical)
			if math.Abs(back-tc.display) > tc.tol {
				t.Errorf("FromCanonical(%v) = %v, want %v", tc.canonical, back, tc.display)
			}
		})
	}
}

// canonicalSamples gives typical canonical values for every family.
func canonicalSamples(f Family) []float64 {
	switch f {
	case Temperature:
		return []float64{-5, 0, 18.4, 24, 25, 25.7, 26.5, 28, 30.05, 40}
	case Salinity:
		return []float64{0, 1.5, 20, 30, 33.5, 34, 35, 35.5, 36.2, 40, 45}
	case Alkalinity:
		return []float64{0, 2, 6.5, 7, 8, 8.3, 9.1, 11, 14.95, 20}
	case Phosphate:
		return []float64{0, 0.001, 0.005, 0.02, 0.03, 0.08, 0.25, 1, 2.5}
	case Volume:
		return []float64{0.5, 5, 38, 75.7, 100, 208.2, 340, 1000, 5000}
	default:
		return []float64{-3, 0, 0.25, 7.9, 8.3, 100, 12345.678}
	}
}

// TestRoundTripEveryUnit covers canonical->display->canonical and display->canonical->display for every catalog unit.
func TestRoundTripEveryUnit(t *testing.T) {
	for _, fi := range Catalog() {
		for _, ui := range fi.Units {
			t.Run(string(fi.Family)+"/"+string(ui.Unit), func(t *testing.T) {
				halfStep := 0.5 * math.Pow(10, -float64(ui.Decimals))
				for _, c := range canonicalSamples(fi.Family) {
					d := FromCanonical(fi.Family, ui.Unit, c)
					if math.IsNaN(d) {
						t.Fatalf("FromCanonical(%v) is NaN", c)
					}
					back, err := ToCanonical(fi.Family, ui.Unit, d)
					if err != nil {
						t.Fatalf("ToCanonical(%v): %v", d, err)
					}
					// Exact round trip, apart from the bisection of the SG inverse.
					if tol := 1e-9 * math.Max(1, math.Abs(c)); math.Abs(back-c) > tol {
						t.Errorf("canonical %v -> display %v -> canonical %v", c, d, back)
					}
					// Rounding the displayed number to its precision moves the stored value by at most half a display step.
					shown := Round(d, ui.Decimals)
					rb, err := ToCanonical(fi.Family, ui.Unit, shown)
					if err != nil {
						t.Fatalf("ToCanonical(rounded %v): %v", shown, err)
					}
					if again := Round(FromCanonical(fi.Family, ui.Unit, rb), ui.Decimals); again != shown {
						t.Errorf("display %v -> canonical %v -> display %v, want %v", shown, rb, again, shown)
					}
					if math.Abs(shown-d) > halfStep*(1+1e-9) {
						t.Errorf("rounding %v to %d decimals moved it by %v", d, ui.Decimals, math.Abs(shown-d))
					}
				}
			})
		}
	}
}

// TestDisplayGridStable starts from values a user would type in a display unit.
func TestDisplayGridStable(t *testing.T) {
	grid := map[Family]map[Unit][]float64{
		Temperature: {Celsius: {24, 25.5, 26.1}, Fahrenheit: {75, 77, 78.8, 79.5, 82}},
		Salinity:    {PPT: {33, 34.5, 35}, SG: {1.023, 1.025, 1.026, 1.027}},
		Alkalinity:  {DKH: {7, 8.2, 9.5}, MeqPerL: {2.5, 2.85, 3.2}, PPMCaCO3: {100, 143, 160}},
		Phosphate:   {PPM: {0, 0.01, 0.03, 0.1}, PPB: {5, 30, 100}},
		Volume:      {Liter: {75.7, 200}, USGallon: {20, 55, 120}, UKGallon: {15, 40, 100}},
	}
	for f, byUnit := range grid {
		for u, values := range byUnit {
			dec := Precision(f, u)
			for _, d := range values {
				c, err := ToCanonical(f, u, d)
				if err != nil {
					t.Fatalf("%s/%s ToCanonical(%v): %v", f, u, d, err)
				}
				if got := Round(FromCanonical(f, u, c), dec); got != d {
					t.Errorf("%s/%s: %v -> canonical %v -> %v", f, u, d, c, got)
				}
			}
		}
	}
}

func TestToCanonicalErrors(t *testing.T) {
	tests := []struct {
		name string
		f    Family
		u    Unit
		v    float64
		want error
	}{
		{"wrong unit for family", Temperature, DKH, 1, ErrUnknownUnit},
		{"unknown family", "nope", Celsius, 1, ErrUnknownUnit},
		{"unknown unit", Volume, "bucket", 1, ErrUnknownUnit},
		{"NaN", Temperature, Celsius, math.NaN(), ErrNotFinite},
		{"+Inf", Alkalinity, DKH, math.Inf(1), ErrNotFinite},
		{"-Inf fixed", Fixed, None, math.Inf(-1), ErrNotFinite},
		{"SG below water", Salinity, SG, 0.99, ErrOutOfRange},
		{"SG far too high", Salinity, SG, 1.2, ErrOutOfRange},
		{"negative salinity", Salinity, PPT, -1, ErrOutOfRange},
		{"negative alkalinity", Alkalinity, DKH, -0.1, ErrOutOfRange},
		{"negative alkalinity in meq", Alkalinity, MeqPerL, -0.1, ErrOutOfRange},
		{"negative phosphate", Phosphate, PPM, -0.01, ErrOutOfRange},
		{"negative phosphate in ppb", Phosphate, PPB, -1, ErrOutOfRange},
		{"negative volume", Volume, Liter, -5, ErrOutOfRange},
		{"negative volume in gallons", Volume, USGallon, -5, ErrOutOfRange},
		{"temperature below the sane bound", Temperature, Celsius, -101, ErrOutOfRange},
		{"temperature above the sane bound", Temperature, Fahrenheit, 1000, ErrOutOfRange},
		{"conversion overflows to infinity", Volume, USGallon, math.MaxFloat64, ErrOutOfRange},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ToCanonical(tc.f, tc.u, tc.v)
			if !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestFromCanonicalUnknownIsNaN(t *testing.T) {
	if v := FromCanonical(Temperature, DKH, 25); !math.IsNaN(v) {
		t.Errorf("got %v, want NaN", v)
	}
	if v := FromCanonical("nope", Celsius, 25); !math.IsNaN(v) {
		t.Errorf("got %v, want NaN", v)
	}
}

func TestRound(t *testing.T) {
	tests := []struct {
		v    float64
		dec  int
		want float64
	}{
		{1.02637, 4, 1.0264}, {1.02634, 4, 1.0263}, {1.0264, 3, 1.026}, {34.45, 1, 34.5}, {-2.5, 0, -3}, {0.0296, 3, 0.03}, {12.4, 0, 12},
	}
	for _, tc := range tests {
		if got := Round(tc.v, tc.dec); got != tc.want {
			t.Errorf("Round(%v,%d) = %v, want %v", tc.v, tc.dec, got, tc.want)
		}
	}
}

func TestInfoAndLookup(t *testing.T) {
	if _, ok := Lookup("nope"); ok {
		t.Error("Lookup of unknown family succeeded")
	}
	fi, ok := Lookup(Alkalinity)
	if !ok || fi.Canonical != DKH || len(fi.Units) != 3 {
		t.Errorf("Lookup(Alkalinity) = %+v, %v", fi, ok)
	}
	if _, ok := Info(Phosphate, Liter); ok {
		t.Error("Info accepted a liter as a phosphate unit")
	}
	if ui, ok := Info(Fixed, "NTU"); !ok || ui.Label != "" {
		t.Errorf("Info(Fixed, NTU) = %+v, %v", ui, ok)
	}
}

func TestToCanonicalPhysicalRange(t *testing.T) {
	tests := []struct {
		f Family
		u Unit
		v float64
	}{
		{Temperature, Celsius, -5}, {Temperature, Fahrenheit, 14}, {Salinity, PPT, 0}, {Alkalinity, DKH, 0},
		{Phosphate, PPM, 0}, {Volume, Liter, 0}, {Fixed, None, -3.5},
	}
	for _, tc := range tests {
		if _, err := ToCanonical(tc.f, tc.u, tc.v); err != nil {
			t.Errorf("ToCanonical(%s, %s, %v): %v", tc.f, tc.u, tc.v, err)
		}
	}
}

func TestLookupAndInfoDoNotRebuildCatalog(t *testing.T) {
	if n := testing.AllocsPerRun(100, func() { _, _ = Info(Salinity, SG) }); n != 0 {
		t.Errorf("Info allocs = %v, want 0", n)
	}
	if n := testing.AllocsPerRun(100, func() { _, _ = ToCanonical(Alkalinity, MeqPerL, 2.5) }); n != 0 {
		t.Errorf("ToCanonical allocs = %v, want 0", n)
	}
	if n := testing.AllocsPerRun(100, func() { _ = FromCanonical(Volume, USGallon, 100) }); n != 0 {
		t.Errorf("FromCanonical allocs = %v, want 0", n)
	}
	if n := testing.AllocsPerRun(100, func() { _, _ = Lookup(Salinity) }); n > 1 {
		t.Errorf("Lookup allocs = %v, want at most 1", n)
	}
}

func TestLookupIsFreshCopy(t *testing.T) {
	fi, _ := Lookup(Temperature)
	fi.Units[0].Label = "mutated"
	if ui, _ := Info(Temperature, Celsius); ui.Label != "°C" {
		t.Errorf("Lookup shared state: label = %q", ui.Label)
	}
}

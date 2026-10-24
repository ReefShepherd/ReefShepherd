package units

import (
	"errors"
	"math"
	"testing"
)

// Check values for the EOS-80 density equations: the UNESCO technical paper 44 test table (as reproduced by
// the python seawater package documentation) and the one-atmosphere values at 5 C.
func TestEOS80CheckValues(t *testing.T) {
	tests := []struct {
		name string
		got  float64
		want float64
	}{
		{"rho_w(0)", PureWaterDensity(0), 999.842594},
		{"rho_w(30)", PureWaterDensity(30), 995.65113374},
		{"rho_sw(0,0)", SeawaterDensity(0, 0), 999.842594},
		{"rho_sw(35,0)", SeawaterDensity(35, 0), 1028.10633141},
		{"rho_sw(35,30)", SeawaterDensity(35, 30), 1021.72863949},
		{"rho_w(5)", PureWaterDensity(5), 999.96675},
		{"rho_sw(35,5)", SeawaterDensity(35, 5), 1027.67547},
		{"rho_w(25)", PureWaterDensity(25), 997.04796},
		{"rho_sw(35,25)", SeawaterDensity(35, 25), 1023.34306},
	}
	for _, tc := range tests {
		if math.Abs(tc.got-tc.want) > 5e-6 {
			t.Errorf("%s = %.8f, want %.8f", tc.name, tc.got, tc.want)
		}
	}
}

type sgRow struct {
	ppt                                  float64
	sg2525, sg2020, sg1515, sg204, sg254 float64
}

var sgTable = []sgRow{
	{0, 1.00000, 1.00000, 1.00000, 0.99823, 0.99707},
	{5, 1.00377, 1.00381, 1.00385, 1.00203, 1.00083},
	{10, 1.00753, 1.00760, 1.00769, 1.00582, 1.00458},
	{20, 1.01505, 1.01518, 1.01536, 1.01339, 1.01208},
	{30, 1.02259, 1.02279, 1.02304, 1.02098, 1.01959},
	{32, 1.02410, 1.02431, 1.02458, 1.02250, 1.02110},
	{33, 1.02486, 1.02508, 1.02535, 1.02326, 1.02186},
	{34, 1.02562, 1.02584, 1.02612, 1.02403, 1.02261},
	{35, 1.02637, 1.02660, 1.02690, 1.02479, 1.02337},
	{36, 1.02713, 1.02737, 1.02767, 1.02555, 1.02412},
	{37, 1.02789, 1.02813, 1.02844, 1.02632, 1.02488},
	{38, 1.02865, 1.02890, 1.02921, 1.02708, 1.02564},
	{40, 1.03017, 1.03043, 1.03076, 1.02861, 1.02715},
}

func TestSalinityToSGTable(t *testing.T) {
	cols := []struct {
		name       string
		sample, rf float64
		pick       func(sgRow) float64
	}{
		{"25/25", 25, 25, func(r sgRow) float64 { return r.sg2525 }},
		{"20/20", 20, 20, func(r sgRow) float64 { return r.sg2020 }},
		{"15/15", 15, 15, func(r sgRow) float64 { return r.sg1515 }},
		{"20/4", 20, 4, func(r sgRow) float64 { return r.sg204 }},
		{"25/4", 25, 4, func(r sgRow) float64 { return r.sg254 }},
	}
	for _, col := range cols {
		for _, r := range sgTable {
			got := SalinityToSG(r.ppt, col.sample, col.rf)
			if want := col.pick(r); math.Abs(got-want) > 6e-6 {
				t.Errorf("%s S=%v: SG = %.6f, want %.5f", col.name, r.ppt, got, want)
			}
		}
	}
}

func TestSGToSalinityTable(t *testing.T) {
	for _, r := range sgTable {
		got, err := SGToSalinity(r.sg2525, 25, 25)
		if err != nil {
			t.Fatalf("S=%v: %v", r.ppt, err)
		}
		// Table SG values are rounded to 5 decimals, which is about 0.007 ppt.
		if math.Abs(got-r.ppt) > 0.01 {
			t.Errorf("SG %.5f -> %.4f ppt, want %v", r.sg2525, got, r.ppt)
		}
	}
}

func TestSalinityRoundTrip(t *testing.T) {
	temps := [][2]float64{{25, 25}, {20, 20}, {15, 15}, {20, 4}, {25, 4}, {26, 25}}
	for _, tr := range temps {
		for s := 0.0; s <= 50; s += 0.37 {
			sg := SalinityToSG(s, tr[0], tr[1])
			back, err := SGToSalinity(sg, tr[0], tr[1])
			if err != nil {
				t.Fatalf("T=%v S=%v: %v", tr, s, err)
			}
			if math.Abs(back-s) > 1e-9 {
				t.Errorf("T=%v: %v ppt -> %v SG -> %v ppt", tr, s, sg, back)
			}
		}
	}
}

func TestSalinityMonotonicAndHobbyPairs(t *testing.T) {
	prev := 0.0
	for s := 0.0; s <= 50; s += 0.1 {
		sg := SalinityToSG(s, DefaultSampleC, DefaultReferenceC)
		if sg <= prev {
			t.Fatalf("SG not increasing at %v ppt", s)
		}
		prev = sg
	}
	// 35 ppt is 1.0264 on a 25/25 hydrometer, and a nominal 1.026 reads a little under 35 ppt.
	if got := Round(FromCanonical(Salinity, SG, 35), 4); got != 1.0264 {
		t.Errorf("35 ppt = SG %v, want 1.0264", got)
	}
	ppt, err := ToCanonical(Salinity, SG, 1.026)
	if err != nil {
		t.Fatal(err)
	}
	if ppt < 34.4 || ppt > 34.7 {
		t.Errorf("SG 1.026 = %v ppt, want about 34.5", ppt)
	}
	if ppt, err := ToCanonical(Salinity, SG, 1.0264); err != nil || math.Abs(ppt-35) > 0.05 {
		t.Errorf("SG 1.0264 = %v ppt (%v), want about 35", ppt, err)
	}
}

func TestSGToSalinityBounds(t *testing.T) {
	if ppt, err := SGToSalinity(1, 25, 25); err != nil || math.Abs(ppt) > 1e-9 {
		t.Errorf("SG 1.0 = %v, %v; want 0", ppt, err)
	}
	for _, sg := range []float64{0.9999, 0.5, 1.04, 2} {
		if _, err := SGToSalinity(sg, 25, 25); !errors.Is(err, ErrOutOfRange) {
			t.Errorf("SG %v: err = %v, want ErrOutOfRange", sg, err)
		}
	}
}

package units

import (
	"fmt"
	"math"
)

// Reference temperatures in degrees Celsius for the salinity to specific gravity conversion (sample/reference, 25/25).
const (
	DefaultSampleC    = 25.0
	DefaultReferenceC = 25.0
)

// maxSalinity bounds the inverse search; seawater above this is not a reef aquarium.
const maxSalinity = 50.0

// PureWaterDensity returns the density of pure water in kg/m3 at tC degrees Celsius (EOS-80).
func PureWaterDensity(tC float64) float64 {
	return 999.842594 + tC*(6.793952e-2+tC*(-9.095290e-3+tC*(1.001685e-4+tC*(-1.120083e-6+tC*6.536332e-9))))
}

// SeawaterDensity returns the one-atmosphere density in kg/m3 of seawater at salinity ppt and tC degrees Celsius (EOS-80).
func SeawaterDensity(ppt, tC float64) float64 {
	a := 8.24493e-1 + tC*(-4.0899e-3+tC*(7.6438e-5+tC*(-8.2467e-7+tC*5.3875e-9)))
	b := -5.72466e-3 + tC*(1.0227e-4+tC*-1.6546e-6)
	const c = 4.8314e-4
	return PureWaterDensity(tC) + a*ppt + b*ppt*math.Sqrt(ppt) + c*ppt*ppt
}

// SalinityToSG returns the specific gravity of seawater at salinity ppt measured at sampleC against pure water at referenceC.
func SalinityToSG(ppt, sampleC, referenceC float64) float64 {
	return SeawaterDensity(ppt, sampleC) / PureWaterDensity(referenceC)
}

// SGToSalinity inverts SalinityToSG by bisection and fails when sg lies outside 0 to 50 ppt.
func SGToSalinity(sg, sampleC, referenceC float64) (float64, error) {
	lo, hi := 0.0, maxSalinity
	if sg < SalinityToSG(lo, sampleC, referenceC) || sg > SalinityToSG(hi, sampleC, referenceC) {
		return 0, fmt.Errorf("specific gravity %v: %w", sg, ErrOutOfRange)
	}
	// 64 halvings of a 50 ppt interval reach float64 resolution.
	for range 64 {
		mid := (lo + hi) / 2
		if SalinityToSG(mid, sampleC, referenceC) < sg {
			lo = mid
		} else {
			hi = mid
		}
	}
	return (lo + hi) / 2, nil
}

package stats

import (
	"golang.org/x/exp/rand"
	"gonum.org/v1/gonum/stat/distuv"
	"time"
)

func SampleTruncatedNormalDistribution(lo, hi, mean, variance float64) float64 {
	// Set the random seed to the current time for sufficient uniqueness.
	randSeed := uint64(time.Now().UTC().UnixNano())

	// Use an inverse transform method to sample from the distribution.
	// Reference: https://www.r-bloggers.com/2020/08/generating-data-from-a-truncated-distribution/
	norm := distuv.Normal{
		Mu:    mean,
		Sigma: variance,
		Src:   rand.NewSource(randSeed),
	}

	a := norm.CDF(lo)
	b := norm.CDF(hi)
	u := distuv.Uniform{
		Min: a,
		Max: b,
		Src: rand.NewSource(randSeed),
	}.Rand()

	return norm.Quantile(u)
}

package onlinetraining

import (
	"golang.org/x/exp/rand"
	"time"
)

type ProbabilitySampler struct {
	random *rand.Rand
}

func NewProbabilitySampler() *ProbabilitySampler {
	// Set the random seed to the current time for sufficient uniqueness.
	randSeed := uint64(time.Now().UTC().UnixNano())

	return &ProbabilitySampler{
		random: rand.New(rand.NewSource(randSeed)),
	}
}

// Sample samples an array of probabilities.
func (s *ProbabilitySampler) Sample(n int) []float64 {
	var probs []float64

	for i := 0; i < n; i++ {
		probs = append(probs, s.random.Float64())
	}

	return probs
}

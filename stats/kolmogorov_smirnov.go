package stats

import (
	"fmt"
	"gonum.org/v1/gonum/stat"
	"log"
	"math"
	"sort"
)

type Percentile = int

const (
	P90 Percentile = iota
	P95
	P97d5
	P99
	P99d5
	P99d9
)

// coefficients are KS-coefficients.
// Retrieved from: https://www.webdepot.umontreal.ca/Usagers/angers/MonDepotPublic/STT3500H10/Critical_KS.pdf
var coefficients = map[Percentile]float64{
	P90:   1.22,
	P95:   1.36,
	P97d5: 1.48,
	P99:   1.63,
	P99d5: 1.73,
	P99d9: 1.95,
}

// KolmogorovSmirnovTestRejection performs a two-tailed KS-test, returning true
// if rejected (i.e., the distributions are different) and returning false if
// the candidate distribution belongs to the control distribution.
func KolmogorovSmirnovTestRejection(control []float64, candidate []float64, percentile Percentile) bool {
	// Calculate the KS-coefficient based on the percentile.
	coeff, ok := coefficients[percentile]
	if !ok {
		panic(fmt.Sprintf("unexpected percentile %v, see Percentile type", percentile))
	}

	// Calculate the critical value.
	criticalValue := coeff * math.Sqrt(float64(len(control)+len(candidate))/float64(len(control)*len(candidate)))

	// Copy the input slices so we can sort them.
	sortedControl := make([]float64, len(control))
	copy(sortedControl, control)
	sort.Float64s(sortedControl)

	sortedCandidate := make([]float64, len(candidate))
	copy(sortedCandidate, candidate)
	sort.Float64s(sortedCandidate)

	// Pass in nil weights as gonum's stat package allows inputs to be
	// weighted, which is not relevant to our situation.
	testStatistic := stat.KolmogorovSmirnov(sortedControl, nil, sortedCandidate, nil)
	log.Printf("test statistic: %.3f\n", testStatistic)

	return testStatistic > criticalValue
}

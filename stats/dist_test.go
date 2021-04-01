package stats

import (
	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/vg"
	"math"
	"testing"
)

func TestSampleTruncatedNormalDistribution(t *testing.T) {
	var samples []float64
	for i := 0; i < 10000; i++ {
		samples = append(samples, SampleTruncatedNormalDistribution(
			0,
			math.Inf(1),
			0, 3,
		))
	}

	p, err := plot.New()
	if err != nil {
		panic(err)
	}

	hist, err := plotter.NewHist(plotter.Values(samples), 1000)
	if err != nil {
		panic(err)
	}
	p.Add(hist)

	if err := p.Save(10*vg.Inch, 10*vg.Inch, "out/plot.png"); err != nil {
		panic(err)
	}
}

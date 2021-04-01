package responsetimecollector

import (
	"github.com/jamiealquiza/tachymeter"
	"math"
	"sync/atomic"
	"time"
)

// tachymeterCollector uses the jamiealquiza/tachymeter library to capture and
// calculate timings locally. This collector should be used where the overhead
// of local instrumentation does not affect the use case.
type tachymeterCollector struct {
	window int
	tach   *tachymeter.Tachymeter
}

func NewTachymeterCollector(window int) *tachymeterCollector {
	return &tachymeterCollector{
		window: window,
		tach: tachymeter.New(&tachymeter.Config{
			Size: window,
		}),
	}
}

func (c *tachymeterCollector) All() []float64 {
	c.tach.Lock()
	defer c.tach.Unlock()

	if atomic.LoadUint64(&c.tach.Count) == 0 {
		return []float64{}
	}

	samples := int(math.Min(float64(atomic.LoadUint64(&c.tach.Count)), float64(c.tach.Size)))
	durations := make([]time.Duration, samples)
	copy(durations, c.tach.Times[:samples])

	var durationsSeconds []float64
	for i := range durations {
		durationsSeconds = append(durationsSeconds, float64(durations[i])/float64(time.Second))
	}

	return durationsSeconds
}

func (c *tachymeterCollector) Len() int {
	return c.window
}

func (c *tachymeterCollector) Add(t time.Duration) {
	c.tach.AddTime(t)
}

func (c *tachymeterCollector) Aggregate() *Aggregation {
	aggregation := c.tach.Calc()
	return &Aggregation{
		P50: aggregation.Time.P50,
		P75: aggregation.Time.P75,
		P95: aggregation.Time.P95,
	}
}

func (c *tachymeterCollector) Reset() {
	c.tach.Reset()
}

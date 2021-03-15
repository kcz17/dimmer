package responsetimecollector

import (
	"github.com/jamiealquiza/tachymeter"
	"time"
)

// tachymeterCollector uses the jamiealquiza/tachymeter library to capture and
// calculate timings locally. This collector should be used where the overhead
// of local instrumentation does not affect the use case.
type tachymeterCollector struct {
	tach *tachymeter.Tachymeter
}

func NewTachymeterCollector(window int) *tachymeterCollector {
	return &tachymeterCollector{tach: tachymeter.New(&tachymeter.Config{
		Size: window,
	})}
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

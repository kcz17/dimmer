package response_time

import (
	"github.com/jamiealquiza/tachymeter"
	"time"
)

// tachymeterResponseTimeCollector uses the jamiealquiza/tachymeter library to
// capture and calculate timings locally. This collector should be used where
// the overhead of local instrumentation does not affect the use case.
type tachymeterResponseTimeCollector struct {
	tach *tachymeter.Tachymeter
}

func NewTachymeterResponseTimeCollector(window int) *tachymeterResponseTimeCollector {
	return &tachymeterResponseTimeCollector{tach: tachymeter.New(&tachymeter.Config{
		Size: window,
	})}
}

func (c *tachymeterResponseTimeCollector) Add(t time.Duration) {
	c.tach.AddTime(t)
}

func (c *tachymeterResponseTimeCollector) Aggregate() *ResponseTimeAggregation {
	aggregation := c.tach.Calc()
	return &ResponseTimeAggregation{
		P50: aggregation.Time.P50,
		P75: aggregation.Time.P75,
		P95: aggregation.Time.P95,
	}
}

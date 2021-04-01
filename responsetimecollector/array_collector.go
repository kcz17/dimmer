package responsetimecollector

import (
	"fmt"
	"github.com/montanaflynn/stats"
	"sync"
	"time"
)

// arrayCollector uses a struct to capture all response timings. As storage and
// computation are both O(n), this has been designed for ephemeral usage in
// training mode.
type arrayCollector struct {
	responseTimesSeconds    []float64
	responseTimesSecondsMux *sync.Mutex
}

func NewArrayCollector() *arrayCollector {
	return &arrayCollector{
		responseTimesSeconds:    []float64{},
		responseTimesSecondsMux: &sync.Mutex{},
	}
}

func (c *arrayCollector) All() []float64 {
	c.responseTimesSecondsMux.Lock()
	defer c.responseTimesSecondsMux.Unlock()
	times := make([]float64, len(c.responseTimesSeconds))
	copy(times, c.responseTimesSeconds)
	return times
}

func (c *arrayCollector) Add(t time.Duration) {
	c.responseTimesSecondsMux.Lock()
	c.responseTimesSeconds = append(c.responseTimesSeconds, float64(t)/float64(time.Second))
	c.responseTimesSecondsMux.Unlock()
}

func (c *arrayCollector) Aggregate() *Aggregation {
	// The stats package creates a copy of the array, so we must hold onto the
	// mutex while calculations are being made.
	c.responseTimesSecondsMux.Lock()
	defer c.responseTimesSecondsMux.Unlock()

	// The stats package requires input arrays to be non-empty.
	if len(c.responseTimesSeconds) == 0 {
		return &Aggregation{
			P50: 0,
			P75: 0,
			P95: 0,
		}
	}

	p50, err := stats.Median(c.responseTimesSeconds)
	if err != nil {
		panic(fmt.Errorf("unexpected err in ArrayCollector.Aggregate() while calculating p50: %w", err))
	}
	p75, err := stats.Percentile(c.responseTimesSeconds, 75)
	if err != nil {
		panic(fmt.Errorf("unexpected err in ArrayCollector.Aggregate() while calculating p75: %w", err))
	}
	p95, err := stats.Percentile(c.responseTimesSeconds, 95)
	if err != nil {
		panic(fmt.Errorf("unexpected err in ArrayCollector.Aggregate() while calculating p95: %w", err))
	}

	return &Aggregation{
		P50: time.Duration(p50 * float64(time.Second)),
		P75: time.Duration(p75 * float64(time.Second)),
		P95: time.Duration(p95 * float64(time.Second)),
	}
}

func (c *arrayCollector) Reset() {
	c.responseTimesSecondsMux.Lock()
	c.responseTimesSeconds = []float64{}
	c.responseTimesSecondsMux.Unlock()
}

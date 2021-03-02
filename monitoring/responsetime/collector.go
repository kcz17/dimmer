package responsetime

import "time"

type Aggregation struct {
	P50 time.Duration // P50 is the 50th percentile response time.
	P75 time.Duration // P75 is the 75th percentile response time.
	P95 time.Duration // P95 is the 95th percentile response time.
}

type Collector interface {
	Add(t time.Duration)     // Add sends a new response time to the collector.
	Aggregate() *Aggregation // Aggregate calculates aggregate metrics over a defined time period.
}

package profiling

import (
	"sync"
	"sync/atomic"
	"time"
)

const decayPeriod = 30 * time.Second
const decayFactor = 2

// ProfiledRequestAggregator captures data used to ensure high priority
// requests are dimmed when low priority requests are exhausted and vice-versa.
// The aggregator is implemented using atomic integers such that overhead added
// to requests is reduced. The counters decay over time to approximate a
// sliding window of requests without having to store the timestamps of
// requests.
type ProfiledRequestAggregator struct {
	lowCount  int32
	highCount int32
	// decayMux exists despite use of atomic counters due to the need to
	// synchronise decay at the same time.
	decayMux *sync.RWMutex
}

func NewProfiledRequestAggregator() *ProfiledRequestAggregator {
	a := &ProfiledRequestAggregator{
		lowCount:  0,
		highCount: 0,
	}

	go func() {
		for range time.Tick(decayPeriod) {
			a.decayMux.Lock()
			atomic.StoreInt32(&a.lowCount, atomic.LoadInt32(&a.lowCount)/decayFactor)
			atomic.StoreInt32(&a.highCount, atomic.LoadInt32(&a.highCount)/decayFactor)
			a.decayMux.Unlock()
		}
	}()

	return a
}

func (a *ProfiledRequestAggregator) MarkLowPriorityVisit() {
	atomic.AddInt32(&a.lowCount, 1)
}

func (a *ProfiledRequestAggregator) MarkHighPriorityVisit() {
	atomic.AddInt32(&a.highCount, 1)
}

func (a *ProfiledRequestAggregator) GetLowPriorityVisits() int32 {
	a.decayMux.RLock()
	defer a.decayMux.RUnlock()
	return atomic.LoadInt32(&a.lowCount)
}

func (a *ProfiledRequestAggregator) GetHighPriorityVisits() int32 {
	a.decayMux.RLock()
	defer a.decayMux.RUnlock()
	return atomic.LoadInt32(&a.highCount)
}

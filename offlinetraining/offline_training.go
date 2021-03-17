package offlinetraining

import (
	"github.com/kcz17/dimmer/responsetimecollector"
	"time"
)

// OfflineTraining represents the offline training mode. When this mode is
// enabled, all paths under RequestFilter will be dimmed according to
// PathProbabilities, regardless of the ControlLoop output.
type OfflineTraining struct {
	// responseTimeCollector allows external clients to monitor the response
	// time.
	responseTimeCollector responsetimecollector.Collector
}

func NewOfflineTraining() *OfflineTraining {
	return &OfflineTraining{
		responseTimeCollector: responsetimecollector.NewArrayCollector(),
	}
}

func (t *OfflineTraining) AddResponseTime(d time.Duration) {
	t.responseTimeCollector.Add(d)
}

func (t *OfflineTraining) GetResponseTimeMetrics() *responsetimecollector.Aggregation {
	return t.responseTimeCollector.Aggregate()
}

func (t *OfflineTraining) ResetCollector() {
	t.responseTimeCollector.Reset()
}

package serving

import (
	"errors"
	"fmt"
	"github.com/kcz17/dimmer/internal/monitoring/responsetime"
	"github.com/kcz17/dimmer/internal/serving/controller"
	"github.com/kcz17/dimmer/internal/serving/logging"
	"sync"
	"time"
)

// Percentiles the developer can choose as response time input.
const (
	P50 = "p50"
	P75 = "p75"
	P95 = "p95"
)

type DimmerControlLoop struct {
	pid *controller.PIDController
	// responseTimeCollector calculates the input to the PID controller.
	responseTimeCollector responsetime.Collector
	// responseTimePercentile is the response time percentile the dimmer will
	// pass to the PID controller as input.
	responseTimePercentile string
	logger                 logging.Logger
	// dimmingPercentage is the output of the PID controller, protected from
	// race conditions by dimmingPercentageMux.
	dimmingPercentage    float64
	dimmingPercentageMux *sync.RWMutex
	// loopWG allows the spawned goroutine to be gracefully stopped.
	loopStarted bool
	loopWG      *sync.WaitGroup
	loopStop    chan bool
}

// StartNewDimmerControlLoop spawns a new goroutine with the control loop.
func StartNewDimmerControlLoop(
	pid *controller.PIDController,
	responseTimeCollector responsetime.Collector,
	responseTimePercentile string,
	logger logging.Logger,
) (*DimmerControlLoop, error) {
	if responseTimePercentile != P50 &&
		responseTimePercentile != P75 &&
		responseTimePercentile != P95 {
		return nil, errors.New(fmt.Sprintf("StartNewDimmerControlLoop() expected responseTimePercentile to be one of {p50|p75|p95}; got %s", responseTimePercentile))
	}

	c := &DimmerControlLoop{
		pid:                    pid,
		responseTimeCollector:  responseTimeCollector,
		responseTimePercentile: responseTimePercentile,
		logger:                 logger,
		dimmingPercentage:      0.0,
		dimmingPercentageMux:   &sync.RWMutex{},
		loopStarted:            false,
	}
	c.Restart()

	return c, nil
}

// ReadDimmingPercentage retrieves the output of the PID controller as a value
// between 0 and 100 (subject to PID controller min/max parameters).
func (c *DimmerControlLoop) ReadDimmingPercentage() float64 {
	// A mutex is used to ensure no race conditions occur as the control loop
	// runs and overwrites the dimming percentage.
	c.dimmingPercentageMux.RLock()
	dimmingPercentage := c.dimmingPercentage
	c.dimmingPercentageMux.RUnlock()
	return dimmingPercentage
}

// AddResponseTime adds a new response time to the response time collector,
// likely changing the input at the next control loop.
func (c *DimmerControlLoop) AddResponseTime(t time.Duration) {
	c.responseTimeCollector.Add(t)
}

func (c *DimmerControlLoop) controlLoop() {
	ticker := time.NewTicker(time.Second * 1)
	defer ticker.Stop()
	defer c.loopWG.Done()
	for {
		select {
		case <-ticker.C:
			aggregation := c.responseTimeCollector.Aggregate()

			// PID controller and logger operate with seconds.
			p50 := float64(aggregation.P50) / float64(time.Second)
			p75 := float64(aggregation.P75) / float64(time.Second)
			p95 := float64(aggregation.P95) / float64(time.Second)
			c.logger.LogAggregateResponseTimes(p50, p75, p95)

			// Retrieve the PID output.
			var pidOutput float64
			if c.responseTimePercentile == P50 {
				pidOutput = c.pid.Output(p50)
			} else if c.responseTimePercentile == P75 {
				pidOutput = c.pid.Output(p75)
			} else if c.responseTimePercentile == P95 {
				pidOutput = c.pid.Output(p95)
			} else {
				panic(fmt.Sprintf("DimmerControlLoop.controlLoop() expected responseTimePercentile to be one of {50|75|95}; got %s", c.responseTimePercentile))
			}
			c.logger.LogDimmerOutput(pidOutput)
			c.logger.LogPIDControllerState(c.pid.DebugP, c.pid.DebugI, c.pid.DebugD, c.pid.DebugErr)

			// Apply the PID output.
			c.dimmingPercentageMux.Lock()
			c.dimmingPercentage = pidOutput
			c.dimmingPercentageMux.Unlock()
		case <-c.loopStop:
			return
		}

	}
}

func (c *DimmerControlLoop) Restart() {
	// Reset the control loop, response time collector and PID controller
	// in this order to ensure stale data is not written between each reset.
	if c.loopStarted {
		close(c.loopStop)
		c.loopWG.Wait()
		// Replace the mutex as we are unsure whether the consumers will exit
		// gracefully (i.e., the lock is still acquired by a listener while the
		// server is killed).
		c.dimmingPercentageMux = &sync.RWMutex{}

		c.responseTimeCollector.Reset()
		c.pid.Reset()
	}

	c.loopStop = make(chan bool, 1)
	c.loopWG = &sync.WaitGroup{}

	c.loopWG.Add(1)
	go c.controlLoop()
}

package serving

import (
	"errors"
	"fmt"
	"github.com/kcz17/dimmer/logging"
	"github.com/kcz17/dimmer/monitoring/responsetime"
	"github.com/kcz17/dimmer/pidcontroller"
	"sync"
	"time"
)

// Percentiles the developer can choose as response time input.
const (
	P50 = "p50"
	P75 = "p75"
	P95 = "p95"
)

type ServerControlLoop struct {
	pid *pidcontroller.PIDController
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

// NewServerControlLoop initialises the control loop.
func NewServerControlLoop(
	pid *pidcontroller.PIDController,
	responseTimeCollector responsetime.Collector,
	responseTimePercentile string,
	logger logging.Logger,
) (*ServerControlLoop, error) {
	if responseTimePercentile != P50 &&
		responseTimePercentile != P75 &&
		responseTimePercentile != P95 {
		return nil, errors.New(fmt.Sprintf("NewServerControlLoop() expected responseTimePercentile to be one of {p50|p75|p95}; got %s", responseTimePercentile))
	}

	c := &ServerControlLoop{
		pid:                    pid,
		responseTimeCollector:  responseTimeCollector,
		responseTimePercentile: responseTimePercentile,
		logger:                 logger,
		dimmingPercentage:      0.0,
		dimmingPercentageMux:   &sync.RWMutex{},
	}

	return c, nil
}

func (c *ServerControlLoop) mustStart() {
	if c.loopStarted {
		panic("control loop already started")
	}

	c.loopStop = make(chan bool, 1)
	c.loopWG = &sync.WaitGroup{}
	c.loopWG.Add(1)
	go c.controlLoop()

	c.loopStarted = true
}

func (c *ServerControlLoop) mustStop() {
	if !c.loopStarted {
		panic("control loop not yet started")
	}

	// Reset the control loop, response time collector and PID controller
	// in this order to ensure stale data is not written between each reset.
	close(c.loopStop)
	c.loopWG.Wait()
	c.responseTimeCollector.Reset()
	c.pid.Reset()

	c.dimmingPercentageMux.Lock()
	c.dimmingPercentage = 0.0
	c.dimmingPercentageMux.Unlock()

	c.loopStarted = false
}

// readDimmingPercentage retrieves the output of the PID controller as a value
// between 0 and 100 (subject to PID controller min/max parameters).
func (c *ServerControlLoop) readDimmingPercentage() float64 {
	// A mutex is used to ensure no race conditions occur as the control loop
	// runs and overwrites the dimming percentage.
	c.dimmingPercentageMux.RLock()
	defer c.dimmingPercentageMux.RUnlock()
	return c.dimmingPercentage
}

// addResponseTime adds a new response time to the response time collector,
// likely changing the input at the next control loop.
func (c *ServerControlLoop) addResponseTime(t time.Duration) {
	c.responseTimeCollector.Add(t)
}

func (c *ServerControlLoop) controlLoop() {
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
				panic(fmt.Sprintf("ServerControlLoop.controlLoop() expected responseTimePercentile to be one of {50|75|95}; got %s", c.responseTimePercentile))
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

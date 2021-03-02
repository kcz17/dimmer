package main

import (
	"errors"
	"fmt"
	"github.com/kcz17/dimmer/controller"
	"github.com/kcz17/dimmer/logging"
	"github.com/kcz17/dimmer/monitoring/response_time"
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
	responseTimeCollector response_time.ResponseTimeCollector
	// responseTimePercentile is the response time percentile the dimmer will
	// pass to the PID controller as input.
	responseTimePercentile string
	logger                 logging.Logger
	// dimmingPercentage is the output of the PID controller, protected from
	// race conditions by dimmingPercentageMux.
	dimmingPercentage    float64
	dimmingPercentageMux *sync.RWMutex
}

func StartNewDimmerControlLoop(
	pid *controller.PIDController,
	responseTimeCollector response_time.ResponseTimeCollector,
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
	}
	go c.controlLoop()

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

func (c *DimmerControlLoop) controlLoop() {
	for range time.Tick(time.Second * 1) {
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
	}
}

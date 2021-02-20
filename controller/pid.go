package controller

import (
	"errors"
	"time"
)

type PIDController struct {
	clock         Clock     // Used to read the current time in a testable manner.
	setpoint      float64   // Setpoint for the PID to aim to achieve.
	kp            float64   // Proportional gain constant.
	ki            float64   // Integral gain constant.
	kd            float64   // Differential gain constant.
	minOutput     float64   // Output will never go below lower bound.
	maxOutput     float64   // Output will never go above upper bound.
	minSampleTime float64   // Output will not change before minSampleTime is elapsed.
	lastOutput    float64   // If minSampleTime has not yet elapsed, this will be the output.
	lastTick      time.Time // Used to scale differential and integral terms and to enforce minSampleTime.
	lastInput     float64   // Used to calculate the differential term.
	integral      float64   // Running integral term for PID calculation.
}

func NewPIDController(clock Clock, setpoint float64, kp float64, ki float64, kd float64, isReversed bool, minOutput float64, maxOutput float64, minSampleTime float64) (*PIDController, error) {
	if kp < 0 || ki < 0 || kd < 0 {
		return nil, errors.New("expected positive controller parameters; got negative (toggle isReversed instead)")
	}

	// If reversed, then a positive error (setpoint - input) will decrease
	// the control output.
	if isReversed {
		kp = -kp
		ki = -ki
		kd = -kd
	}

	return &PIDController{
		clock:         clock,
		setpoint:      setpoint,
		kp:            kp,
		ki:            ki,
		kd:            kd,
		minOutput:     minOutput,
		maxOutput:     maxOutput,
		minSampleTime: minSampleTime,
	}, nil
}

func (c *PIDController) Output(input float64) float64 {
	now := c.clock.Now()

	// The elapsed time > 0 only once a control loop has been made.
	var elapsed float64
	if !c.lastTick.IsZero() {
		elapsed = now.Sub(c.lastTick).Seconds()
		if elapsed < c.minSampleTime {
			// Ensure the control loop is called after the minimum sample time has passed.
			return c.lastOutput
		}
	}

	// Calculate PID terms.
	err := c.setpoint - input
	p := c.kp * err

	c.integral += c.ki * err * elapsed

	var d float64
	if elapsed != 0 {
		d = c.kd * -((input - c.lastInput) / elapsed)
	}

	output := p + c.integral + d
	if output > c.maxOutput {
		output = c.maxOutput
	} else if output < c.minOutput {
		output = c.minOutput
	}

	// Save calculations for the next loop.
	c.lastTick = now
	c.lastInput = input
	c.lastOutput = output

	return output
}

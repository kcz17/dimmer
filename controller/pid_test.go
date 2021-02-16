package controller

import (
	"gonum.org/v1/plot/plotter"
	"testing"
	"time"

	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotutil"
	"gonum.org/v1/plot/vg"
)

// simulatedClock provides us control over the exact time and seconds to advance by.
type simulatedClock struct {
	t time.Time
}

func newSimulatedClock() *simulatedClock {
	return &simulatedClock{t: time.Date(2020, 1, 1, 12, 0, 0, 0, time.UTC)}
}

func (c *simulatedClock) Now() time.Time { return c.t }

func (c *simulatedClock) advance(seconds int) {
	c.t = c.t.Add(time.Second * time.Duration(seconds))
}

// Simple simulation of a water boiler where heat dissipates slowly over time.
// Based on https://github.com/m-lundberg/simple-pid/blob/master/examples/water_boiler/water_boiler.py.
type waterBoiler struct {
	temp float64
}

func newWaterBoiler() *waterBoiler {
	return &waterBoiler{temp: 20}
}

func (b *waterBoiler) advance(powerDuringTimeElapsed float64, secondsElapsed int) {
	// Produce heat over the elapsed seconds only if boiler has power.
	if powerDuringTimeElapsed > 0 {
		b.temp += powerDuringTimeElapsed * float64(secondsElapsed)
	}

	// Dissipate heat over the elapsed seconds.
	b.temp -= 0.02 * float64(secondsElapsed)
}

// Basic integration test over a simulated period of time.
func TestPidController_WaterBoilerSimulation(t *testing.T) {
	clock := newSimulatedClock()
	boiler := newWaterBoiler()
	controller := NewPIDController(
		clock,
		60,
		0.05,
		0,
		0,
		0,
		100,
		0.5,
	)

	loops := 300
	secondsPerIteration := 5
	times := make([]int, loops)
	temps := make([]float64, loops)
	powers := make([]float64, loops)
	for i := 0; i < loops; i++ {
		power := controller.Output(boiler.temp)
		t.Logf("Current temperature: %.3f | New boiler power: %.3f\n", boiler.temp, power)
		times[i] = i * secondsPerIteration
		temps[i] = boiler.temp
		powers[i] = power

		// Set the power and play out the scenario over one second.
		clock.advance(secondsPerIteration)
		boiler.advance(power, secondsPerIteration)
	}

	// Plot the results
	p, err := plot.New()
	if err != nil {
		panic(err)
	}

	err = plotutil.AddLinePoints(p,
		"Temperatures", toPlotterXYs(times, temps),
		"Controller Outputs (Power)", toPlotterXYs(times, powers),
	)
	if err != nil {
		panic(err)
	}

	p.Y.Min = -10
	p.Y.Max = 120

	// Save the plot to a PNG file.
	if err := p.Save(10*vg.Inch, 10*vg.Inch, "out/points.png"); err != nil {
		panic(err)
	}
}

func toPlotterXYs(x []int, y []float64) plotter.XYs {
	points := make(plotter.XYs, len(x))
	for i := range points {
		points[i].X = float64(x[i])
		points[i].Y = y[i]
	}
	return points
}

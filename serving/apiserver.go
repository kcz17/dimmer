package serving

import (
	"encoding/json"
	"fmt"
	"github.com/jackwhelpton/fasthttp-routing/v2"
	"github.com/kcz17/dimmer/filters"
	"github.com/valyala/fasthttp"
	"time"
)

type APIServer struct {
	Server *Server
}

func (a *APIServer) ListenAndServe(addr string) error {
	router := routing.New()

	router.Post("/reset", a.resetControlLoopHandler())

	router.Get("/probabilities", a.listPathProbabilitiesHandler())
	router.Post("/probabilities", a.setPathProbabilitiesHandler())
	router.Delete("/probabilities", a.clearPathProbabilitiesHandler())

	router.Post("/collector", a.startResponseTimeCollectorHandler())
	router.Delete("/collector", a.stopResponseTimeCollectorHandler())
	router.Get("/collector", a.responseTimeCollectorStatsHandler())

	return fasthttp.ListenAndServe(addr, router.HandleRequest)
}

func (a *APIServer) resetControlLoopHandler() routing.Handler {
	return func(c *routing.Context) error {
		if err := a.Server.ResetControlLoop(); err != nil {
			return fmt.Errorf("could not reset control loop: err = %w\n", err)
		}

		return c.Write("server control loop reset\n")
	}
}

func (a *APIServer) listPathProbabilitiesHandler() routing.Handler {
	return func(c *routing.Context) error {
		return c.Write(fmt.Sprintf("probabilities:\n%v\n", a.Server.dimming.PathProbabilities.List()))
	}
}

func (a *APIServer) setPathProbabilitiesHandler() routing.Handler {
	return func(c *routing.Context) error {
		var probabilities []filters.PathProbabilityRule
		if err := c.Read(&probabilities); err != nil {
			return err
		}

		if err := a.Server.dimming.PathProbabilities.SetAll(probabilities); err != nil {
			return fmt.Errorf("could not set probabilities: err = %w\n", err)
		}

		return c.Write("probabilities written\n")
	}
}

func (a *APIServer) clearPathProbabilitiesHandler() routing.Handler {
	return func(c *routing.Context) error {
		a.Server.dimming.PathProbabilities.Clear()
		return c.Write(fmt.Sprintf("probabilities cleared\n"))
	}
}

func (a *APIServer) startResponseTimeCollectorHandler() routing.Handler {
	return func(c *routing.Context) error {
		a.Server.StartExtResponseTimeCollector()
		return c.Write(fmt.Sprintf("started\n"))
	}
}

func (a *APIServer) stopResponseTimeCollectorHandler() routing.Handler {
	return func(c *routing.Context) error {
		a.Server.StopExtResponseTimeCollector()
		return c.Write(fmt.Sprintf("stopped\n"))
	}
}

func (a *APIServer) responseTimeCollectorStatsHandler() routing.Handler {
	return func(c *routing.Context) error {
		aggregation := a.Server.offlineTraining.ExtResponseTimeCollector.Aggregate()
		response := &struct {
			P50 float64
			P75 float64
			P95 float64
		}{
			P50: float64(aggregation.P50) / float64(time.Second),
			P75: float64(aggregation.P75) / float64(time.Second),
			P95: float64(aggregation.P95) / float64(time.Second),
		}

		b, err := json.Marshal(response)
		if err != nil {
			return fmt.Errorf("could not marshal aggregation: err = %w", err)
		}
		return c.Write(b)
	}
}

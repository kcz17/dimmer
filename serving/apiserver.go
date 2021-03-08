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

	router.Post("/start", a.startHandler())
	router.Post("/stop", a.stopHandler())

	router.Get("/probabilities", a.listPathProbabilitiesHandler())
	router.Post("/probabilities", a.setPathProbabilitiesHandler())
	router.Delete("/probabilities", a.clearPathProbabilitiesHandler())

	router.Post("/collector", a.startResponseTimeCollectorHandler())
	router.Delete("/collector", a.stopResponseTimeCollectorHandler())
	router.Get("/collector", a.responseTimeCollectorStatsHandler())

	return fasthttp.ListenAndServe(addr, router.HandleRequest)
}

func (a *APIServer) startHandler() routing.Handler {
	return func(c *routing.Context) error {
		if err := a.Server.Start(); err != nil {
			return fmt.Errorf("could not start server: err = %w\n", err)
		}

		return c.Write("server started\n")
	}
}

func (a *APIServer) stopHandler() routing.Handler {
	return func(c *routing.Context) error {
		if err := a.Server.Stop(); err != nil {
			return fmt.Errorf("could not stop server: err = %w\n", err)
		}

		return c.Write("server stopped\n")
	}
}

func (a *APIServer) listPathProbabilitiesHandler() routing.Handler {
	return func(c *routing.Context) error {
		return c.Write(fmt.Sprintf("probabilities:\n%v\n", a.Server.PathProbabilities.List()))
	}
}

func (a *APIServer) setPathProbabilitiesHandler() routing.Handler {
	return func(c *routing.Context) error {
		var probabilities []filters.PathProbabilityRule
		if err := c.Read(&probabilities); err != nil {
			return err
		}

		if err := a.Server.PathProbabilities.SetAll(probabilities); err != nil {
			return fmt.Errorf("could not set probabilities: err = %w\n", err)
		}

		return c.Write("probabilities written\n")
	}
}

func (a *APIServer) clearPathProbabilitiesHandler() routing.Handler {
	return func(c *routing.Context) error {
		a.Server.PathProbabilities.Clear()
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
		aggregation := a.Server.ExtResponseTimeCollector.Aggregate()
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

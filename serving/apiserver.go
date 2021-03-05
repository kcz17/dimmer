package serving

import (
	"fmt"
	"github.com/jackwhelpton/fasthttp-routing/v2"
	"github.com/kcz17/dimmer/filters"
	"github.com/valyala/fasthttp"
)

type APIServer struct {
	Server *Server
}

func (a *APIServer) ListenAndServe(addr string) error {
	router := routing.New()

	router.Post("/start", a.startHandler())
	router.Post("/stop", a.stopHandler())
	router.Post("/probabilities", a.setPathProbabilitiesHandler())

	return fasthttp.ListenAndServe(addr, router.HandleRequest)
}

func (a *APIServer) startHandler() routing.Handler {
	return func(c *routing.Context) error {
		if err := a.Server.Start(); err != nil {
			return fmt.Errorf("could not start server: err = %w\n", err)
		}

		_ = c.Write("server started")
		return nil
	}
}

func (a *APIServer) stopHandler() routing.Handler {
	return func(c *routing.Context) error {
		if err := a.Server.Stop(); err != nil {
			return fmt.Errorf("could not stop server: err = %w\n", err)
		}

		_ = c.Write("server stopped")
		return nil
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

		_ = c.Write("probabilities written")
		return nil
	}
}

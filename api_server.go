package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackwhelpton/fasthttp-routing/v2"
	"github.com/kcz17/dimmer/filters"
	"github.com/valyala/fasthttp"
	"time"
)

type APIServer struct {
	Server *Server
}

func (s *APIServer) ListenAndServe(addr string) error {
	router := routing.New()

	router.Post("/mode", s.setServerModeHandler())

	router.Get("/probabilities", s.listPathProbabilitiesHandler())
	router.Post("/probabilities", s.setPathProbabilitiesHandler())
	router.Delete("/probabilities", s.clearPathProbabilitiesHandler())

	return fasthttp.ListenAndServe(addr, router.HandleRequest)
}

func (s *APIServer) setServerModeHandler() routing.Handler {
	return func(c *routing.Context) error {
		mode := &struct {
			Mode string
		}{}
		if err := c.Read(&mode); err != nil {
			return fmt.Errorf("could not parse body: %w", err)
		}

		var err error
		switch mode.Mode {
		case "Default":
			err = s.Server.SetDimmingMode(s.Server.defaultDimmingMode)
		case "Disabled":
			err = s.Server.SetDimmingMode(Disabled)
			break
		case "OfflineTraining":
			err = s.Server.SetDimmingMode(OfflineTraining)
			break
		case "Dimming":
			err = s.Server.SetDimmingMode(Dimming)
			break
		case "DimmingWithOnlineTraining":
			err = s.Server.SetDimmingMode(DimmingWithOnlineTraining)
			break
		default:
			err = errors.New("mode must be one of {Default|Disabled|OfflineTraining|Dimming|DimmingWithOnlineTraining}")
			break
		}
		if err != nil {
			return err
		}

		return c.Write("mode set\n")
	}
}

func (s *APIServer) getOfflineTrainingStatsHandler() routing.Handler {
	return func(c *routing.Context) error {
		aggregation := s.Server.offlineTraining.GetResponseTimeMetrics()
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

func (s *APIServer) listPathProbabilitiesHandler() routing.Handler {
	return func(c *routing.Context) error {
		return c.Write(fmt.Sprintf("probabilities:\n%v\n", s.Server.dimming.PathProbabilities.List()))
	}
}

func (s *APIServer) setPathProbabilitiesHandler() routing.Handler {
	return func(c *routing.Context) error {
		var probabilities []filters.PathProbabilityRule
		if err := c.Read(&probabilities); err != nil {
			return err
		}

		if err := s.Server.UpdatePathProbabilities(probabilities); err != nil {
			return err
		}

		return c.Write("probabilities written\n")
	}
}

func (s *APIServer) clearPathProbabilitiesHandler() routing.Handler {
	return func(c *routing.Context) error {
		s.Server.dimming.PathProbabilities.Clear()
		return c.Write(fmt.Sprintf("probabilities cleared\n"))
	}
}

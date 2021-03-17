package main

import (
	"encoding/json"
	"fmt"
	"github.com/jackwhelpton/fasthttp-routing/v2"
	"github.com/kcz17/dimmer/filters"
	"github.com/valyala/fasthttp"
	"time"
)

type OfflineTrainingAPIServer struct {
	Server *Server
}

func (s *OfflineTrainingAPIServer) ListenAndServe(addr string) error {
	router := routing.New()

	router.Get("/training/stats", s.getTrainingModeStatsHandler())
	router.Post("/training", s.startTrainingModeHandler())
	router.Delete("/training", s.stopTrainingModeHandler())

	router.Get("/probabilities", s.listPathProbabilitiesHandler())
	router.Post("/probabilities", s.setPathProbabilitiesHandler())
	router.Delete("/probabilities", s.clearPathProbabilitiesHandler())

	return fasthttp.ListenAndServe(addr, router.HandleRequest)
}

func (s *OfflineTrainingAPIServer) getTrainingModeStatsHandler() routing.Handler {
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

func (s *OfflineTrainingAPIServer) startTrainingModeHandler() routing.Handler {
	return func(c *routing.Context) error {
		if err := s.Server.SetDimmingMode(DimmingWithOnlineTraining); err != nil {
			return fmt.Errorf("could not start offline training mode: err = %w\n", err)
		}

		return c.Write("offline training mode started\n")
	}
}

func (s *OfflineTrainingAPIServer) stopTrainingModeHandler() routing.Handler {
	return func(c *routing.Context) error {
		if err := s.Server.SetDimmingMode(s.Server.DefaultDimmingMode()); err != nil {
			return fmt.Errorf("could not stop offline training mode: err = %w\n", err)
		}

		return c.Write("offline training mode stopped\n")
	}
}

func (s *OfflineTrainingAPIServer) listPathProbabilitiesHandler() routing.Handler {
	return func(c *routing.Context) error {
		return c.Write(fmt.Sprintf("probabilities:\n%v\n", s.Server.dimming.PathProbabilities.List()))
	}
}

func (s *OfflineTrainingAPIServer) setPathProbabilitiesHandler() routing.Handler {
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

func (s *OfflineTrainingAPIServer) clearPathProbabilitiesHandler() routing.Handler {
	return func(c *routing.Context) error {
		s.Server.dimming.PathProbabilities.Clear()
		return c.Write(fmt.Sprintf("probabilities cleared\n"))
	}
}

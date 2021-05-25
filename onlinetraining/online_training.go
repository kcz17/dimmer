package onlinetraining

import (
	"errors"
	"fmt"
	"github.com/kcz17/dimmer/filters"
	"github.com/kcz17/dimmer/logging"
	"github.com/kcz17/dimmer/responsetimecollector"
	"github.com/kcz17/dimmer/stats"
	"github.com/valyala/fasthttp"
	"log"
	"math/rand"
	"strings"
	"sync"
	"time"
)

const onlineTrainingCookieKey = "ONLINE_TRAINING"
const onlineTrainingCookieControl = "CONTROL"
const onlineTrainingCookieCandidate = "CANDIDATE"
const onlineTrainingCookieCandidateProbability = 0.05

type OnlineTraining struct {
	logger                      logging.Logger
	controlGroupResponseTimes   responsetimecollector.Collector
	candidateGroupResponseTimes responsetimecollector.Collector
	candidatePathProbabilities  *filters.PathProbabilities
	paths                       []string
	lastPathIndexSampled        int
	// controlPathProbabilities is a pointer to the main ("control") group
	// of path probabilities applied to the majority of requests under Server.
	controlPathProbabilities *filters.PathProbabilities
	// mux protects fields from race conditions.
	mux *sync.Mutex

	// loopStarted is used so the control loop can be started and stopped.
	loopStarted bool
	// As trainingLoop runs in a goroutine, loopWaiter and loopStop allow the
	// spawned goroutine to be gracefully stopped.
	loopWaiter *sync.WaitGroup
	loopStop   chan bool
}

func NewOnlineTraining(logger logging.Logger, paths []string, controlPathProbabilities *filters.PathProbabilities, defaultPathProbability float64) (*OnlineTraining, error) {
	candidatePathProbabilities, err := filters.NewPathProbabilities(defaultPathProbability)
	if err != nil {
		return nil, fmt.Errorf("expected filters.NewPathProbabilities() returns nil err; got err = %w", err)
	}

	for _, path := range paths {
		if err := candidatePathProbabilities.Set(filters.PathProbabilityRule{
			Path:        path,
			Probability: controlPathProbabilities.Get(path),
		}); err != nil {
			return nil, fmt.Errorf("expected initial candidate probabilities setting returns nil err; got err = %w", err)
		}
	}

	return &OnlineTraining{
		logger:                      logger,
		controlGroupResponseTimes:   responsetimecollector.NewTachymeterCollector(2000),
		candidateGroupResponseTimes: responsetimecollector.NewArrayCollector(),
		candidatePathProbabilities:  candidatePathProbabilities,
		paths:                       paths,
		lastPathIndexSampled:        len(paths) - 1,
		controlPathProbabilities:    controlPathProbabilities,
		mux:                         &sync.Mutex{},
	}, nil
}

func (t *OnlineTraining) StartLoop() error {
	if t.loopStarted {
		return errors.New("OnlineTrainingLoop.Start() failed: training loop already started")
	}

	t.loopStop = make(chan bool, 1)
	t.loopWaiter = &sync.WaitGroup{}
	t.loopWaiter.Add(1)
	go t.trainingLoop()

	t.loopStarted = true
	return nil
}

func (t *OnlineTraining) StopLoop() error {
	if !t.loopStarted {
		return errors.New("OnlineTrainingLoop.Stop() failed: training loop not running")
	}

	// ResetCollector the control loop, response time collector and PID controller
	// in this order to ensure stale data is not written between each reset.
	close(t.loopStop)
	t.loopWaiter.Wait()
	t.candidateGroupResponseTimes.Reset()
	t.controlGroupResponseTimes.Reset()

	t.loopStarted = false
	return nil
}

func (t *OnlineTraining) trainingLoop() {
	defer t.loopWaiter.Done()

	// Used to ensure the controller responds to changes in PID values before
	// continuing with another training loop. Initially set to true to allow
	// the controller to react to a new load test.
	isInAdjustmentPeriod := true

	for {
		select {
		// Stop the control loop when Stop() called.
		case <-t.loopStop:
			return
		default:
			if isInAdjustmentPeriod {
				isInAdjustmentPeriod = false
				continue
			}

			// Sample new rules.
			newCandidateRules := t.sampleCandidateGroupProbabilities()
			t.candidatePathProbabilities.Clear()
			if err := t.candidatePathProbabilities.SetAll(newCandidateRules); err != nil {
				panic(fmt.Errorf("expected t.candidatePathProbabilities.SetAll(rules = %+v) returns nil err; got err = %w", newCandidateRules, err))
			}
			log.Printf("[Online Testing] starting test with candidate rules: %+v\n", newCandidateRules)
			t.logger.LogOnlineTrainingProbabilities(
				t.controlPathProbabilities.ListForPaths(t.paths),
				t.candidatePathProbabilities.ListForPaths(t.paths),
			)

			t.candidateGroupResponseTimes.Reset()
			t.controlGroupResponseTimes.Reset()

			// Wait for enough data to be collected while continuing to listen for
			// Stop() in a non-blocking manner.
			select {
			case <-t.loopStop:
				return
			case <-time.After(2 * time.Minute):
				break
			}

			// Test whether the rules collected are significant, overriding the
			// main path probabilities if so.
			comparison := t.checkCandidateImprovesResponseTimes()
			log.Printf(
				"[Online Testing] finished test with %d candidate response times collected for candidate rules: %+v\n",
				t.candidateGroupResponseTimes.Len(),
				newCandidateRules,
			)
			log.Printf("[Online Testing] significant reduction? %t\n", comparison)
			if comparison {
				log.Printf("[Online Testing] updating control with candidate rules\n")
				if err := t.controlPathProbabilities.SetAll(newCandidateRules); err != nil {
					panic(fmt.Errorf("expected t.controlPathProbabilities.SetAll(rules = %+v) returns nil err; got err = %w", newCandidateRules, err))
				}
				isInAdjustmentPeriod = true
			}
		}
	}
}

func (t *OnlineTraining) SetPaths(paths []string) {
	t.mux.Lock()
	t.paths = paths
	t.mux.Unlock()
}

func (t *OnlineTraining) SampleCandidateGroupShouldDim(path string) bool {
	return t.candidatePathProbabilities.SampleShouldDim(path)
}

func (t *OnlineTraining) AddCandidateResponseTime(duration time.Duration) {
	t.candidateGroupResponseTimes.Add(duration)
}

func (t *OnlineTraining) AddControlResponseTime(duration time.Duration) {
	t.controlGroupResponseTimes.Add(duration)
}

func (t *OnlineTraining) sampleCandidateGroupProbabilities() []filters.PathProbabilityRule {
	t.mux.Lock()
	defer t.mux.Unlock()

	// Sample a set of probabilities for rules using random optimisation with
	// a normal distribution, setting the mean to be the current path
	// probability. The variance is set to 0.5 based on empirical observations.
	variance := 0.5

	nextIndexToSample := (t.lastPathIndexSampled + 1) % len(t.paths)

	var rules []filters.PathProbabilityRule
	for i, path := range t.paths {
		var probability float64
		if i == nextIndexToSample {
			probability = stats.SampleTruncatedNormalDistribution(
				0,
				1,
				t.candidatePathProbabilities.Get(path),
				variance,
			)
		} else {
			probability = t.candidatePathProbabilities.Get(path)
		}

		rules = append(rules, filters.PathProbabilityRule{
			Path:        path,
			Probability: probability,
		})
	}

	t.lastPathIndexSampled = nextIndexToSample
	return rules
}

func (t *OnlineTraining) checkCandidateImprovesResponseTimes() bool {
	controlAggregate := t.controlGroupResponseTimes.Aggregate()
	candidateAggregate := t.candidateGroupResponseTimes.Aggregate()

	controlP95 := float64(controlAggregate.P95) / float64(time.Second)
	candidateP95 := float64(candidateAggregate.P95) / float64(time.Second)
	log.Printf("[Online Testing] control p95: %.3f, candidate p95: %.3f\n", controlP95, candidateP95)

	// Use a heuristic based on whether the P95 > 50ms to determine whether
	// enough data has been collected and a significant change is possible.
	candidateCollectedEnoughData := candidateP95 > 0.05
	if !candidateCollectedEnoughData {
		log.Printf("candidate p95 does not have enough data\n")
		return false
	}

	// The candidate P95 must be significantly lower than the control P95 for
	// there to be a potential improvement in response times.
	if 0.85*controlP95 <= candidateP95 {
		return false
	}

	// Test whether there is a significant change in response time distributions
	// by performing a Kolmogorov-Smirnov test at the 99th percentile. The 99th
	// percentile has been chosen based on empirical tests where the 99.5th
	// percentile is overly sensitive.
	controlAll := t.controlGroupResponseTimes.All()
	candidateAll := t.candidateGroupResponseTimes.All()
	return stats.KolmogorovSmirnovTestRejection(controlAll, candidateAll, stats.P99d5)
}

func RequestHasCookie(request *fasthttp.Request) bool {
	return len(request.Header.Cookie(onlineTrainingCookieKey)) != 0
}

func RequestHasCandidateCookie(request *fasthttp.Request) bool {
	return strings.Compare(onlineTrainingCookieCandidate,
		string(request.Header.Cookie(onlineTrainingCookieKey))) == 0
}

func SampleCookie() *fasthttp.Cookie {
	if rand.Float64() < onlineTrainingCookieCandidateProbability {
		return candidateCookie()
	} else {
		return controlCookie()
	}
}

func controlCookie() *fasthttp.Cookie {
	cookie := &fasthttp.Cookie{}
	cookie.SetKey(onlineTrainingCookieKey)
	cookie.SetValue(onlineTrainingCookieControl)
	return cookie
}

func candidateCookie() *fasthttp.Cookie {
	cookie := &fasthttp.Cookie{}
	cookie.SetKey(onlineTrainingCookieKey)
	cookie.SetValue(onlineTrainingCookieCandidate)
	return cookie
}

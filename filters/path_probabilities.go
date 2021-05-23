package filters

import (
	"errors"
	"fmt"
	"math/rand"
	"sync"
)

// PathProbabilities stores per-path probabilities which affect how each path
// is dimmed.
//
// A key invariant is that Get operations must be insensitive of a path's
// leading slash. To keep Get lookup O(1), Set is responsible for O(n) string
// operations which add both leading slash inclusive and exclusive paths to the
// map, enabling O(1) Get lookup.
type PathProbabilities struct {
	// probabilities is a map from a path to a probability. Paths must be
	// inserted with and without their leading slash to allow the leading-
	// slash-insensitive Get operation to work without string manipulation.
	probabilities map[string]float64
	// probabilitiesMux guards probabilities from concurrent reads and writes,
	// where reads occur while requests are being served and writes can be
	// made at runtime.
	probabilitiesMux *sync.RWMutex
	// defaultValue is the value returned to the user if a path does not exist
	// in the map.
	defaultValue float64
}

type PathProbabilityRule struct {
	Path        string
	Probability float64
}

func NewPathProbabilities(defaultValue float64) (*PathProbabilities, error) {
	if defaultValue < 0 || defaultValue > 1 {
		return nil, errors.New(fmt.Sprintf("NewPathProbabilities() expected defaultValue between 0 and 1; got probability = %v", defaultValue))
	}

	return &PathProbabilities{
		probabilities:    map[string]float64{},
		probabilitiesMux: &sync.RWMutex{},
		defaultValue:     defaultValue,
	}, nil
}

func (p *PathProbabilities) List() map[string]float64 {
	p.probabilitiesMux.RLock()
	defer p.probabilitiesMux.RUnlock()
	return p.probabilities
}

func (p *PathProbabilities) ListForPaths(paths []string) map[string]float64 {
	p.probabilitiesMux.RLock()
	defer p.probabilitiesMux.RUnlock()

	probabilities := make(map[string]float64)
	for _, path := range paths {
		probabilities[path] = p.Get(path)
	}
	return probabilities
}

func (p *PathProbabilities) Get(path string) float64 {
	p.probabilitiesMux.RLock()
	probability, exists := p.probabilities[path]
	p.probabilitiesMux.RUnlock()

	if !exists {
		return p.defaultValue
	}
	return probability
}

func (p *PathProbabilities) Set(rule PathProbabilityRule) error {
	if rule.Probability < 0 || rule.Probability > 1 {
		return errors.New(fmt.Sprintf("PathProbabilities.Set() with path %s expected probability between 0 and 1; got probability = %v", rule.Path, rule.Probability))
	}

	// Ensure rules exist for the path both with and without a leading slash.
	path := prependLeadingSlashIfMissing(rule.Path)
	p.probabilitiesMux.Lock()
	p.probabilities[path] = rule.Probability
	p.probabilities[path[1:]] = rule.Probability
	p.probabilitiesMux.Unlock()

	return nil
}

func (p *PathProbabilities) SetAll(rules []PathProbabilityRule) error {
	for _, rule := range rules {
		if err := p.Set(rule); err != nil {
			return fmt.Errorf("PathProbabilities.SetAll() encountered error: %w", err)
		}
	}
	return nil
}

func (p *PathProbabilities) Clear() {
	p.probabilitiesMux.Lock()
	p.probabilities = map[string]float64{}
	p.probabilitiesMux.Unlock()
}

func (p *PathProbabilities) SampleShouldDim(path string) bool {
	return rand.Float64() < p.Get(path)
}

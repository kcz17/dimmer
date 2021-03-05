package filters

import (
	"errors"
	"fmt"
	"sync"
)

type PathProbabilities struct {
	// probabilities is a map from a path to a probability. Paths must be
	// inserted with and without their trailing slash to allow the trailing-
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
	path        string
	probability float64
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
	if rule.probability < 0 || rule.probability > 1 {
		return errors.New(fmt.Sprintf("PathProbabilities.Set() with path %s expected probability between 0 and 1; got probability = %v", rule.path, rule.probability))
	}

	// Ensure rules exist for the path both with and without a trailing slash.
	path := prependLeadingSlashIfMissing(rule.path)
	p.probabilitiesMux.Lock()
	p.probabilities[path] = rule.probability
	p.probabilities[path[1:]] = rule.probability
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

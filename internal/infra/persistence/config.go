package persistence

import (
	"context"
	"fmt"
	"sync"

	"audio-ui/internal/domain/valueobject"
)

// ConfigStore persists data/config.json atomically.
type ConfigStore struct {
	path string
	mu   sync.Mutex
}

func NewConfigStore(path string) *ConfigStore {
	return &ConfigStore{path: path}
}

func (s *ConfigStore) Load(ctx context.Context) (valueobject.AppConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return valueobject.AppConfig{}, err
	}
	var config valueobject.AppConfig
	found, err := readJSONFile(s.path, &config)
	if err != nil {
		return valueobject.AppConfig{}, fmt.Errorf("%w: %s: %v", ErrCorruptConfig, s.path, err)
	}
	if !found {
		return valueobject.NormalizeConfig(valueobject.AppConfig{}), nil
	}
	return normalizeConfig(config)
}

func (s *ConfigStore) Save(ctx context.Context, config valueobject.AppConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	normalized, err := normalizeConfig(config)
	if err != nil {
		return err
	}
	if err := writeJSONAtomic(s.path, normalized); err != nil {
		return fmt.Errorf("写入 %s: %w", s.path, err)
	}
	return nil
}

func normalizeConfig(config valueobject.AppConfig) (valueobject.AppConfig, error) {
	config = valueobject.NormalizeConfig(config)
	if config.LastProjectID != "" {
		if err := valueobject.ValidateID(config.LastProjectID); err != nil {
			return valueobject.AppConfig{}, fmt.Errorf("%w: lastProjectId: %v", ErrInvalidConfig, err)
		}
	}
	if config.ContextTokenBudget < 0 {
		return valueobject.AppConfig{}, fmt.Errorf("%w: contextTokenBudget must be positive", ErrInvalidConfig)
	}
	return config, nil
}

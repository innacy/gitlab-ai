package platform

import (
	"fmt"

	"gitlab-ai/pkg/config"
)

type ProviderFactory func(cfg *config.AppConfig) (Provider, error)

var registry = map[string]ProviderFactory{}

func Register(name string, factory ProviderFactory) {
	registry[name] = factory
}

func NewProvider(cfg *config.AppConfig) (Provider, error) {
	name := cfg.Platform
	if name == "" {
		name = "gitlab"
	}

	factory, ok := registry[name]
	if !ok {
		supported := make([]string, 0, len(registry))
		for k := range registry {
			supported = append(supported, k)
		}
		return nil, fmt.Errorf("unknown platform %q (supported: %v)", name, supported)
	}

	return factory(cfg)
}

func SupportedPlatforms() []string {
	names := make([]string, 0, len(registry))
	for k := range registry {
		names = append(names, k)
	}
	return names
}

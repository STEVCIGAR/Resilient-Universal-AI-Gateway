package ai

import (
	"fmt"
	"sync"

	"github.com/jsjg1/resilient-universal-ai-gateway/internal/domain"
)

// ProviderFactory implements a thread-safe registry/factory for AIProvider instances.
type ProviderFactory struct {
	mu        sync.RWMutex
	providers map[domain.ProviderType]domain.AIProvider
}

// NewProviderFactory creates an empty ProviderFactory.
func NewProviderFactory() *ProviderFactory {
	return &ProviderFactory{
		providers: make(map[domain.ProviderType]domain.AIProvider),
	}
}

// Register adds or updates a provider strategy in the factory.
func (f *ProviderFactory) Register(provider domain.AIProvider) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.providers[provider.Type()] = provider
}

// Get retrieves a provider by its type identifier.
func (f *ProviderFactory) Get(pType domain.ProviderType) (domain.AIProvider, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	provider, exists := f.providers[pType]
	if !exists {
		return nil, fmt.Errorf("%w: %s", domain.ErrProviderNotFound, pType)
	}
	return provider, nil
}

// ListProviders returns all registered provider types.
func (f *ProviderFactory) ListProviders() []domain.ProviderType {
	f.mu.RLock()
	defer f.mu.RUnlock()

	keys := make([]domain.ProviderType, 0, len(f.providers))
	for k := range f.providers {
		keys = append(keys, k)
	}
	return keys
}

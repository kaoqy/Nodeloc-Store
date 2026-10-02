package infrastructure

import (
	"sort"
	"strings"

	"github.com/kaoqy/Nodeloc-Store/internal/modules/plugin/contract"
)

// Registry is the set of plugin providers compiled into this binary.
//
// The runtime ships with the shop and providers register here at start-up, so a
// shop that installs a plugin never downloads or executes third-party code: it
// enrolls one of the capabilities the release already carries.
type Registry struct {
	providers map[string]contract.Provider
}

func NewRegistry(providers ...contract.Provider) *Registry {
	registry := &Registry{providers: map[string]contract.Provider{}}
	for _, provider := range providers {
		if provider == nil {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(provider.Key()))
		if key == "" {
			continue
		}
		registry.providers[key] = provider
	}
	return registry
}

func (r *Registry) Lookup(key string) (contract.Provider, bool) {
	provider, ok := r.providers[strings.ToLower(strings.TrimSpace(key))]
	return provider, ok
}

func (r *Registry) All() []contract.Provider {
	keys := make([]string, 0, len(r.providers))
	for key := range r.providers {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]contract.Provider, 0, len(keys))
	for _, key := range keys {
		out = append(out, r.providers[key])
	}
	return out
}

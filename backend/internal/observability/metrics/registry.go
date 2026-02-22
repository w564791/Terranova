package metrics

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

var (
	registry     *prometheus.Registry
	registryOnce sync.Once
)

// InitRegistry creates a new Prometheus registry and registers the default
// Go runtime and process collectors. It is safe to call multiple times;
// only the first invocation performs initialisation.
func InitRegistry() *prometheus.Registry {
	registryOnce.Do(func() {
		registry = prometheus.NewRegistry()
		registry.MustRegister(
			collectors.NewGoCollector(),
			collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		)
	})
	return registry
}

// GetRegistry returns the singleton Prometheus registry.
// It panics if InitRegistry has not been called yet.
func GetRegistry() *prometheus.Registry {
	if registry == nil {
		panic("metrics: InitRegistry() must be called before GetRegistry()")
	}
	return registry
}

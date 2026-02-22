package metrics

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resetRegistry() {
	registry = nil
	registryOnce = sync.Once{}
}

func TestInitRegistry_ReturnsNonNil(t *testing.T) {
	resetRegistry()
	defer resetRegistry()

	reg := InitRegistry()
	assert.NotNil(t, reg, "InitRegistry() must return a non-nil registry")
}

func TestInitRegistry_Idempotent(t *testing.T) {
	resetRegistry()
	defer resetRegistry()

	reg1 := InitRegistry()
	reg2 := InitRegistry()
	assert.Same(t, reg1, reg2, "InitRegistry() must return the same registry on repeated calls")
}

func TestInitRegistry_GatherContainsGoGoroutines(t *testing.T) {
	resetRegistry()
	defer resetRegistry()

	reg := InitRegistry()

	families, err := reg.Gather()
	require.NoError(t, err, "Gather() must not return an error")

	found := false
	for _, mf := range families {
		if mf.GetName() == "go_goroutines" {
			found = true
			break
		}
	}
	assert.True(t, found, "Gather() output must contain the go_goroutines metric family")
}

func TestGetRegistry_PanicsBeforeInit(t *testing.T) {
	resetRegistry()
	defer resetRegistry()

	assert.Panics(t, func() {
		GetRegistry()
	}, "GetRegistry() must panic if InitRegistry() has not been called")
}

func TestGetRegistry_ReturnsRegistryAfterInit(t *testing.T) {
	resetRegistry()
	defer resetRegistry()

	expected := InitRegistry()
	actual := GetRegistry()
	assert.Same(t, expected, actual, "GetRegistry() must return the same registry as InitRegistry()")
}

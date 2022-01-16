package wasm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetModuleInstance(t *testing.T) {
	name := "test"

	// 'jit' and 'wazeroir' package cannot be used because of circular import.
	// Here we will use 'nil' instead. Should we have an Engine for testing?
	s := NewStore(nil)

	m1 := s.getModuleInstance(name)
	assert.Equal(t, m1, s.ModuleInstances[name])
	assert.NotNil(t, m1.Exports)

	m2 := s.getModuleInstance(name)
	assert.Equal(t, m1, m2)
}

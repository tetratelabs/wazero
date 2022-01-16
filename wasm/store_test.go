package wasm

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetModuleInstance(t *testing.T) {
	name := "test"

	// 'jit' and 'wazeroir' package cannot be used because of circular import.
	// Here we will use 'nil' instead. Should we have an Engine for testing?
	s := NewStore(nil)

	m1 := s.getModuleInstance(name)
	require.Equal(t, m1, s.ModuleInstances[name])
	require.NotNil(t, m1.Exports)

	m2 := s.getModuleInstance(name)
	require.Equal(t, m1, m2)
}

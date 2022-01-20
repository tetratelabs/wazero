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

func TestBuildFunctionInstances_FunctionNames(t *testing.T) {
	name := "test"
	s := NewStore(nil)
	mi := s.getModuleInstance(name)

	zero := Index(0)
	nopCode := &Code{0, nil, []byte{OpcodeEnd}}
	m := &Module{
		TypeSection:     []*FunctionType{{}},
		FunctionSection: []Index{zero, zero, zero, zero, zero},
		NameSection: &NameSection{
			FunctionNames: NameMap{
				{Index: Index(1), Name: "two"},
				{Index: Index(3), Name: "four"},
				{Index: Index(4), Name: "five"},
			},
		},
		CodeSection: []*Code{nopCode, nopCode, nopCode, nopCode, nopCode},
	}

	_, err := s.buildFunctionInstances(m, mi)
	require.NoError(t, err)

	var names []string
	for _, f := range mi.Functions {
		names = append(names, f.Name)
	}

	// We expect unknown for any functions missing data in the NameSection
	require.Equal(t, []string{"unknown", "two", "unknown", "four", "five"}, names)
}

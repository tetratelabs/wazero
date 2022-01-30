package wasm

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetModuleInstance(t *testing.T) {
	name := "test"

	s := NewStore(nopEngineInstance)

	m1 := s.getModuleInstance(name)
	require.Equal(t, m1, s.ModuleInstances[name])
	require.NotNil(t, m1.Exports)

	m2 := s.getModuleInstance(name)
	require.Equal(t, m1, m2)
}

func TestBuildFunctionInstances_FunctionNames(t *testing.T) {
	name := "test"

	s := NewStore(nopEngineInstance)
	mi := s.getModuleInstance(name)

	zero := Index(0)
	nopCode := &Code{nil, []byte{OpcodeEnd}}
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

var nopEngineInstance Engine = &nopEngine{}

type nopEngine struct {
}

func (e *nopEngine) Call(_ *FunctionInstance, _ ...uint64) (results []uint64, err error) {
	return nil, nil
}

func (e *nopEngine) Compile(_ *FunctionInstance) error {
	return nil
}

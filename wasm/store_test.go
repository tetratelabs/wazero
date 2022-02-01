package wasm

import (
	"reflect"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStore_GetModuleInstance(t *testing.T) {
	name := "test"

	s := NewStore(nopEngineInstance)

	m1 := s.getModuleInstance(name)
	require.Equal(t, m1, s.ModuleInstances[name])
	require.NotNil(t, m1.Exports)

	m2 := s.getModuleInstance(name)
	require.Equal(t, m1, m2)
}

func TestStore_AddHostFunction(t *testing.T) {
	s := NewStore(nopEngineInstance)
	hostFunction := func(_ *HostFunctionCallContext) {
	}

	err := s.AddHostFunction("test", "fn", reflect.ValueOf(hostFunction))
	require.NoError(t, err)

	// The function was added to the store, prefixed by the owning module name
	require.Equal(t, 1, len(s.Functions))
	fn := s.Functions[0]
	require.Equal(t, "test.fn", fn.Name)

	// The function was exported in the module
	m := s.getModuleInstance("test")
	require.Equal(t, 1, len(m.Exports))
	exp, ok := m.Exports["fn"]
	require.True(t, ok)

	// Trying to add it again should fail
	hostFunction2 := func(_ *HostFunctionCallContext) {
	}
	err = s.AddHostFunction("test", "fn", reflect.ValueOf(hostFunction2))
	require.EqualError(t, err, `"fn" is already exported in module "test"`)

	// Any side effects should be reverted
	require.Equal(t, []*FunctionInstance{fn}, s.Functions)
	require.Equal(t, map[string]*ExportInstance{"fn": exp}, m.Exports)
}

func TestStore_BuildFunctionInstances_FunctionNames(t *testing.T) {
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

func TestStore_addFunctionInstance(t *testing.T) {
	t.Run("too many functions", func(t *testing.T) {
		s := NewStore(nopEngineInstance)
		const max = 10
		s.maxmimuFunctionAddress = max
		s.Functions = make([]*FunctionInstance, max)
		err := s.addFunctionInstance(nil)
		require.Error(t, err)
	})
	t.Run("ok", func(t *testing.T) {
		s := NewStore(nopEngineInstance)
		for i := 0; i < 10; i++ {
			f := &FunctionInstance{}
			require.Len(t, s.Functions, i)

			err := s.addFunctionInstance(f)
			require.NoError(t, err)

			// After the addition, one instance is added.
			require.Len(t, s.Functions, i+1)

			// The added function instance must have i for its funcaddr.
			require.Equal(t, FunctionAddress(i), f.Address)
		}
	})
}

func TestStore_getTypeInstance(t *testing.T) {
	t.Run("too many functions", func(t *testing.T) {
		s := NewStore(nopEngineInstance)
		const max = 10
		s.maxmimuFunctionTypes = max
		s.TypeIDs = make(map[string]FunctionTypeID)
		for i := 0; i < max; i++ {
			s.TypeIDs[strconv.Itoa(i)] = 0
		}
		_, err := s.getTypeInstance(&FunctionType{})
		require.Error(t, err)
	})
	t.Run("ok", func(t *testing.T) {
		for _, tc := range []*FunctionType{
			{Params: []ValueType{}},
			{Params: []ValueType{ValueTypeF32}},
			{Results: []ValueType{ValueTypeF64}},
			{Params: []ValueType{ValueTypeI32}, Results: []ValueType{ValueTypeI64}},
		} {
			tc := tc
			t.Run(tc.String(), func(t *testing.T) {
				s := NewStore(nopEngineInstance)
				actual, err := s.getTypeInstance(tc)
				require.NoError(t, err)

				expectedTypeID, ok := s.TypeIDs[tc.String()]
				require.True(t, ok)
				require.Equal(t, expectedTypeID, actual.TypeID)
				require.Equal(t, tc, actual.Type)
			})
		}
	})
}

package wasm

import (
	"context"
	"fmt"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestStore_registerModule(t *testing.T) {
	s := newStore()
	m1 := &ModuleInstance{ModuleName: "m1"}

	t.Run("adds module", func(t *testing.T) {
		require.NoError(t, s.registerModule(m1))
		require.Equal(t, map[string]*ModuleInstance{m1.ModuleName: m1}, s.nameToModule)
		require.Equal(t, m1, s.moduleList)
		require.Equal(t, nameToModuleShrinkThreshold, s.nameToModuleCap)
	})

	t.Run("adds second module", func(t *testing.T) {
		m2 := &ModuleInstance{ModuleName: "m2"}
		require.NoError(t, s.registerModule(m2))
		require.Equal(t, map[string]*ModuleInstance{m1.ModuleName: m1, m2.ModuleName: m2}, s.nameToModule)
		require.Equal(t, m2, s.moduleList)
		require.Equal(t, nameToModuleShrinkThreshold, s.nameToModuleCap)
	})

	t.Run("error on duplicated non anonymous", func(t *testing.T) {
		m1Second := &ModuleInstance{ModuleName: "m1"}
		require.EqualError(t, s.registerModule(m1Second), "module[m1] has already been instantiated")
	})

	t.Run("error on closed", func(t *testing.T) {
		require.NoError(t, s.CloseWithExitCode(context.Background(), 0))
		require.Error(t, s.registerModule(m1))
	})
}

func TestStore_deleteModule(t *testing.T) {
	s, m1, m2 := newTestStore()

	t.Run("delete one module", func(t *testing.T) {
		require.NoError(t, s.deleteModule(m2))

		// Leaves the other module alone.
		require.Equal(t, map[string]*ModuleInstance{m1.ModuleName: m1}, s.nameToModule)
		require.Equal(t, m1, s.moduleList)
		require.Equal(t, nameToModuleShrinkThreshold, s.nameToModuleCap)
	})

	t.Run("ok if missing", func(t *testing.T) {
		require.NoError(t, s.deleteModule(m2))
	})

	t.Run("delete last module", func(t *testing.T) {
		require.NoError(t, s.deleteModule(m1))

		require.Zero(t, len(s.nameToModule))
		require.Nil(t, s.moduleList)
		require.Equal(t, nameToModuleShrinkThreshold, s.nameToModuleCap)
	})

	t.Run("delete middle", func(t *testing.T) {
		s := newStore()
		one, two, three := &ModuleInstance{ModuleName: "1"}, &ModuleInstance{ModuleName: "2"}, &ModuleInstance{ModuleName: "3"}
		require.NoError(t, s.registerModule(one))
		require.NoError(t, s.registerModule(two))
		require.NoError(t, s.registerModule(three))
		require.Equal(t, three, s.moduleList)
		require.Nil(t, three.prev)
		require.Equal(t, two, three.next)
		require.Equal(t, two.prev, three)
		require.Equal(t, one, two.next)
		require.Equal(t, one.prev, two)
		require.Nil(t, one.next)
		require.NoError(t, s.deleteModule(two))
		require.Equal(t, three, s.moduleList)
		require.Nil(t, three.prev)
		require.Equal(t, one, three.next)
		require.Equal(t, one.prev, three)
		require.Nil(t, one.next)
	})
}

func TestStore_module(t *testing.T) {
	s, m1, _ := newTestStore()

	t.Run("ok", func(t *testing.T) {
		got, err := s.module(m1.ModuleName)
		require.NoError(t, err)
		require.Equal(t, m1, got)
	})

	t.Run("unknown", func(t *testing.T) {
		got, err := s.module("unknown")
		require.Error(t, err)
		require.Nil(t, got)
	})

	t.Run("store closed", func(t *testing.T) {
		require.NoError(t, s.CloseWithExitCode(context.Background(), 0))
		got, err := s.module(m1.ModuleName)
		require.Error(t, err)
		require.Nil(t, got)
	})
}

func TestStore_nameToModuleCap(t *testing.T) {
	t.Run("nameToModuleCap grows beyond initial cap", func(t *testing.T) {
		s := newStore()
		for i := 0; i < 300; i++ {
			require.NoError(t, s.registerModule(&ModuleInstance{ModuleName: fmt.Sprintf("m%d", i)}))
		}

		require.Equal(t, 300, s.nameToModuleCap)
	})

	t.Run("nameToModuleCap shrinks by half the cap", func(t *testing.T) {
		s := newStore()
		for i := 0; i < 400; i++ {
			require.NoError(t, s.registerModule(&ModuleInstance{ModuleName: fmt.Sprintf("m%d", i)}))
		}

		for i := 0; i < 250; i++ {
			require.NoError(t, s.deleteModule(s.nameToModule[fmt.Sprintf("m%d", i)]))
		}

		require.Equal(t, 200, s.nameToModuleCap)
	})

	t.Run("nameToModuleCap does not shrink below initial size", func(t *testing.T) {
		s := newStore()
		for i := 0; i < 400; i++ {
			require.NoError(t, s.registerModule(&ModuleInstance{ModuleName: fmt.Sprintf("m%d", i)}))
		}

		for i := 0; i < 350; i++ {
			require.NoError(t, s.deleteModule(s.nameToModule[fmt.Sprintf("m%d", i)]))
		}

		require.Equal(t, nameToModuleShrinkThreshold, s.nameToModuleCap)
	})

	t.Run("nameToModuleCap does not grow when if nameToModule does not grow", func(t *testing.T) {
		s := newStore()
		for i := 0; i < 99; i++ {
			require.NoError(t, s.registerModule(&ModuleInstance{ModuleName: fmt.Sprintf("m%d", i)}))
		}
		for i := 0; i < 400; i++ {
			require.NoError(t, s.registerModule(&ModuleInstance{ModuleName: fmt.Sprintf("m%d", i+99)}))
			require.NoError(t, s.deleteModule(s.nameToModule[fmt.Sprintf("m%d", i+99)]))
			require.Equal(t, nameToModuleShrinkThreshold, s.nameToModuleCap)
		}
	})
}

func TestStore_Module(t *testing.T) {
	s, m1, _ := newTestStore()

	t.Run("ok", func(t *testing.T) {
		require.Equal(t, m1, s.Module(m1.ModuleName))
	})

	t.Run("unknown", func(t *testing.T) {
		require.Nil(t, s.Module("unknown"))
	})
}

// newTestStore sets up a new Store without adding test coverage its functions.
func newTestStore() (*Store, *ModuleInstance, *ModuleInstance) {
	s := newStore()
	m1 := &ModuleInstance{ModuleName: "m1"}
	m2 := &ModuleInstance{ModuleName: "m2"}

	m1.prev = m2
	m2.next = m1
	s.nameToModule = map[string]*ModuleInstance{m1.ModuleName: m1, m2.ModuleName: m2}
	s.moduleList = m2
	return s, m1, m2
}

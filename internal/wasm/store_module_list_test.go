package wasm

import (
	"context"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestStore_registerModule(t *testing.T) {
	s := newStore()
	m1 := &ModuleInstance{ModuleName: "m1"}

	t.Run("adds module", func(t *testing.T) {
		require.NoError(t, s.registerModule(m1))
		require.Equal(t, map[string]*ModuleInstance{m1.ModuleName: m1}, s.nameToNode)
		require.Equal(t, m1, s.moduleList)
	})

	t.Run("adds second module", func(t *testing.T) {
		m2 := &ModuleInstance{ModuleName: "m2"}
		require.NoError(t, s.registerModule(m2))
		require.Equal(t, map[string]*ModuleInstance{m1.ModuleName: m1, m2.ModuleName: m2}, s.nameToNode)
		require.Equal(t, m2, s.moduleList)
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

		// Leaves the other module alone
		require.Equal(t, map[string]*ModuleInstance{m1.ModuleName: m1}, s.nameToNode)
		require.Equal(t, m1, s.moduleList)
	})

	t.Run("ok if missing", func(t *testing.T) {
		require.NoError(t, s.deleteModule(m2))
	})

	t.Run("delete last module", func(t *testing.T) {
		require.NoError(t, s.deleteModule(m1))

		require.Zero(t, len(s.nameToNode))
		require.Nil(t, s.moduleList)
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

func TestStore_requireModules(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		s, m1, _ := newTestStore()

		modules, err := s.requireModules(map[string]struct{}{m1.ModuleName: {}})
		require.NoError(t, err)
		require.Equal(t, map[string]*ModuleInstance{m1.ModuleName: m1}, modules)
	})
	t.Run("module not instantiated", func(t *testing.T) {
		s, _, _ := newTestStore()

		_, err := s.requireModules(map[string]struct{}{"unknown": {}})
		require.EqualError(t, err, "module[unknown] not instantiated")
	})
	t.Run("store closed", func(t *testing.T) {
		s, _, _ := newTestStore()
		require.NoError(t, s.CloseWithExitCode(context.Background(), 0))

		_, err := s.requireModules(map[string]struct{}{"unknown": {}})
		require.Error(t, err)
	})
}

func TestStore_AliasModule(t *testing.T) {
	s := newStore()
	m1 := &ModuleInstance{ModuleName: "m1"}
	s.nameToNode[m1.ModuleName] = m1

	t.Run("alias module", func(t *testing.T) {
		require.NoError(t, s.AliasModule("m1", "m2"))
		require.Equal(t, map[string]*ModuleInstance{"m1": m1, "m2": m1}, s.nameToNode)
		// Doesn't affect module names
		require.Nil(t, s.moduleList)
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
	s.nameToNode = map[string]*ModuleInstance{m1.ModuleName: m1, m2.ModuleName: m2}
	s.moduleList = m2
	return s, m1, m2
}

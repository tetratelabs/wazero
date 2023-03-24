package wasm

import (
	"context"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestStore_setModule(t *testing.T) {
	s := newStore()
	m1 := &ModuleInstance{ModuleName: "m1"}

	t.Run("errors if not required", func(t *testing.T) {
		require.Error(t, s.setModule(m1))
	})

	t.Run("adds module", func(t *testing.T) {
		s.nameToNode[m1.ModuleName] = &moduleListNode{name: m1.ModuleName}
		require.NoError(t, s.setModule(m1))
		require.Equal(t, map[string]*moduleListNode{m1.ModuleName: {name: m1.ModuleName, module: m1}}, s.nameToNode)

		// Doesn't affect module names
		require.Nil(t, s.moduleList)
	})

	t.Run("redundant ok", func(t *testing.T) {
		require.NoError(t, s.setModule(m1))
		require.Equal(t, map[string]*moduleListNode{m1.ModuleName: {name: m1.ModuleName, module: m1}}, s.nameToNode)

		// Doesn't affect module names
		require.Nil(t, s.moduleList)
	})

	t.Run("adds second module", func(t *testing.T) {
		m2 := &ModuleInstance{ModuleName: "m2"}
		s.nameToNode[m2.ModuleName] = &moduleListNode{name: m2.ModuleName}
		require.NoError(t, s.setModule(m2))
		require.Equal(t, map[string]*moduleListNode{m1.ModuleName: {name: m1.ModuleName, module: m1}, m2.ModuleName: {name: m2.ModuleName, module: m2}}, s.nameToNode)

		// Doesn't affect module names
		require.Nil(t, s.moduleList)
	})

	t.Run("error on closed", func(t *testing.T) {
		require.NoError(t, s.CloseWithExitCode(context.Background(), 0))
		require.Error(t, s.setModule(m1))
	})
}

func TestStore_deleteModule(t *testing.T) {
	s, m1, m2 := newTestStore()

	t.Run("delete one module", func(t *testing.T) {
		require.NoError(t, s.deleteModule(m2.moduleListNode))

		// Leaves the other module alone
		m1Node := &moduleListNode{name: m1.ModuleName, module: m1}
		require.Equal(t, map[string]*moduleListNode{m1.ModuleName: m1Node}, s.nameToNode)
		require.Equal(t, m1Node, s.moduleList)
	})

	t.Run("ok if missing", func(t *testing.T) {
		require.NoError(t, s.deleteModule(m2.moduleListNode))
	})

	t.Run("delete last module", func(t *testing.T) {
		require.NoError(t, s.deleteModule(m1.moduleListNode))

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

	t.Run("not set", func(t *testing.T) {
		s.nameToNode["not set"] = &moduleListNode{name: "not set"}
		got, err := s.module("not set")
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

func TestStore_requireModuleName(t *testing.T) {
	s := newStore()

	t.Run("first", func(t *testing.T) {
		_, err := s.requireModuleName("m1")
		require.NoError(t, err)

		// Ensure it adds the module name, and doesn't impact the module list.
		require.Equal(t, &moduleListNode{name: "m1"}, s.moduleList)
		require.Equal(t, map[string]*moduleListNode{"m1": {name: "m1"}}, s.nameToNode)
	})
	t.Run("second", func(t *testing.T) {
		_, err := s.requireModuleName("m2")
		require.NoError(t, err)
		m2Node := &moduleListNode{name: "m2"}
		m1Node := &moduleListNode{name: "m1", prev: m2Node}
		m2Node.next = m1Node

		// Appends in order.
		require.Equal(t, m2Node, s.moduleList)
		require.Equal(t, map[string]*moduleListNode{"m1": m1Node, "m2": m2Node}, s.nameToNode)
	})
	t.Run("existing", func(t *testing.T) {
		_, err := s.requireModuleName("m2")
		require.EqualError(t, err, "module[m2] has already been instantiated")
	})
}

func TestStore_AliasModule(t *testing.T) {
	s := newStore()

	m1 := &ModuleInstance{ModuleName: "m1"}
	s.nameToNode[m1.ModuleName] = &moduleListNode{name: m1.ModuleName, module: m1}

	t.Run("alias module", func(t *testing.T) {
		require.NoError(t, s.AliasModule("m1", "m2"))
		m1node := &moduleListNode{name: "m1", module: m1}
		require.Equal(t, map[string]*moduleListNode{"m1": m1node, "m2": m1node}, s.nameToNode)
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

	node1 := &moduleListNode{name: m1.ModuleName, module: m1}
	node2 := &moduleListNode{name: m2.ModuleName, module: m2, next: node1}
	node1.prev = node2
	s.nameToNode = map[string]*moduleListNode{m1.ModuleName: node1, m2.ModuleName: node2}
	s.moduleList = node2

	m1.moduleListNode = node1
	m2.moduleListNode = node2
	return s, m1, m2
}

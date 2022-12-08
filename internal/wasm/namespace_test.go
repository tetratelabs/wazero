package wasm

import (
	"errors"
	"testing"

	"github.com/tetratelabs/wazero/internal/sys"
	testfs "github.com/tetratelabs/wazero/internal/testing/fs"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func Test_newNamespace(t *testing.T) {
	ns := newNamespace()
	require.NotNil(t, ns.modules)
}

func TestNamespace_addModule(t *testing.T) {
	ns := newNamespace()
	m1 := &ModuleInstance{Name: "m1"}

	t.Run("adds module", func(t *testing.T) {
		ns.addModule(m1)

		require.Equal(t, map[string]*ModuleInstance{m1.Name: m1}, ns.modules)
		// Doesn't affect module names
		require.Zero(t, len(ns.moduleNamesSet))
		require.Nil(t, ns.moduleNamesList)
	})

	t.Run("redundant ok", func(t *testing.T) {
		ns.addModule(m1)
		require.Equal(t, map[string]*ModuleInstance{m1.Name: m1}, ns.modules)
	})

	t.Run("adds second module", func(t *testing.T) {
		m2 := &ModuleInstance{Name: "m2"}
		ns.addModule(m2)
		require.Equal(t, map[string]*ModuleInstance{m1.Name: m1, m2.Name: m2}, ns.modules)
	})
}

func TestNamespace_deleteModule(t *testing.T) {
	ns, m1, m2 := newTestNamespace()

	t.Run("delete one module", func(t *testing.T) {
		ns.deleteModule(m2.Name)

		// Leaves the other module alone
		require.Equal(t, map[string]*ModuleInstance{m1.Name: m1}, ns.modules)
		require.Equal(t, map[string]*nameListNode{m1.Name: {name: m1.Name}}, ns.moduleNamesSet)
		require.Equal(t, &nameListNode{name: m1.Name}, ns.moduleNamesList)
	})

	t.Run("ok if missing", func(t *testing.T) {
		ns.deleteModule(m2.Name)
	})

	t.Run("delete last module", func(t *testing.T) {
		ns.deleteModule(m1.Name)

		require.Zero(t, len(ns.modules))
		require.Zero(t, len(ns.moduleNamesSet))
		require.Nil(t, ns.moduleNamesList)
	})
}

func TestNamespace_module(t *testing.T) {
	ns, m1, _ := newTestNamespace()

	t.Run("ok", func(t *testing.T) {
		require.Equal(t, m1, ns.module(m1.Name))
	})

	t.Run("unknown", func(t *testing.T) {
		require.Nil(t, ns.module("unknown"))
	})
}

func TestNamespace_requireModules(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		ns, m1, _ := newTestNamespace()

		modules, err := ns.requireModules(map[string]struct{}{m1.Name: {}})
		require.NoError(t, err)
		require.Equal(t, map[string]*ModuleInstance{m1.Name: m1}, modules)
	})
	t.Run("module not instantiated", func(t *testing.T) {
		ns, _, _ := newTestNamespace()

		_, err := ns.requireModules(map[string]struct{}{"unknown": {}})
		require.EqualError(t, err, "module[unknown] not instantiated")
	})
}

func TestNamespace_requireModuleName(t *testing.T) {
	ns := &Namespace{moduleNamesSet: map[string]*nameListNode{}}

	t.Run("first", func(t *testing.T) {
		err := ns.requireModuleName("m1")
		require.NoError(t, err)

		// Ensure it adds the module name, and doesn't impact the module list.
		require.Equal(t, &nameListNode{name: "m1"}, ns.moduleNamesList)
		require.Equal(t, map[string]*nameListNode{"m1": {name: "m1"}}, ns.moduleNamesSet)
		require.Zero(t, len(ns.modules))
	})
	t.Run("second", func(t *testing.T) {
		err := ns.requireModuleName("m2")
		require.NoError(t, err)
		m2Node := &nameListNode{name: "m2"}
		m1Node := &nameListNode{name: "m1", prev: m2Node}
		m2Node.next = m1Node

		// Appends in order.
		require.Equal(t, m2Node, ns.moduleNamesList)
		require.Equal(t, map[string]*nameListNode{"m1": m1Node, "m2": m2Node}, ns.moduleNamesSet)
	})
	t.Run("existing", func(t *testing.T) {
		err := ns.requireModuleName("m2")
		require.EqualError(t, err, "module[m2] has already been instantiated")
	})
}

func TestNamespace_AliasModule(t *testing.T) {
	ns := newNamespace()
	m1 := &ModuleInstance{Name: "m1"}
	ns.addModule(m1)

	ns.AliasModule("m1", "m2")
	require.Equal(t, map[string]*ModuleInstance{"m1": m1, "m2": m1}, ns.modules)
	// Doesn't affect module names
	require.Zero(t, len(ns.moduleNamesSet))
	require.Nil(t, ns.moduleNamesList)
}

func TestNamespace_CloseWithExitCode(t *testing.T) {
	tests := []struct {
		name       string
		testClosed bool
	}{
		{
			name:       "nothing closed",
			testClosed: false,
		},
		{
			name:       "partially closed",
			testClosed: true,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			ns, m1, m2 := newTestNamespace()

			if tc.testClosed {
				err := m2.CallCtx.CloseWithExitCode(testCtx, 2)
				require.NoError(t, err)
			}

			err := ns.CloseWithExitCode(testCtx, 2)
			require.NoError(t, err)

			// Both modules were closed
			require.Equal(t, uint64(1)+uint64(2)<<32, *m1.CallCtx.closed)
			require.Equal(t, uint64(1)+uint64(2)<<32, *m2.CallCtx.closed)

			// Namespace state zeroed
			require.Zero(t, len(ns.modules))
			require.Zero(t, len(ns.moduleNamesSet))
			require.Nil(t, ns.moduleNamesList)
		})
	}

	t.Run("error closing", func(t *testing.T) {
		// Right now, the only way to err closing the sys context is if a File.Close erred.
		testFS := testfs.FS{"foo": &testfs.File{CloseErr: errors.New("error closing")}}
		sysCtx := sys.DefaultContext(testFS)
		fsCtx := sysCtx.FS(testCtx)

		_, err := fsCtx.OpenFile(testCtx, "/foo")
		require.NoError(t, err)

		ns, m1, m2 := newTestNamespace()
		m1.CallCtx.Sys = sysCtx // This should err, but both should close

		err = ns.CloseWithExitCode(testCtx, 2)
		require.EqualError(t, err, "error closing")

		// Both modules were closed
		require.Equal(t, uint64(1)+uint64(2)<<32, *m1.CallCtx.closed)
		require.Equal(t, uint64(1)+uint64(2)<<32, *m2.CallCtx.closed)

		// Namespace state zeroed
		require.Zero(t, len(ns.modules))
		require.Zero(t, len(ns.moduleNamesSet))
		require.Nil(t, ns.moduleNamesList)
	})
}

func TestNamespace_Module(t *testing.T) {
	ns, m1, _ := newTestNamespace()

	t.Run("ok", func(t *testing.T) {
		require.Equal(t, m1.CallCtx, ns.Module(m1.Name))
	})

	t.Run("unknown", func(t *testing.T) {
		require.Nil(t, ns.Module("unknown"))
	})
}

// newTestNamespace sets up a new Namespace without adding test coverage its functions.
func newTestNamespace() (*Namespace, *ModuleInstance, *ModuleInstance) {
	ns := &Namespace{}
	m1 := &ModuleInstance{Name: "m1"}
	m1.CallCtx = NewCallContext(ns, m1, nil)

	m2 := &ModuleInstance{Name: "m2"}
	m2.CallCtx = NewCallContext(ns, m2, nil)

	ns.modules = map[string]*ModuleInstance{m1.Name: m1, m2.Name: m2}
	node1 := &nameListNode{name: m1.Name}
	node2 := &nameListNode{name: m2.Name, next: node1}
	ns.moduleNamesSet = map[string]*nameListNode{m1.Name: node1, m2.Name: node2}
	ns.moduleNamesList = node2
	return ns, m1, m2
}

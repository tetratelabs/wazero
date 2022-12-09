package wasm

import (
	"context"
	"errors"
	"testing"

	"github.com/tetratelabs/wazero/internal/sys"
	testfs "github.com/tetratelabs/wazero/internal/testing/fs"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func Test_newNamespace(t *testing.T) {
	ns := newNamespace()
	require.NotNil(t, ns.nameToNode)
}

func TestNamespace_setModule(t *testing.T) {
	ns := newNamespace()
	m1 := &ModuleInstance{Name: "m1"}

	t.Run("errors if not required", func(t *testing.T) {
		require.Error(t, ns.setModule(m1))
	})

	t.Run("adds module", func(t *testing.T) {
		ns.nameToNode[m1.Name] = &moduleListNode{name: m1.Name}
		require.NoError(t, ns.setModule(m1))
		require.Equal(t, map[string]*moduleListNode{m1.Name: {name: m1.Name, module: m1}}, ns.nameToNode)

		// Doesn't affect module names
		require.Nil(t, ns.moduleList)
	})

	t.Run("redundant ok", func(t *testing.T) {
		require.NoError(t, ns.setModule(m1))
		require.Equal(t, map[string]*moduleListNode{m1.Name: {name: m1.Name, module: m1}}, ns.nameToNode)

		// Doesn't affect module names
		require.Nil(t, ns.moduleList)
	})

	t.Run("adds second module", func(t *testing.T) {
		m2 := &ModuleInstance{Name: "m2"}
		ns.nameToNode[m2.Name] = &moduleListNode{name: m2.Name}
		require.NoError(t, ns.setModule(m2))
		require.Equal(t, map[string]*moduleListNode{m1.Name: {name: m1.Name, module: m1}, m2.Name: {name: m2.Name, module: m2}}, ns.nameToNode)

		// Doesn't affect module names
		require.Nil(t, ns.moduleList)
	})

	t.Run("error on closed", func(t *testing.T) {
		require.NoError(t, ns.CloseWithExitCode(context.Background(), 0))
		require.Error(t, ns.setModule(m1))
	})
}

func TestNamespace_deleteModule(t *testing.T) {
	ns, m1, m2 := newTestNamespace()

	t.Run("delete one module", func(t *testing.T) {
		require.NoError(t, ns.deleteModule(m2.Name))

		// Leaves the other module alone
		m1Node := &moduleListNode{name: m1.Name, module: m1}
		require.Equal(t, map[string]*moduleListNode{m1.Name: m1Node}, ns.nameToNode)
		require.Equal(t, m1Node, ns.moduleList)
	})

	t.Run("ok if missing", func(t *testing.T) {
		require.NoError(t, ns.deleteModule(m2.Name))
	})

	t.Run("delete last module", func(t *testing.T) {
		require.NoError(t, ns.deleteModule(m1.Name))

		require.Zero(t, len(ns.nameToNode))
		require.Nil(t, ns.moduleList)
	})

	t.Run("error on closed", func(t *testing.T) {
		require.NoError(t, ns.CloseWithExitCode(context.Background(), 0))
		require.Error(t, ns.deleteModule(m1.Name))

		require.Zero(t, len(ns.nameToNode))
		require.Nil(t, ns.moduleList)
	})
}

func TestNamespace_module(t *testing.T) {
	ns, m1, _ := newTestNamespace()

	t.Run("ok", func(t *testing.T) {
		got, err := ns.module(m1.Name)
		require.NoError(t, err)
		require.Equal(t, m1, got)
	})

	t.Run("unknown", func(t *testing.T) {
		got, err := ns.module("unknown")
		require.Error(t, err)
		require.Nil(t, got)
	})

	t.Run("not set", func(t *testing.T) {
		ns.nameToNode["not set"] = &moduleListNode{name: "not set"}
		got, err := ns.module("not set")
		require.Error(t, err)
		require.Nil(t, got)
	})

	t.Run("namespace closed", func(t *testing.T) {
		require.NoError(t, ns.CloseWithExitCode(context.Background(), 0))
		got, err := ns.module(m1.Name)
		require.Error(t, err)
		require.Nil(t, got)
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
	t.Run("namespace closed", func(t *testing.T) {
		ns, _, _ := newTestNamespace()
		ns.CloseWithExitCode(context.Background(), 0)

		_, err := ns.requireModules(map[string]struct{}{"unknown": {}})
		require.Error(t, err)
	})
}

func TestNamespace_requireModuleName(t *testing.T) {
	ns := &Namespace{nameToNode: map[string]*moduleListNode{}, closed: new(uint32)}

	t.Run("first", func(t *testing.T) {
		err := ns.requireModuleName("m1")
		require.NoError(t, err)

		// Ensure it adds the module name, and doesn't impact the module list.
		require.Equal(t, &moduleListNode{name: "m1"}, ns.moduleList)
		require.Equal(t, map[string]*moduleListNode{"m1": {name: "m1"}}, ns.nameToNode)
	})
	t.Run("second", func(t *testing.T) {
		err := ns.requireModuleName("m2")
		require.NoError(t, err)
		m2Node := &moduleListNode{name: "m2"}
		m1Node := &moduleListNode{name: "m1", prev: m2Node}
		m2Node.next = m1Node

		// Appends in order.
		require.Equal(t, m2Node, ns.moduleList)
		require.Equal(t, map[string]*moduleListNode{"m1": m1Node, "m2": m2Node}, ns.nameToNode)
	})
	t.Run("existing", func(t *testing.T) {
		err := ns.requireModuleName("m2")
		require.EqualError(t, err, "module[m2] has already been instantiated")
	})
	t.Run("namespace closed", func(t *testing.T) {
		ns.CloseWithExitCode(context.Background(), 0)
		require.Error(t, ns.requireModuleName("m3"))
	})
}

func TestNamespace_AliasModule(t *testing.T) {
	ns := newNamespace()
	m1 := &ModuleInstance{Name: "m1"}
	ns.nameToNode[m1.Name] = &moduleListNode{name: m1.Name, module: m1}

	t.Run("alias module", func(t *testing.T) {
		require.NoError(t, ns.AliasModule("m1", "m2"))
		m1node := &moduleListNode{name: "m1", module: m1}
		require.Equal(t, map[string]*moduleListNode{"m1": m1node, "m2": m1node}, ns.nameToNode)
		// Doesn't affect module names
		require.Nil(t, ns.moduleList)
	})
	t.Run("namespace closed", func(t *testing.T) {
		ns.CloseWithExitCode(context.Background(), 0)
		require.Error(t, ns.AliasModule("m3", "m4"))
		require.Nil(t, ns.nameToNode)
		require.Nil(t, ns.moduleList)
	})
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
			require.Zero(t, len(ns.nameToNode))
			require.Nil(t, ns.moduleList)
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
		require.Zero(t, len(ns.nameToNode))
		require.Nil(t, ns.moduleList)
	})
	t.Run("multiple closes", func(t *testing.T) {
		ns, m1, m2 := newTestNamespace()

		require.NoError(t, ns.CloseWithExitCode(testCtx, 2))

		// Both modules were closed
		require.Equal(t, uint64(1)+uint64(2)<<32, *m1.CallCtx.closed)
		require.Equal(t, uint64(1)+uint64(2)<<32, *m2.CallCtx.closed)

		// Namespace state zeroed
		require.Zero(t, len(ns.nameToNode))
		require.Nil(t, ns.moduleList)

		require.NoError(t, ns.CloseWithExitCode(testCtx, 2))
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
	ns := &Namespace{closed: new(uint32)}
	m1 := &ModuleInstance{Name: "m1"}
	m1.CallCtx = NewCallContext(ns, m1, nil)

	m2 := &ModuleInstance{Name: "m2"}
	m2.CallCtx = NewCallContext(ns, m2, nil)

	node1 := &moduleListNode{name: m1.Name, module: m1}
	node2 := &moduleListNode{name: m2.Name, module: m2, next: node1}
	node1.prev = node2
	ns.nameToNode = map[string]*moduleListNode{m1.Name: node1, m2.Name: node2}
	ns.moduleList = node2
	return ns, m1, m2
}

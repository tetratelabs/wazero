package internalwasm

import (
	"context"
	"path"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestModuleContext_WithContext(t *testing.T) {
	type key string
	tests := []struct {
		name       string
		mod        *ModuleContext
		ctx        context.Context
		expectSame bool
	}{
		{
			name:       "nil->nil: same",
			mod:        &ModuleContext{},
			ctx:        nil,
			expectSame: true,
		},
		{
			name:       "nil->ctx: not same",
			mod:        &ModuleContext{},
			ctx:        context.WithValue(context.Background(), key("a"), "b"),
			expectSame: false,
		},
		{
			name:       "ctx->nil: same",
			mod:        &ModuleContext{ctx: context.Background()},
			ctx:        nil,
			expectSame: true,
		},
		{
			name:       "ctx1->ctx2: not same",
			mod:        &ModuleContext{ctx: context.Background()},
			ctx:        context.WithValue(context.Background(), key("a"), "b"),
			expectSame: false,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			mod2 := tc.mod.WithContext(tc.ctx)
			if tc.expectSame {
				require.Same(t, tc.mod, mod2)
			} else {
				require.NotSame(t, tc.mod, mod2)
				require.Equal(t, tc.ctx, mod2.Context())
			}
		})
	}
}

func TestModuleContext_WithMemory(t *testing.T) {
	tests := []struct {
		name       string
		mod        *ModuleContext
		mem        *MemoryInstance
		expectSame bool
	}{
		{
			name:       "nil->nil: same",
			mod:        &ModuleContext{},
			mem:        nil,
			expectSame: true,
		},
		{
			name:       "nil->mem: not same",
			mod:        &ModuleContext{},
			mem:        &MemoryInstance{},
			expectSame: false,
		},
		{
			name:       "mem->nil: same",
			mod:        &ModuleContext{memory: &MemoryInstance{}},
			mem:        nil,
			expectSame: true,
		},
		{
			name:       "mem1->mem2: not same",
			mod:        &ModuleContext{memory: &MemoryInstance{}},
			mem:        &MemoryInstance{},
			expectSame: false,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			mod2 := tc.mod.WithMemory(tc.mem)
			if tc.expectSame {
				require.Same(t, tc.mod, mod2)
			} else {
				require.NotSame(t, tc.mod, mod2)
				require.Equal(t, tc.mem, mod2.memory)
			}
		})
	}
}

func TestModuleContext_String(t *testing.T) {
	s := newStore()

	tests := []struct {
		name, moduleName, expected string
	}{
		{
			name:       "empty",
			moduleName: "",
			expected:   "Module[]",
		},
		{
			name:       "not empty",
			moduleName: "math",
			expected:   "Module[math]",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			// Ensure paths that can create the host module can see the name.
			m, err := s.Instantiate(context.Background(), &Module{}, tc.moduleName, nil)
			defer m.Close() //nolint

			require.NoError(t, err)
			require.Equal(t, tc.expected, m.String())
			require.Equal(t, tc.expected, s.Module(m.Name()).String())
		})
	}
}

func TestModuleContext_Close(t *testing.T) {
	s := newStore()

	t.Run("calls store.CloseWithExitCode(module.name)", func(t *testing.T) {
		moduleName := t.Name()
		m, err := s.Instantiate(context.Background(), &Module{}, moduleName, nil)
		require.NoError(t, err)

		// We use side effects to determine if Close in fact called store.CloseWithExitCode (without repeating store_test.go).
		// One side effect of store.CloseWithExitCode is that the moduleName can no longer be looked up. Verify our base case.
		require.Equal(t, s.Module(moduleName), m)

		// Closing should not err.
		require.NoError(t, m.Close())

		// Verify our intended side-effect
		require.Nil(t, s.Module(moduleName))

		// Verify no error closing again.
		require.NoError(t, m.Close())
	})

	t.Run("calls SysContext.Close()", func(t *testing.T) {
		tempDir := t.TempDir()
		pathName := "test"
		file, _ := createWriteableFile(t, tempDir, pathName, make([]byte, 0))

		sys, err := NewSysContext(
			0,   // max
			nil, // args
			nil, // environ
			nil, // stdin
			nil, // stdout
			nil, // stderr
			map[uint32]*FileEntry{ // openedFiles
				3: {Path: "."},
				4: {Path: path.Join(".", pathName), File: file},
			},
		)
		require.NoError(t, err)

		moduleName := t.Name()
		m, err := s.Instantiate(context.Background(), &Module{}, moduleName, sys)
		require.NoError(t, err)

		// We use side effects to determine if Close in fact called SysContext.Close (without repeating sys_test.go).
		// One side effect of SysContext.Close is that it clears the openedFiles map. Verify our base case.
		require.NotEmpty(t, sys.openedFiles)

		// Closing should not err.
		require.NoError(t, m.Close())

		// Verify our intended side-effect
		require.Empty(t, sys.openedFiles)

		// Verify no error closing again.
		require.NoError(t, m.Close())
	})
}

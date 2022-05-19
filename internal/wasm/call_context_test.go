package wasm

import (
	"context"
	"fmt"
	"path"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestCallContext_WithMemory(t *testing.T) {
	tests := []struct {
		name       string
		mod        *CallContext
		mem        *MemoryInstance
		expectSame bool
	}{
		{
			name:       "nil->nil: same",
			mod:        &CallContext{},
			mem:        nil,
			expectSame: true,
		},
		{
			name:       "nil->mem: not same",
			mod:        &CallContext{},
			mem:        &MemoryInstance{},
			expectSame: false,
		},
		{
			name:       "mem->nil: same",
			mod:        &CallContext{memory: &MemoryInstance{}},
			mem:        nil,
			expectSame: true,
		},
		{
			name:       "mem1->mem2: not same",
			mod:        &CallContext{memory: &MemoryInstance{}},
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

func TestCallContext_String(t *testing.T) {
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
			m, err := s.Instantiate(context.Background(), &Module{}, tc.moduleName, nil, nil)
			defer m.Close(testCtx) //nolint

			require.NoError(t, err)
			require.Equal(t, tc.expected, m.String())
			require.Equal(t, tc.expected, s.Module(m.Name()).String())
		})
	}
}

func TestCallContext_Close(t *testing.T) {
	s := newStore()

	tests := []struct {
		name           string
		closer         func(context.Context, *CallContext) error
		expectedClosed uint64
	}{
		{
			name: "Close()",
			closer: func(ctx context.Context, callContext *CallContext) error {
				return callContext.Close(ctx)
			},
			expectedClosed: uint64(1),
		},
		{
			name: "CloseWithExitCode(255)",
			closer: func(ctx context.Context, callContext *CallContext) error {
				return callContext.CloseWithExitCode(ctx, 255)
			},
			expectedClosed: uint64(255)<<32 + 1,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(fmt.Sprintf("%s calls store.CloseWithExitCode(module.name))", tc.name), func(t *testing.T) {
			for _, ctx := range []context.Context{nil, testCtx} { // Ensure it doesn't crash on nil!
				moduleName := t.Name()
				m, err := s.Instantiate(ctx, &Module{}, moduleName, nil, nil)
				require.NoError(t, err)

				// We use side effects to see if Close called store.CloseWithExitCode (without repeating store_test.go).
				// One side effect of store.CloseWithExitCode is that the moduleName can no longer be looked up.
				require.Equal(t, s.Module(moduleName), m)

				// Closing should not err.
				require.NoError(t, tc.closer(ctx, m))

				require.Equal(t, tc.expectedClosed, *m.closed)

				// Verify our intended side-effect
				require.Nil(t, s.Module(moduleName))

				// Verify no error closing again.
				require.NoError(t, tc.closer(ctx, m))
			}
		})
	}

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
			nil, // randSource
			map[uint32]*FileEntry{ // openedFiles
				3: {Path: "."},
				4: {Path: path.Join(".", pathName), File: file},
			},
		)
		require.NoError(t, err)

		fsCtx := sys.FS()

		moduleName := t.Name()
		m, err := s.Instantiate(context.Background(), &Module{}, moduleName, sys, nil)
		require.NoError(t, err)

		// We use side effects to determine if Close in fact called SysContext.Close (without repeating sys_test.go).
		// One side effect of SysContext.Close is that it clears the openedFiles map. Verify our base case.
		require.True(t, len(fsCtx.openedFiles) > 0, "sys.openedFiles was empty")

		// Closing should not err.
		require.NoError(t, m.Close(testCtx))

		// Verify our intended side-effect
		require.Equal(t, 0, len(fsCtx.openedFiles), "expected no opened files")

		// Verify no error closing again.
		require.NoError(t, m.Close(testCtx))
	})
}

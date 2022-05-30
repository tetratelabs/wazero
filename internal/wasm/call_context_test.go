package wasm

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"testing"

	"github.com/tetratelabs/wazero/internal/sys"
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
	s, ns := newStore()

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
			m, err := s.Instantiate(context.Background(), ns, &Module{}, tc.moduleName, nil, nil)
			defer m.Close(testCtx) //nolint

			require.NoError(t, err)
			require.Equal(t, tc.expected, m.String())
			require.Equal(t, tc.expected, ns.Module(m.Name()).String())
		})
	}
}

func TestCallContext_Close(t *testing.T) {
	s, ns := newStore()

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
		t.Run(fmt.Sprintf("%s calls ns.CloseWithExitCode(module.name))", tc.name), func(t *testing.T) {
			for _, ctx := range []context.Context{nil, testCtx} { // Ensure it doesn't crash on nil!
				moduleName := t.Name()
				m, err := s.Instantiate(ctx, ns, &Module{}, moduleName, nil, nil)
				require.NoError(t, err)

				// We use side effects to see if Close called ns.CloseWithExitCode (without repeating store_test.go).
				// One side effect of ns.CloseWithExitCode is that the moduleName can no longer be looked up.
				require.Equal(t, ns.Module(moduleName), m)

				// Closing should not err.
				require.NoError(t, tc.closer(ctx, m))

				require.Equal(t, tc.expectedClosed, *m.closed)

				// Verify our intended side-effect
				require.Nil(t, ns.Module(moduleName))

				// Verify no error closing again.
				require.NoError(t, tc.closer(ctx, m))
			}
		})
	}

	t.Run("calls SysContext.Close()", func(t *testing.T) {
		sysCtx := DefaultSysContext()
		sysCtx.FS().OpenFile(&sys.FileEntry{Path: "."})

		m, err := s.Instantiate(context.Background(), ns, &Module{}, t.Name(), sysCtx, nil)
		require.NoError(t, err)

		// We use side effects to determine if Close in fact called SysContext.Close (without repeating sys_test.go).
		// One side effect of SysContext.Close is that it clears the openedFiles map. Verify our base case.
		fsCtx := sysCtx.FS()
		_, ok := fsCtx.OpenedFile(3)
		require.True(t, ok, "sysCtx.openedFiles was empty")

		// Closing should not err.
		require.NoError(t, m.Close(testCtx))

		// Verify our intended side-effect
		_, ok = fsCtx.OpenedFile(3)
		require.False(t, ok, "expected no opened files")

		// Verify no error closing again.
		require.NoError(t, m.Close(testCtx))
	})

	t.Run("error closing", func(t *testing.T) {
		// Right now, the only way to err closing the sys context is if a File.Close erred.
		sysCtx := DefaultSysContext()
		sysCtx.FS().OpenFile(&sys.FileEntry{Path: ".", File: &testFile{errors.New("error closing")}})

		m, err := s.Instantiate(context.Background(), ns, &Module{}, t.Name(), sysCtx, nil)
		require.NoError(t, err)

		require.EqualError(t, m.Close(testCtx), "error closing")

		// Verify our intended side-effect
		_, ok := sysCtx.FS().OpenedFile(3)
		require.False(t, ok, "expected no opened files")
	})
}

// compile-time check to ensure testFile implements fs.File
var _ fs.File = &testFile{}

type testFile struct{ closeErr error }

func (f *testFile) Close() error                       { return f.closeErr }
func (f *testFile) Stat() (fs.FileInfo, error)         { return nil, nil }
func (f *testFile) Read(_ []byte) (int, error)         { return 0, nil }
func (f *testFile) Seek(_ int64, _ int) (int64, error) { return 0, nil }

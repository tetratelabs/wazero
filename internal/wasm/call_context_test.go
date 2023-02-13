package wasm

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	internalsys "github.com/tetratelabs/wazero/internal/sys"
	"github.com/tetratelabs/wazero/internal/sysfs"
	testfs "github.com/tetratelabs/wazero/internal/testing/fs"
	"github.com/tetratelabs/wazero/internal/testing/hammer"
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
			m, err := s.Instantiate(testCtx, &Module{}, tc.moduleName, nil)
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
		t.Run(fmt.Sprintf("%s calls ns.CloseWithExitCode(module.name))", tc.name), func(t *testing.T) {
			for _, ctx := range []context.Context{nil, testCtx} { // Ensure it doesn't crash on nil!
				moduleName := t.Name()
				m, err := s.Instantiate(ctx, &Module{}, moduleName, nil)
				require.NoError(t, err)

				// We use side effects to see if Close called ns.CloseWithExitCode (without repeating store_test.go).
				// One side effect of ns.CloseWithExitCode is that the moduleName can no longer be looked up.
				require.Equal(t, s.Module(moduleName), m)

				// Closing should not err.
				require.NoError(t, tc.closer(ctx, m))

				require.Equal(t, tc.expectedClosed, *m.Closed)

				// Verify our intended side-effect
				require.Nil(t, s.Module(moduleName))

				// Verify no error closing again.
				require.NoError(t, tc.closer(ctx, m))
			}
		})
	}

	t.Run("calls Context.Close()", func(t *testing.T) {
		for _, concurrent := range []bool{false, true} {
			concurrent := concurrent
			t.Run(fmt.Sprintf("concurrent=%v", concurrent), func(t *testing.T) {
				testFS := sysfs.Adapt(testfs.FS{"foo": &testfs.File{}})
				sysCtx := internalsys.DefaultContext(testFS)
				fsCtx := sysCtx.FS()

				_, err := fsCtx.OpenFile(testFS, "/foo", os.O_RDONLY, 0)
				require.NoError(t, err)

				m, err := s.Instantiate(testCtx, &Module{}, t.Name(), sysCtx)
				require.NoError(t, err)

				// We use side effects to determine if Close in fact called Context.Close (without repeating sys_test.go).
				// One side effect of Context.Close is that it clears the openedFiles map. Verify our base case.
				_, ok := fsCtx.LookupFile(3)
				require.True(t, ok, "sysCtx.openedFiles was empty")

				// Closing should not err.
				if concurrent {
					hammer.NewHammer(t, 100, 10).Run(func(name string) {
						require.NoError(t, m.Close(testCtx))
						// closeWithExitCode is the one called during Store.CloseWithExitCode.
						require.NoError(t, m.closeWithExitCode(testCtx, 0))
					}, nil)
					if t.Failed() {
						return // At least one test failed, so return now.
					}
				} else {
					require.NoError(t, m.Close(testCtx))
				}

				// Verify our intended side-effect
				_, ok = fsCtx.LookupFile(3)
				require.False(t, ok, "expected no opened files")

				// Verify no error closing again.
				require.NoError(t, m.Close(testCtx))
			})
		}
	})

	t.Run("error closing", func(t *testing.T) {
		// Right now, the only way to err closing the sys context is if a File.Close erred.
		testFS := sysfs.Adapt(testfs.FS{"foo": &testfs.File{CloseErr: errors.New("error closing")}})
		sysCtx := internalsys.DefaultContext(testFS)
		fsCtx := sysCtx.FS()

		_, err := fsCtx.OpenFile(testFS, "/foo", os.O_RDONLY, 0)
		require.NoError(t, err)

		m, err := s.Instantiate(testCtx, &Module{}, t.Name(), sysCtx)
		require.NoError(t, err)

		require.EqualError(t, m.Close(testCtx), "error closing")

		// Verify our intended side-effect
		_, ok := fsCtx.LookupFile(3)
		require.False(t, ok, "expected no opened files")
	})
}

func TestCallContext_CallDynamic(t *testing.T) {
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
		t.Run(fmt.Sprintf("%s calls ns.CloseWithExitCode(module.name))", tc.name), func(t *testing.T) {
			for _, ctx := range []context.Context{nil, testCtx} { // Ensure it doesn't crash on nil!
				moduleName := t.Name()
				m, err := s.Instantiate(ctx, &Module{}, moduleName, nil)
				require.NoError(t, err)

				// We use side effects to see if Close called ns.CloseWithExitCode (without repeating store_test.go).
				// One side effect of ns.CloseWithExitCode is that the moduleName can no longer be looked up.
				require.Equal(t, s.Module(moduleName), m)

				// Closing should not err.
				require.NoError(t, tc.closer(ctx, m))

				require.Equal(t, tc.expectedClosed, *m.Closed)

				// Verify our intended side-effect
				require.Nil(t, s.Module(moduleName))

				// Verify no error closing again.
				require.NoError(t, tc.closer(ctx, m))
			}
		})
	}

	t.Run("calls Context.Close()", func(t *testing.T) {
		testFS := sysfs.Adapt(testfs.FS{"foo": &testfs.File{}})
		sysCtx := internalsys.DefaultContext(testFS)
		fsCtx := sysCtx.FS()

		_, err := fsCtx.OpenFile(testFS, "/foo", os.O_RDONLY, 0)
		require.NoError(t, err)

		m, err := s.Instantiate(testCtx, &Module{}, t.Name(), sysCtx)
		require.NoError(t, err)

		// We use side effects to determine if Close in fact called Context.Close (without repeating sys_test.go).
		// One side effect of Context.Close is that it clears the openedFiles map. Verify our base case.
		_, ok := fsCtx.LookupFile(3)
		require.True(t, ok, "sysCtx.openedFiles was empty")

		// Closing should not err.
		require.NoError(t, m.Close(testCtx))

		// Verify our intended side-effect
		_, ok = fsCtx.LookupFile(3)
		require.False(t, ok, "expected no opened files")

		// Verify no error closing again.
		require.NoError(t, m.Close(testCtx))
	})

	t.Run("error closing", func(t *testing.T) {
		// Right now, the only way to err closing the sys context is if a File.Close erred.
		testFS := sysfs.Adapt(testfs.FS{"foo": &testfs.File{CloseErr: errors.New("error closing")}})
		sysCtx := internalsys.DefaultContext(testFS)
		fsCtx := sysCtx.FS()

		path := "/foo"
		_, err := fsCtx.OpenFile(testFS, path, os.O_RDONLY, 0)
		require.NoError(t, err)

		m, err := s.Instantiate(testCtx, &Module{}, t.Name(), sysCtx)
		require.NoError(t, err)

		require.EqualError(t, m.Close(testCtx), "error closing")

		// Verify our intended side-effect
		_, ok := fsCtx.LookupFile(3)
		require.False(t, ok, "expected no opened files")
	})
}

func TestCallContext_CloseModuleOnCanceledOrTimeout(t *testing.T) {
	s := newStore()
	t.Run("timeout", func(t *testing.T) {
		cc := &CallContext{Closed: new(uint64), module: &ModuleInstance{Name: "test"}, s: s}
		const duration = time.Second
		ctx, cancel := context.WithTimeout(context.Background(), duration)
		defer cancel()
		done := cc.CloseModuleOnCanceledOrTimeout(context.WithValue(ctx, struct{}{}, 1)) // Wrapping arbitrary context.
		time.Sleep(duration * 2)
		defer done()

		err := cc.FailIfClosed()
		require.EqualError(t, err, "module \"test\" closed with context deadline exceeded")
	})

	t.Run("cancel", func(t *testing.T) {
		cc := &CallContext{Closed: new(uint64), module: &ModuleInstance{Name: "test"}, s: s}
		ctx, cancel := context.WithCancel(context.Background())
		done := cc.CloseModuleOnCanceledOrTimeout(context.WithValue(ctx, struct{}{}, 1)) // Wrapping arbitrary context.
		cancel()
		// Make sure nothing panics or otherwise gets weird with redundant call to cancel().
		cancel()
		cancel()
		defer done()

		time.Sleep(time.Second)
		err := cc.FailIfClosed()
		require.EqualError(t, err, "module \"test\" closed with context canceled")
	})

	t.Run("timeout over cancel", func(t *testing.T) {
		cc := &CallContext{Closed: new(uint64), module: &ModuleInstance{Name: "test"}, s: s}
		const duration = time.Second
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		// Wrap the cancel context by timeout.
		ctx, cancel = context.WithTimeout(ctx, duration)
		defer cancel()
		done := cc.CloseModuleOnCanceledOrTimeout(context.WithValue(ctx, struct{}{}, 1)) // Wrapping arbitrary context.
		time.Sleep(duration * 2)
		defer done()
	})

	t.Run("cancel over timeout", func(t *testing.T) {
		cc := &CallContext{Closed: new(uint64), module: &ModuleInstance{Name: "test"}, s: s}
		ctx, cancel := context.WithCancel(context.Background())
		// Wrap the timeout context by cancel context.
		var timeoutDone context.CancelFunc
		ctx, timeoutDone = context.WithTimeout(ctx, time.Second*1000)
		defer timeoutDone()

		done := cc.CloseModuleOnCanceledOrTimeout(context.WithValue(ctx, struct{}{}, 1)) // Wrapping arbitrary context.
		cancel()
		defer done()

		time.Sleep(time.Second)
		err := cc.FailIfClosed()
		require.EqualError(t, err, "module \"test\" closed with context canceled")
	})

	t.Run("cancel works", func(t *testing.T) {
		cc := &CallContext{Closed: new(uint64), module: &ModuleInstance{Name: "test"}, s: s}
		goroutineDone, cancelFn := context.WithCancel(context.Background())
		fn := cc.closeModuleOnCanceledOrTimeoutClosure(context.Background(), goroutineDone)
		var wg sync.WaitGroup
		wg.Add(1)

		// Ensure that fn returned by closeModuleOnCanceledOrTimeoutClosure exists after cancelFn is called.
		go func() {
			defer wg.Done()
			fn()
		}()
		cancelFn()
		wg.Wait()
	})
}

type mockCloser struct{ called int }

func (m *mockCloser) Close(context.Context) error {
	m.called++
	return nil
}

func TestCallContext_ensureResourcesClosed(t *testing.T) {
	closer := &mockCloser{}

	for _, tc := range []struct {
		name string
		m    *CallContext
	}{
		{m: &CallContext{CodeCloser: closer}},
		{m: &CallContext{Sys: internalsys.DefaultContext(nil)}},
		{m: &CallContext{Sys: internalsys.DefaultContext(nil), CodeCloser: closer}},
	} {
		err := tc.m.ensureResourcesClosed(context.Background())
		require.NoError(t, err)
		require.Nil(t, tc.m.Sys)
		require.Nil(t, tc.m.CodeCloser)

		// Ensure multiple invocation is safe.
		err = tc.m.ensureResourcesClosed(context.Background())
		require.NoError(t, err)
	}
	require.Equal(t, 2, closer.called)
}

// Package issue2558 is a regression test for
// https://github.com/tetratelabs/wazero/issues/2558
//
// With WithCloseOnContextDone(true), a context canceled while the guest is in a
// host call closes the module with exitCodeFlagResourceNotClosed, deferring
// resource closure to FailIfClosed. The wazevo compiler only calls FailIfClosed
// on the normal (non-panic) return path of callWithStack, so a guest that traps
// right after the canceled host call never releases its linear memory:
// LinearMemory.Free is never called and a later mod.Close is a no-op
// (setExitCode's CAS fails because Closed is already set).
//
// guest.wasm (testdata) is built with wat2wasm from:
//
//	(module
//	  (import "host" "cancel" (func $cancel))
//	  (import "wasi_snapshot_preview1" "proc_exit" (func $proc_exit (param i32)))
//	  (memory 1)
//	  (func (export "trap") (call $cancel) unreachable)
//	  (func (export "exit") (call $cancel) (call $proc_exit (i32.const 0)))
//	)
package issue2558

import (
	"context"
	"embed"
	"testing"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

//go:embed testdata/guest.wasm
var testdata embed.FS

// trackingMemory is a LinearMemory that records whether Free was called.
type trackingMemory struct {
	buf   []byte
	freed bool
}

func (m *trackingMemory) Reallocate(size uint64) []byte {
	if size > uint64(len(m.buf)) {
		m.buf = append(m.buf, make([]byte, size-uint64(len(m.buf)))...)
	}
	return m.buf[:size]
}

func (m *trackingMemory) Free() { m.freed = true }

// runCase instantiates the guest with a tracking allocator and
// WithCloseOnContextDone, cancels the context during the "cancel" host call,
// waits until the module is actually closed by the context watcher, then
// unwinds via the given export. It returns whether the linear memory's Free
// was called by the time the invocation returned.
func runCase(t *testing.T, engine wazero.RuntimeConfig, export string) (freed bool) {
	t.Helper()

	var mem *trackingMemory
	allocator := experimental.MemoryAllocatorFunc(func(cap, max uint64) experimental.LinearMemory {
		mem = &trackingMemory{buf: make([]byte, cap)}
		return mem
	})

	r := wazero.NewRuntimeWithConfig(context.Background(), engine.WithCloseOnContextDone(true))
	defer r.Close(context.Background())

	ctx, cancel := context.WithCancel(context.Background())

	// The host "cancel" function cancels the caller's context while the guest
	// is inside this host call, then waits until the module is closed by the
	// watcher so the exitCodeFlagResourceNotClosed state is deterministic
	// before the guest unwinds.
	wasi_snapshot_preview1.MustInstantiate(ctx, r)

	var guest api.Module
	_, err := r.NewHostModuleBuilder("host").
		NewFunctionBuilder().
		WithGoModuleFunction(api.GoModuleFunc(func(ctx context.Context, _ api.Module, _ []uint64) {
			cancel()
			deadline := time.Now().Add(10 * time.Second)
			for guest == nil || !guest.IsClosed() {
				if time.Now().After(deadline) {
					break
				}
				time.Sleep(time.Millisecond)
			}
		}), nil, nil).Export("cancel").
		Instantiate(ctx)
	require.NoError(t, err)

	var err2 error
	guest, err2 = r.InstantiateWithConfig(
		experimental.WithMemoryAllocator(ctx, allocator), testdataGuest(t),
		wazero.NewModuleConfig().WithName("guest"))
	require.NoError(t, err2)

	// The guest's export calls "cancel" first, then traps or exits.
	_, err = guest.ExportedFunction(export).Call(ctx)
	require.Error(t, err, "call must return an error after the module was closed")

	require.True(t, guest.IsClosed(), "module must be closed by the context watcher when the call returned")

	// The module was closed with exitCodeFlagResourceNotClosed, so resource
	// closure was deferred to FailIfClosed, which must have run by now.
	return mem.freed
}

func testdataGuest(t *testing.T) []byte {
	t.Helper()
	b, err := testdata.ReadFile("testdata/guest.wasm")
	require.NoError(t, err)
	return b
}

func TestResourceLeakAfterCanceledHostCall(t *testing.T) {
	for _, tc := range []struct {
		name   string
		engine wazero.RuntimeConfig
		export string
	}{
		{"compiler/trap", wazero.NewRuntimeConfigCompiler(), "trap"},
		{"compiler/proc_exit", wazero.NewRuntimeConfigCompiler(), "exit"},
		{"interpreter/trap", wazero.NewRuntimeConfigInterpreter(), "trap"},
		{"interpreter/proc_exit", wazero.NewRuntimeConfigInterpreter(), "exit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.True(t, runCase(t, tc.engine, tc.export),
				"linear memory Free must be called once the module was closed by the context watcher")
		})
	}
}

package wasi_snapshot_preview1

import (
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/watzero"
	"github.com/tetratelabs/wazero/sys"
)

func Test_ProcExit(t *testing.T) {
	tests := []struct {
		name     string
		exitCode uint32
	}{
		{
			name:     "success (exitcode 0)",
			exitCode: 0,
		},
		{
			name:     "arbitrary non-zero exitcode",
			exitCode: 42,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			// Note: Unlike most tests, this uses fn, not the 'a' result
			// parameter. This is because currently, this function body
			// panics, and we expect Call to unwrap the panic.
			mod, fn := instantiateModule(testCtx, t, functionProcExit, importProcExit, nil)
			defer mod.Close(testCtx)

			// When ProcExit is called, CallEngine.Call returns immediately,
			// returning the exit code as the error.
			_, err := fn.Call(testCtx, uint64(tc.exitCode))
			require.Equal(t, tc.exitCode, err.(*sys.ExitError).ExitCode())
		})
	}
}

var unreachableAfterExit = `(module
  (import "wasi_snapshot_preview1" "proc_exit"
    (func $wasi.proc_exit (param $rval i32)))
  (func $main
    i32.const 0
    call $wasi.proc_exit
	unreachable ;; If abort doesn't panic, this code is reached.
  )
  (start $main)
)`

// Test_ProcExit_StopsExecution ensures code that follows a proc_exit isn't invoked.
func Test_ProcExit_StopsExecution(t *testing.T) {
	r := wazero.NewRuntime()
	defer r.Close(testCtx)

	_, err := NewBuilder(r).Instantiate(testCtx, r)
	require.NoError(t, err)

	exitWasm, err := watzero.Wat2Wasm(unreachableAfterExit)
	require.NoError(t, err)

	_, err = r.InstantiateModuleFromBinary(testCtx, exitWasm)
	require.Error(t, err)
	require.Equal(t, uint32(0), err.(*sys.ExitError).ExitCode())
}

// Test_ProcRaise only tests it is stubbed for GrainLang per #271
func Test_ProcRaise(t *testing.T) {
	mod, fn := instantiateModule(testCtx, t, functionProcRaise, importProcRaise, nil)
	defer mod.Close(testCtx)

	t.Run("wasi.ProcRaise", func(t *testing.T) {
		errno := a.ProcRaise(testCtx, mod, 0)
		require.Equal(t, ErrnoNosys, errno, ErrnoName(errno))
	})

	t.Run(functionProcRaise, func(t *testing.T) {
		results, err := fn.Call(testCtx, 0)
		require.NoError(t, err)
		errno := Errno(results[0]) // results[0] is the errno
		require.Equal(t, ErrnoNosys, errno, ErrnoName(errno))
	})
}

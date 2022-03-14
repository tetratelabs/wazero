package adhoc

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tetratelabs/wazero"
)

func TestJITConcurrency(t *testing.T) {
	if !wazero.JITSupported {
		t.Skip()
	}
	runAdhocTestsUnderHighConcurrency(t, wazero.NewRuntimeConfigJIT)
	singleModuleHighConcurrency(t, wazero.NewRuntimeConfigJIT)
}

func TestInterpreterConcurrency(t *testing.T) {
	runAdhocTestsUnderHighConcurrency(t, wazero.NewRuntimeConfigInterpreter)
	singleModuleHighConcurrency(t, wazero.NewRuntimeConfigJIT)
}

func runAdhocTestsUnderHighConcurrency(t *testing.T, newRuntimeConfig func() *wazero.RuntimeConfig) {
	t.Run("huge stack", func(t *testing.T) {
		t.Parallel()
		runAdhocTestUnderHighConcurrency(t, newRuntimeConfig, testHugeStack)
	})
	t.Run("unreachable", func(t *testing.T) {
		t.Parallel()
		runAdhocTestUnderHighConcurrency(t, newRuntimeConfig, testUnreachable)
	})
	t.Run("recursive entry", func(t *testing.T) {
		t.Parallel()
		runAdhocTestUnderHighConcurrency(t, newRuntimeConfig, testRecursiveEntry)
	})
	t.Run("imported-and-exported func", func(t *testing.T) {
		t.Parallel()
		runAdhocTestUnderHighConcurrency(t, newRuntimeConfig, testImportedAndExportedFunc)
	})
}

// runAdhocTestUnderHighConcurrency runs a test case in adhoc_test.go with multiple goroutines.
func runAdhocTestUnderHighConcurrency(t *testing.T, newRuntimeConfig func() *wazero.RuntimeConfig,
	adhocTest func(t *testing.T, newRuntimeConfig func() *wazero.RuntimeConfig)) {
	const goroutinesPerCase = 1000
	var wg sync.WaitGroup
	wg.Add(goroutinesPerCase)
	for i := 0; i < goroutinesPerCase; i++ {
		go func() {
			defer wg.Done()
			adhocTest(t, newRuntimeConfig)
		}()
	}
	wg.Wait()
}

func singleModuleHighConcurrency(t *testing.T, newRuntimeConfig func() *wazero.RuntimeConfig) {
	t.Run("single module", func(t *testing.T) {
		r := wazero.NewRuntimeWithConfig(newRuntimeConfig())
		imported, err := r.NewModuleBuilder("host").ExportFunctions(map[string]interface{}{
			"delay": func() { time.Sleep(100 * time.Millisecond) },
		}).Instantiate()
		require.NoError(t, err)

		source := []byte(`(module
			(import "host" "delay" (func $delay ))
			(func $delay_add
				(param $value_1 i32) (param $value_2 i32)
				(result i32)
				local.get 0
				local.get 1
				i32.add
				call $delay
			)
			(export "delay_add" (func $delay_add))
		)`)

		module, err := r.InstantiateModuleFromSource(source)
		require.NoError(t, err)

		args := []uint64{1, 123}
		exp := args[0] + args[1]

		t.Run("close importing module while in use", func(t *testing.T) {
			fn := module.ExportedFunction("delay_add")
			require.NotNil(t, fn)

			var wg sync.WaitGroup
			const goroutines = 1000
			wg.Add(goroutines)
			for i := 0; i < goroutines; i++ {
				if i == 200 {
					// Close the importing module.
					module.Close()
				} else if i == 400 {
					// Re-instantiate the importing module, and swap the function.
					module, err = r.InstantiateModuleFromSource(source)
					require.NoError(t, err)
					fn = module.ExportedFunction("delay_add")
					require.NotNil(t, fn)
				}
				go func() {
					defer wg.Done()
					res, err := fn.Call(nil, args...)
					require.NoError(t, err)
					require.Equal(t, exp, res[0])
				}()
			}
			wg.Wait()
		})
		t.Run("close imported module while in use", func(t *testing.T) {
			fn := module.ExportedFunction("delay_add")
			require.NotNil(t, fn)

			var newImportedModuleShouldPanic bool = true
			var wg sync.WaitGroup
			const goroutines = 1000
			wg.Add(goroutines)
			for i := 0; i < goroutines; i++ {
				if i == 200 {
					// Close the imported module.
					imported.Close()
					// Re-instantiate the imported module but at this point importing module should
					// not use the new one.
					imported, err = r.NewModuleBuilder("host").ExportFunctions(map[string]interface{}{
						"delay": func() {
							if newImportedModuleShouldPanic {
								panic("unreachable")
							} else {
								time.Sleep(10 * time.Millisecond)
							}
						},
					}).Instantiate()
					require.NoError(t, err)
				} else if i == 400 {
					// Re-instantiate the importing module, and swap the function.
					module.Close()
					module, err = r.InstantiateModuleFromSource(source)
					require.NoError(t, err)
					fn = module.ExportedFunction("delay_add")
					require.NotNil(t, fn)
					// The new imported module is now in use.
					newImportedModuleShouldPanic = false
				}
				go func() {
					defer wg.Done()
					res, err := fn.Call(nil, 1, 123)
					require.NoError(t, err)
					require.Equal(t, exp, res[0])
				}()
			}
			wg.Wait()
		})
	})
}

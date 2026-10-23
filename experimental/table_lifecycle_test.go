package experimental_test

import (
	"context"
	"fmt"
	"math"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/testing/binaryencoding"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

func TestTableLimiterLifecycle(t *testing.T) {
	engines := []struct {
		name   string
		config wazero.RuntimeConfig
	}{{"interpreter", wazero.NewRuntimeConfigInterpreter()}}
	if platform.CompilerSupported() {
		engines = append(engines, struct {
			name   string
			config wazero.RuntimeConfig
		}{"compiler", wazero.NewRuntimeConfigCompiler()})
	}
	for _, engine := range engines {
		t.Run(engine.name, func(t *testing.T) {
			ctx := context.Background()
			t.Run("concurrent shared admission", func(t *testing.T) {
				budget := newTableBudget(8)
				func() {
					r := wazero.NewRuntimeWithConfig(ctx, engine.config)
					defer r.Close(ctx)
					limited := experimental.WithTableLimiter(ctx, budget)
					binary := binaryencoding.EncodeModule(tableGrowModule(1, nil, false))
					results := make(chan error, 16)
					var wg sync.WaitGroup
					for i := 0; i < 16; i++ {
						wg.Add(1)
						go func(i int) {
							defer wg.Done()
							_, err := r.InstantiateWithConfig(limited, binary, wazero.NewModuleConfig().WithName(fmt.Sprintf("module-%d", i)))
							results <- err
						}(i)
					}
					wg.Wait()
					close(results)
					admitted := 0
					for err := range results {
						if err == nil {
							admitted++
						} else {
							require.EqualError(t, err, "table[0] allocation of 1 elements denied by table limiter")
						}
					}
					require.Equal(t, 8, admitted)
					requests := make([][3]uint32, 16)
					for i := range requests {
						requests[i] = [3]uint32{0, 1, math.MaxUint32}
					}
					budget.check(t, 8, requests...)
					runtime.KeepAlive(r)
				}()
				awaitTableReleases(t, budget, 8, 1)
			})
			t.Run("unlimited import stays unlimited", func(t *testing.T) {
				r := wazero.NewRuntimeWithConfig(ctx, engine.config)
				defer r.Close(ctx)
				_, err := r.InstantiateWithConfig(ctx, binaryencoding.EncodeModule(tableGrowModule(1, nil, false)), wazero.NewModuleConfig().WithName("owner"))
				require.NoError(t, err)
				budget := newTableBudget(0)
				alias, err := r.InstantiateWithConfig(experimental.WithTableLimiter(ctx, budget), binaryencoding.EncodeModule(tableGrowModule(1, nil, true)), wazero.NewModuleConfig().WithName("alias"))
				require.NoError(t, err)
				result, err := alias.ExportedFunction("grow").Call(ctx, 2)
				require.NoError(t, err)
				require.Equal(t, []uint64{1}, result)
				budget.check(t, 0)
			})
			t.Run("partial admission refunds every table", func(t *testing.T) {
				budget := newTableBudget(2)
				func() {
					r := wazero.NewRuntimeWithConfig(ctx, engine.config)
					defer r.Close(ctx)
					m := &wasm.Module{TableSection: []wasm.Table{
						{Min: 1, Type: wasm.RefTypeFuncref}, {Min: 1, Type: wasm.RefTypeFuncref}, {Min: 1, Type: wasm.RefTypeFuncref},
					}}
					_, err := r.Instantiate(experimental.WithTableLimiter(ctx, budget), binaryencoding.EncodeModule(m))
					require.EqualError(t, err, "table[2] allocation of 1 elements denied by table limiter")
				}()
				awaitTableReleases(t, budget, 2, 1)
			})
			t.Run("externref growth", func(t *testing.T) {
				budget := newTableBudget(3)
				func() {
					r := wazero.NewRuntimeWithConfig(ctx, engine.config)
					defer r.Close(ctx)
					m := tableGrowModule(1, nil, false)
					m.TableSection[0].Type = wasm.RefTypeExternref
					m.CodeSection[0].Body[1] = byte(wasm.RefTypeExternref)
					mod, err := r.Instantiate(experimental.WithTableLimiter(ctx, budget), binaryencoding.EncodeModule(m))
					require.NoError(t, err)
					result, err := mod.ExportedFunction("grow").Call(ctx, 2)
					require.NoError(t, err)
					require.Equal(t, []uint64{1}, result)
					result, err = mod.ExportedFunction("grow").Call(ctx, 1)
					require.NoError(t, err)
					require.Equal(t, []uint64{math.MaxUint32}, result)
					budget.check(t, 3, [3]uint32{0, 1, math.MaxUint32}, [3]uint32{1, 3, math.MaxUint32}, [3]uint32{3, 4, math.MaxUint32})
				}()
				awaitTableRelease(t, budget, 3)
			})
		})
	}
}

func awaitTableReleases(t *testing.T, budget *tableBudget, count int, elements uint32) {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for received := 0; received < count; {
		runtime.GC()
		select {
		case actual := <-budget.released:
			require.Equal(t, elements, actual)
			received++
		case <-deadline:
			t.Fatalf("received %d of %d table refunds", received, count)
		case <-time.After(10 * time.Millisecond):
		}
	}
	budget.mu.Lock()
	defer budget.mu.Unlock()
	require.Equal(t, uint32(0), budget.used)
}

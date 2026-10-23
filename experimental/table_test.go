package experimental_test

import (
	"context"
	"math"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/testing/binaryencoding"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

func TestTableLimiter(t *testing.T) {
	for _, engine := range []struct {
		name   string
		config wazero.RuntimeConfig
	}{
		{"interpreter", wazero.NewRuntimeConfigInterpreter()},
		{"compiler", wazero.NewRuntimeConfigCompiler()},
	} {
		t.Run(engine.name, func(t *testing.T) {
			if engine.name == "compiler" && !platform.CompilerSupported() {
				t.Skip("compiler not supported")
			}
			ctx := context.Background()
			newRuntime := func(t *testing.T) wazero.Runtime {
				r := wazero.NewRuntimeWithConfig(ctx, engine.config)
				t.Cleanup(func() { require.NoError(t, r.Close(ctx)) })
				return r
			}
			instantiate := func(t *testing.T, r wazero.Runtime, ctx context.Context, name string, m *wasm.Module) api.Module {
				mod, err := r.InstantiateWithConfig(ctx, binaryencoding.EncodeModule(m), wazero.NewModuleConfig().WithName(name))
				require.NoError(t, err)
				return mod
			}
			grow := func(t *testing.T, mod api.Module, delta, expected uint32) {
				// No limiter on this call context: the table retains its limiter.
				results, err := mod.ExportedFunction("grow").Call(ctx, uint64(delta))
				require.NoError(t, err)
				require.Equal(t, []uint64{uint64(expected)}, results)
			}

			t.Run("initial allocation denied", func(t *testing.T) {
				budget := newTableBudget(65535)
				max := uint32(65536)
				limited := experimental.WithTableLimiter(ctx, budget)
				limited = experimental.WithMemoryAllocator(limited, experimental.MemoryAllocatorFunc(func(uint64, uint64) experimental.LinearMemory {
					panic("a table must not use the linear memory allocator")
				}))
				_, err := newRuntime(t).Instantiate(limited, binaryencoding.EncodeModule(tableGrowModule(65536, &max, false)))
				require.EqualError(t, err, "table[0] allocation of 65536 elements denied by table limiter")
				budget.check(t, 0, [3]uint32{0, 65536, 65536})
			})

			t.Run("growth and aggregate admission", func(t *testing.T) {
				r := newRuntime(t)
				budget := newTableBudget(5)
				limited := experimental.WithTableLimiter(ctx, budget)
				mod := instantiate(t, r, limited, "first", tableGrowModule(2, nil, false))
				grow(t, mod, 2, 2)
				grow(t, mod, 2, math.MaxUint32)
				grow(t, mod, 0, 4)
				grow(t, mod, math.MaxUint32, math.MaxUint32)
				instantiate(t, r, limited, "second", tableGrowModule(1, nil, false))
				_, err := r.Instantiate(limited, binaryencoding.EncodeModule(tableGrowModule(1, nil, false)))
				require.EqualError(t, err, "table[0] allocation of 1 elements denied by table limiter")
				budget.check(t, 5,
					[3]uint32{0, 2, math.MaxUint32}, [3]uint32{2, 4, math.MaxUint32},
					[3]uint32{4, 6, math.MaxUint32}, [3]uint32{0, 1, math.MaxUint32},
					[3]uint32{0, 1, math.MaxUint32})
			})

			t.Run("declared maximum precedes limiter", func(t *testing.T) {
				budget := newTableBudget(10)
				max := uint32(3)
				mod := instantiate(t, newRuntime(t), experimental.WithTableLimiter(ctx, budget), "", tableGrowModule(2, &max, false))
				grow(t, mod, 2, math.MaxUint32)
				grow(t, mod, 1, 2)
				budget.check(t, 3, [3]uint32{0, 2, 3}, [3]uint32{2, 3, 3})
			})

			t.Run("import survives owner close", func(t *testing.T) {
				budget, other := newTableBudget(3), newTableBudget(0)
				func() {
					r := wazero.NewRuntimeWithConfig(ctx, engine.config)
					defer r.Close(ctx)
					owner := instantiate(t, r, experimental.WithTableLimiter(ctx, budget), "owner", tableGrowModule(1, nil, false))
					alias := instantiate(t, r, experimental.WithTableLimiter(ctx, other), "alias", tableGrowModule(1, nil, true))
					require.NoError(t, owner.Close(ctx))
					runtime.GC()
					grow(t, alias, 2, 1)
					grow(t, alias, 1, math.MaxUint32)
					other.check(t, 0)
					budget.check(t, 3, [3]uint32{0, 1, math.MaxUint32}, [3]uint32{1, 3, math.MaxUint32}, [3]uint32{3, 4, math.MaxUint32})
				}()
				awaitTableRelease(t, budget, 3)
			})

			t.Run("partial instantiation is collected", func(t *testing.T) {
				budget := newTableBudget(2)
				m := &wasm.Module{TableSection: []wasm.Table{
					{Min: 2, Type: wasm.RefTypeFuncref}, {Min: 1, Type: wasm.RefTypeFuncref},
				}}
				_, err := newRuntime(t).Instantiate(experimental.WithTableLimiter(ctx, budget), binaryencoding.EncodeModule(m))
				require.EqualError(t, err, "table[1] allocation of 1 elements denied by table limiter")
				awaitTableRelease(t, budget, 2)
			})

			t.Run("failed start retains imported module references", func(t *testing.T) {
				budget := newTableBudget(2)
				func() {
					r := wazero.NewRuntimeWithConfig(ctx, engine.config)
					defer r.Close(ctx)
					owner := instantiate(t, r, ctx, "owner", tableGrowModule(1, nil, false))
					zero := uint32(0)
					m := &wasm.Module{
						TypeSection:     []wasm.FunctionType{{}},
						ImportSection:   []wasm.Import{{Module: "owner", Name: "table", Type: wasm.ExternTypeTable, DescTable: wasm.Table{Min: 1, Type: wasm.RefTypeFuncref}}},
						TableSection:    []wasm.Table{{Min: 2, Type: wasm.RefTypeFuncref}},
						FunctionSection: []wasm.Index{0},
						CodeSection:     []wasm.Code{{Body: []byte{wasm.OpcodeUnreachable, wasm.OpcodeEnd}}},
						StartSection:    &zero,
					}
					_, err := r.Instantiate(experimental.WithTableLimiter(ctx, budget), binaryencoding.EncodeModule(m))
					require.Error(t, err)
					require.Contains(t, err.Error(), "unreachable")
					runtime.GC()
					budget.check(t, 2, [3]uint32{0, 2, math.MaxUint32})
					runtime.KeepAlive(owner)
				}()
				awaitTableRelease(t, budget, 2)
			})

			t.Run("empty table is admitted and released", func(t *testing.T) {
				budget := newTableBudget(0)
				func() {
					r := wazero.NewRuntimeWithConfig(ctx, engine.config)
					defer r.Close(ctx)
					instantiate(t, r, experimental.WithTableLimiter(ctx, budget), "", tableGrowModule(0, nil, false))
					budget.check(t, 0, [3]uint32{0, 0, math.MaxUint32})
				}()
				awaitTableRelease(t, budget, 0)
			})
		})
	}
}

func TestWithTableLimiter_nil(t *testing.T) {
	require.Same(t, testCtx, experimental.WithTableLimiter(testCtx, nil))
}

// tableGrowModule exports a table and a function (param i32) (result i32) that
// grows it with null references. Importers use the table exported by "owner".
func tableGrowModule(min uint32, max *uint32, imported bool) *wasm.Module {
	table := wasm.Table{Min: min, Max: max, Type: wasm.RefTypeFuncref}
	m := &wasm.Module{
		TypeSection:     []wasm.FunctionType{{Params: []wasm.ValueType{wasm.ValueTypeI32}, Results: []wasm.ValueType{wasm.ValueTypeI32}}},
		FunctionSection: []wasm.Index{0},
		ExportSection:   []wasm.Export{{Name: "grow", Type: wasm.ExternTypeFunc}, {Name: "table", Type: wasm.ExternTypeTable}},
		CodeSection: []wasm.Code{{Body: []byte{
			wasm.OpcodeRefNull, byte(wasm.RefTypeFuncref), wasm.OpcodeLocalGet, 0,
			wasm.OpcodeMiscPrefix, wasm.OpcodeMiscTableGrow, 0, wasm.OpcodeEnd,
		}}},
	}
	if imported {
		m.ImportSection = []wasm.Import{{Module: "owner", Name: "table", Type: wasm.ExternTypeTable, DescTable: table}}
	} else {
		m.TableSection = []wasm.Table{table}
	}
	return m
}

type tableBudget struct {
	mu          sync.Mutex
	limit, used uint32
	requests    [][3]uint32
	released    chan uint32
}

func newTableBudget(limit uint32) *tableBudget {
	return &tableBudget{limit: limit, released: make(chan uint32, 8)}
}

func (b *tableBudget) Grow(current, desired, maximum uint32) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.requests = append(b.requests, [3]uint32{current, desired, maximum})
	if desired-current > b.limit-b.used {
		return false
	}
	b.used += desired - current
	return true
}

func (b *tableBudget) Release(elements uint32) {
	b.mu.Lock()
	b.used -= elements
	b.mu.Unlock()
	b.released <- elements
}

func (b *tableBudget) check(t *testing.T, used uint32, requests ...[3]uint32) {
	t.Helper()
	b.mu.Lock()
	defer b.mu.Unlock()
	require.Equal(t, used, b.used)
	require.Equal(t, requests, b.requests)
}

func awaitTableRelease(t *testing.T, b *tableBudget, elements uint32) {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for {
		runtime.GC()
		select {
		case actual := <-b.released:
			require.Equal(t, elements, actual)
			b.mu.Lock()
			require.Equal(t, uint32(0), b.used)
			b.mu.Unlock()
			return
		case <-deadline:
			t.Fatal("table reservation was not released")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

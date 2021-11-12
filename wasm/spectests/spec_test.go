package spectests

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/wasm"
	"github.com/tetratelabs/wazero/wasm/wazeroir"
)

func TestSpecification(t *testing.T) {
	const caseDir = "./cases"
	files, err := os.ReadDir(caseDir)
	require.NoError(t, err)

	for _, f := range files {
		if filepath.Ext(f.Name()) != ".json" {
			continue
		}
		jsonPath := filepath.Join(caseDir, f.Name())
		raw, err := os.ReadFile(jsonPath)
		require.NoError(t, err)

		var base Testbase
		require.NoError(t, json.Unmarshal(raw, &base))

		wastName := filepath.Base(base.SourceFile)
		if wastName != "call.wast" {
			continue
		}
		t.Run(wastName, func(t *testing.T) {
			for _, tc := range []struct {
				name   string
				engine wasm.Engine
			}{
				// {engine: naivevm.NewEngine(), name: "naivevm"},
				{engine: wazeroir.NewEngine(), name: "wazeroir_interpreter"},
			} {
				tc := tc
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()
					store := wasm.NewStore(tc.engine)
					require.NoError(t, AddSpectestModule(store))

					var lastInstanceName string
					for _, c := range base.Commands {
						t.Run(fmt.Sprintf("%s/line:%d", c.CommandType, c.Line), func(t *testing.T) {
							msg := fmt.Sprintf("%s:%d", wastName, c.Line)
							switch c.CommandType {
							case "module":
								buf, err := os.ReadFile(filepath.Join(caseDir, c.Filename))
								require.NoError(t, err, msg)

								mod, err := wasm.DecodeModule(buf)
								require.NoError(t, err, msg)

								lastInstanceName = c.Name
								if lastInstanceName == "" {
									lastInstanceName = c.Filename
								}
								err = store.Instantiate(mod, lastInstanceName)
								require.NoError(t, err)
							case "register":
								name := lastInstanceName
								if c.Name != "" {
									name = c.Name
								}
								store.ModuleInstances[c.As] = store.ModuleInstances[name]
							case "assert_return", "action":
								moduleName := lastInstanceName
								if c.Action.Module != "" {
									moduleName = c.Action.Module
								}
								switch c.Action.ActionType {
								case "invoke":
									args, exps := c.GetAssertReturnArgsExps()
									msg = fmt.Sprintf("%s invoke %s (%s)", msg, c.Action.Field, c.Action.Args)
									if c.Action.Module != "" {
										msg += " in module " + c.Action.Module
									}
									vals, types, err := store.CallFunction(moduleName, c.Action.Field, args...)
									if assert.NoError(t, err, msg) &&
										assert.Equal(t, len(exps), len(vals), msg) &&
										assert.Equal(t, len(exps), len(types), msg) {
										for i, exp := range exps {
											assertValueEq(t, vals[i], exp, types[i], msg)
										}
									}
								case "get":
									_, exps := c.GetAssertReturnArgsExps()
									require.Len(t, exps, 1)
									msg = fmt.Sprintf("%s invoke %s (%s)", msg, c.Action.Field, c.Action.Args)
									if c.Action.Module != "" {
										msg += " in module " + c.Action.Module
									}
									inst, ok := store.ModuleInstances[moduleName]
									require.True(t, ok, msg)
									addr := inst.Exports[c.Action.Field]
									if addr.Kind != wasm.ExportKindGlobal {
										t.Fatal()
									}
									actual := addr.Global
									var expType wasm.ValueType
									switch c.Exps[0].ValType {
									case "i32":
										expType = wasm.ValueTypeI32
									case "i64":
										expType = wasm.ValueTypeI64
									case "f32":
										expType = wasm.ValueTypeF32
									case "f64":
										expType = wasm.ValueTypeF64
									}
									require.NotNil(t, actual, msg)
									assert.Equal(t, expType, actual.Type.ValType, msg)
									assert.Equal(t, exps[0], actual.Val, expType, msg)
								default:
									t.Fatalf("unsupported action type type: %v", c)
								}
							case "assert_malformed":
								if c.ModuleType == "text" {
									// We don't support direct loading of wast yet.
									t.Skip()
								}
								buf, err := os.ReadFile(filepath.Join(caseDir, c.Filename))
								require.NoError(t, err, msg)
								mod, err := wasm.DecodeModule(buf)
								if err == nil {
									err = store.Instantiate(mod, "")
								}
								require.Error(t, err, msg)
							case "assert_trap":
								moduleName := lastInstanceName
								if c.Action.Module != "" {
									moduleName = c.Action.Module
								}
								switch c.Action.ActionType {
								case "invoke":
									args := c.GetAssertReturnArgs()
									msg = fmt.Sprintf("%s invoke %s (%s)", msg, c.Action.Field, c.Action.Args)
									if c.Action.Module != "" {
										msg += " in module " + c.Action.Module
									}
									_, _, err := store.CallFunction(moduleName, c.Action.Field, args...)
									assert.Error(t, err, msg)
								default:
									t.Fatalf("unsupported action type type: %v", c)
								}
							case "assert_invalid":
								if c.ModuleType == "text" {
									// We don't support direct loading of wast yet.
									t.Skip()
								}
								buf, err := os.ReadFile(filepath.Join(caseDir, c.Filename))
								require.NoError(t, err, msg)
								mod, err := wasm.DecodeModule(buf)
								if err == nil {
									err = store.Instantiate(mod, "")
								}
								require.Error(t, err, msg)
							case "assert_exhaustion":
								moduleName := lastInstanceName
								if c.Action.Module != "" {
									moduleName = c.Action.Module
								}
								switch c.Action.ActionType {
								case "invoke":
									args := c.GetAssertReturnArgs()
									msg = fmt.Sprintf("%s invoke %s (%s)", msg, c.Action.Field, c.Action.Args)
									if c.Action.Module != "" {
										msg += " in module " + c.Action.Module
									}
									_, _, err := store.CallFunction(moduleName, c.Action.Field, args...)
									assert.Error(t, err, msg)
									assert.True(t, errors.Is(err, wasm.ErrCallStackOverflow), msg)
								default:
									t.Fatalf("unsupported action type type: %v", c)
								}
							case "assert_unlinkable":
								if c.ModuleType == "text" {
									// We don't support direct loading of wast yet.
									t.Skip()
								}
								buf, err := os.ReadFile(filepath.Join(caseDir, c.Filename))
								require.NoError(t, err, msg)
								mod, err := wasm.DecodeModule(buf)
								if err == nil {
									err = store.Instantiate(mod, "")
								}
								require.Error(t, err, msg)
							case "assert_uninstantiable":
								buf, err := os.ReadFile(filepath.Join(caseDir, c.Filename))
								require.NoError(t, err, msg)

								mod, err := wasm.DecodeModule(buf)
								require.NoError(t, err, msg)

								err = store.Instantiate(mod, "")
								require.Error(t, err, msg)
							default:
								t.Fatalf("unsupported command type: %s", c)
							}
						})
						// if c.Line == 249 {
						// 	return
						// }
					}
				})
			}

		})
	}
}

func assertValueEq(t *testing.T, actual, expected uint64, valType wasm.ValueType, msg string) {
	switch valType {
	case wasm.ValueTypeI32:
		assert.Equal(t, uint32(expected), uint32(actual), msg)
	case wasm.ValueTypeI64:
		assert.Equal(t, expected, actual, msg)
	case wasm.ValueTypeF32:
		expF := math.Float32frombits(uint32(expected))
		actualF := math.Float32frombits(uint32(actual))
		if math.IsNaN(float64(expF)) {
			assert.True(t, math.IsNaN(float64(actualF)), msg)
		} else {
			assert.Equal(t, expF, actualF, msg)
		}
	case wasm.ValueTypeF64:
		expF := math.Float64frombits(expected)
		actualF := math.Float64frombits(actual)
		if math.IsNaN(expF) {
			assert.True(t, math.IsNaN(actualF), msg)
		} else {
			assert.Equal(t, expF, actualF, msg)
		}
	default:
		t.Fail()
	}
}

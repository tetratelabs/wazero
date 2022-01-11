package spectests

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/wasm"
	"github.com/tetratelabs/wazero/wasm/jit"
)

type (
	testbase struct {
		SourceFile string    `json:"source_filename"`
		Commands   []command `json:"commands"`
	}
	command struct {
		CommandType string `json:"type"`
		Line        int    `json:"line"`

		// Set when type == "module" || "register"
		Name string `json:"name,omitempty"`

		// Set when type == "module" || "assert_uninstantiable" || "assert_malformed"
		Filename string `json:"filename,omitempty"`

		// Set when type == "register"
		As string `json:"as,omitempty"`

		// Set when type == "assert_return" || "action"
		Action commandAction      `json:"action,omitempty"`
		Exps   []commandActionVal `json:"expected"`

		// Set when type == "assert_malformed"
		ModuleType string `json:"module_type"`
	}

	commandAction struct {
		ActionType string             `json:"type"`
		Args       []commandActionVal `json:"args"`

		// Set when ActionType == "invoke"
		Field  string `json:"field,omitempty"`
		Module string `json:"module,omitempty"`
	}

	commandActionVal struct {
		ValType string `json:"type"`
		Value   string `json:"value"`
	}
)

func (c commandActionVal) String() string {
	var v string
	switch c.ValType {
	case "i32":
		v = c.Value
	case "f32":
		ret, _ := strconv.ParseUint(c.Value, 10, 32)
		v = fmt.Sprintf("%f", math.Float32frombits(uint32(ret)))
	case "i64":
		v = c.Value
	case "f64":
		ret, _ := strconv.ParseUint(c.Value, 10, 64)
		v = fmt.Sprintf("%f", math.Float64frombits(ret))
	}
	return fmt.Sprintf("{type: %s, value: %v}", c.ValType, v)
}

func (c command) String() string {
	msg := fmt.Sprintf("line: %d, type: %s", c.Line, c.CommandType)
	switch c.CommandType {
	case "register":
		msg += fmt.Sprintf(", name: %s, as: %s", c.Name, c.As)
	case "module":
		if c.Name != "" {
			msg += fmt.Sprintf(", name: %s, filename: %s", c.Name, c.Filename)
		} else {
			msg += fmt.Sprintf(", filename: %s", c.Filename)
		}
	case "assert_return", "action":
		msg += fmt.Sprintf(", action type: %s", c.Action.ActionType)
		if c.Action.Module != "" {
			msg += fmt.Sprintf(", module: %s", c.Action.Module)
		}
		msg += fmt.Sprintf(", field: %s", c.Action.Field)
		msg += fmt.Sprintf(", args: %v, expected: %v", c.Action.Args, c.Exps)
	case "assert_malformed":
		// TODO:
	case "assert_trap":
		// TODO:
	case "assert_invalid":
		// TODO:
	case "assert_exhaustion":
		// TODO:
	case "assert_unlinkable":
		// TODO:
	case "assert_uninstantiable":
		// TODO:
	}
	return "{" + msg + "}"
}

func (c command) getAssertReturnArgs() []uint64 {
	var args []uint64
	for _, arg := range c.Action.Args {
		args = append(args, arg.toUint64())
	}
	return args
}

func (c command) getAssertReturnArgsExps() ([]uint64, []uint64) {
	var args, exps []uint64
	for _, arg := range c.Action.Args {
		args = append(args, arg.toUint64())
	}
	for _, exp := range c.Exps {
		exps = append(exps, exp.toUint64())
	}
	return args, exps
}

func (v commandActionVal) toUint64() uint64 {
	if strings.Contains(v.Value, "nan") {
		if v.ValType == "f32" {
			return uint64(math.Float32bits(float32(math.NaN())))
		}
		return math.Float64bits(math.NaN())
	}

	if strings.Contains(v.ValType, "32") {
		ret, _ := strconv.ParseUint(v.Value, 10, 32)
		return ret
	} else {
		ret, _ := strconv.ParseUint(v.Value, 10, 64)
		return ret
	}
}

func addSpectestModule(t *testing.T, store *wasm.Store) {
	for n, v := range map[string]reflect.Value{
		"print":         reflect.ValueOf(func(*wasm.HostFunctionCallContext) {}),
		"print_i32":     reflect.ValueOf(func(*wasm.HostFunctionCallContext, uint32) {}),
		"print_f32":     reflect.ValueOf(func(*wasm.HostFunctionCallContext, float32) {}),
		"print_i64":     reflect.ValueOf(func(*wasm.HostFunctionCallContext, uint64) {}),
		"print_f64":     reflect.ValueOf(func(*wasm.HostFunctionCallContext, float64) {}),
		"print_i32_f32": reflect.ValueOf(func(*wasm.HostFunctionCallContext, uint32, float32) {}),
		"print_f64_f64": reflect.ValueOf(func(*wasm.HostFunctionCallContext, float64, float64) {}),
	} {
		require.NoError(t, store.AddHostFunction("spectest", n, v), "AddHostFunction(%s)", n)
	}

	for _, g := range []struct {
		name      string
		valueType wasm.ValueType
		value     uint64
	}{
		{name: "global_i32", valueType: wasm.ValueTypeI32, value: uint64(int32(666))},
		{name: "global_i64", valueType: wasm.ValueTypeI64, value: uint64(int64(666))},
		{name: "global_f32", valueType: wasm.ValueTypeF32, value: uint64(uint32(0x44268000))},
		{name: "global_f64", valueType: wasm.ValueTypeF64, value: uint64(0x4084d00000000000)},
	} {
		require.NoError(t, store.AddGlobal("spectest", g.name, g.value, g.valueType, false), "AddGlobal(%s)", g.name)
	}

	tableLimitMax := uint32(20)
	require.NoError(t, store.AddTableInstance("spectest", "table", 10, &tableLimitMax))

	memoryLimitMax := uint32(2)
	require.NoError(t, store.AddMemoryInstance("spectest", "memory", 1, &memoryLimitMax))
}

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

		var base testbase
		require.NoError(t, json.Unmarshal(raw, &base))

		wastName := filepath.Base(base.SourceFile)
		if !strings.Contains(wastName, "address.wast") {
			t.Skip()
		}
		t.Run(wastName, func(t *testing.T) {
			engines := []struct {
				name   string
				engine wasm.Engine
			}{
				// {engine: wazeroir.NewEngine(), name: "interpreter"},
			}

			// JIT is only implemented for amd64 now.
			if runtime.GOARCH == "amd64" {
				engines = append(engines, struct {
					name   string
					engine wasm.Engine
				}{
					name: "jit", engine: jit.NewEngine(),
				})
			}
			for _, tc := range engines {
				tc := tc
				t.Run(tc.name, func(t *testing.T) {
					// t.Parallel()
					store := wasm.NewStore(tc.engine)
					addSpectestModule(t, store)

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
									args, exps := c.getAssertReturnArgsExps()
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
									_, exps := c.getAssertReturnArgsExps()
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
									args := c.getAssertReturnArgs()
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
									args := c.getAssertReturnArgs()
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

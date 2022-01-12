package spectests

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/wasm"
	"github.com/tetratelabs/wazero/wasm/jit"
	"github.com/tetratelabs/wazero/wasm/wazeroir"
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

func TestJIT(t *testing.T) {
	runTest(t, jit.NewEngine, map[string]bool{
		// "address.wast":       true,
		// "align.wast":         false, // Needs br_table support
		// "binary.wast":        false, // Needs br_table support
		// "binary-leb128.wast": true,
		// "block.wast":         false,
		// "break-drop.wast":    false, // Needs br_table support
		// "br_if.wast":         false, // Needs br_table support
		// "br.wast":            false, // Needs br_table support
		// "call_indirect.wast": false, // Needs call_indirect support
		// "call.wast":          false, // Needs call_indirect support
		// "comments.wast":      true,
		// "const.wast":       true,
		//		"conversions.wast":            true,
		// "custom.wast":                 true,
		// "data.wast":                   true,
		// "elem.wast":                   false, // Needs call_indirect support
		// "endianness.wast":             true, TODO: needs fix.
		// "exports.wast":                true,
		// "f32_bitwise.wast":            true,
		"f32_cmp.wast":                true,
		"f32.wast":                    false,
		"f64_bitwise.wast":            false,
		"f64_cmp.wast":                false,
		"f64.wast":                    false,
		"fac.wast":                    false,
		"float_exprs.wast":            false,
		"float_literals.wast":         false,
		"float_memory.wast":           false,
		"float_misc.wast":             false,
		"forward.wast":                false,
		"func_ptrs.wast":              false,
		"func.wast":                   false,
		"globals.wast":                false,
		"i32.wast":                    false,
		"i64.wast":                    false,
		"if.wast":                     false,
		"imports.wast":                false,
		"inline-module.wast":          false,
		"int_exprs.wast":              false,
		"int_literals.wast":           false,
		"labels.wast":                 false,
		"left-to-right.wast":          false,
		"linking.wast":                false,
		"load.wast":                   false,
		"local_get.wast":              false,
		"local_set.wast":              false,
		"local_tee.wast":              false,
		"loop.wast":                   false,
		"memory_grow.wast":            false,
		"memory_redundancy.wast":      false,
		"memory_size.wast":            false,
		"memory_trap.wast":            false,
		"memory.wast":                 false,
		"names.wast":                  false,
		"nop.wast":                    false,
		"return.wast":                 false,
		"select.wast":                 false,
		"skip-stack-guard-page.wast":  false,
		"stack.wast":                  false,
		"start.wast":                  false,
		"store.wast":                  false,
		"switch.wast":                 false,
		"token.wast":                  false,
		"traps.wast":                  false,
		"type.wast":                   false,
		"unreachable.wast":            false,
		"unreached-invalid.wast":      false,
		"unwind.wast":                 false,
		"utf8-custom-section-id.wast": false,
		"utf8-import-field.wast":      false,
		"utf8-import-module.wast":     false,
	})
}

func TestInterpreter(t *testing.T) {
	runTest(t, wazeroir.NewEngine, nil)
}

func runTest(t *testing.T, newEngine func() wasm.Engine, wastTargets map[string]bool) {
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
		if wastTargets != nil && !wastTargets[wastName] {
			continue
		}
		t.Run(wastName, func(t *testing.T) {
			store := wasm.NewStore(newEngine())
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

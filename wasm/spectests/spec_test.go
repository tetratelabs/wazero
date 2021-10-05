package spec

import (
	"bytes"
	"encoding/json"
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

	"github.com/mathetake/gasm/wasm"
)

type (
	testbase struct {
		SourceFile string    `json:"source_filename"`
		Commands   []command `json:"commands"`
	}
	command struct {
		CommandType string `json:"type"`
		Line        int    `json:"line"`

		// type == "module" || "register"
		Name string `json:"name,omitempty"`

		// type == "module" || "assert_uninstantiable"
		Filename string `json:"filename,omitempty"`

		// type == "register"
		As string `json:"as,omitempty"`

		// type == "assert_return" || "action"
		Action commandAction      `json:"action,omitempty"`
		Exps   []commandActionVal `json:"expected"`
	}

	commandAction struct {
		ActionType string             `json:"type"`
		Args       []commandActionVal `json:"args"`

		// ActionType == "invoke"
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

func (c command) getAssertReturnArgsExps(t *testing.T) ([]uint64, []uint64) {
	var args, exps []uint64
	for _, arg := range c.Action.Args {
		args = append(args, arg.ToUint64(t))
	}
	for _, exp := range c.Exps {
		exps = append(exps, exp.ToUint64(t))
	}
	return args, exps
}

func (v commandActionVal) ToUint64(t *testing.T) uint64 {
	if strings.Contains(v.Value, "nan") {
		if v.ValType == "f32" {
			return uint64(math.Float32bits(float32(math.NaN())))
		}
		return math.Float64bits(math.NaN())
	}

	if strings.Contains(v.ValType, "32") {
		ret, err := strconv.ParseUint(v.Value, 10, 32)
		require.NoError(t, err)
		return ret
	} else {
		ret, err := strconv.ParseUint(v.Value, 10, 64)
		require.NoError(t, err)
		return ret
	}
}

func addSpectestModule(vm *wasm.VirtualMachine) {
	// Add functions
	vm.AddHostFunction("spectest", "print", reflect.ValueOf(func(*wasm.VirtualMachine) {}))
	vm.AddHostFunction("spectest", "print_i32", reflect.ValueOf(func(*wasm.VirtualMachine, uint32) {}))
	vm.AddHostFunction("spectest", "print_f32", reflect.ValueOf(func(*wasm.VirtualMachine, float32) {}))
	vm.AddHostFunction("spectest", "print_i64", reflect.ValueOf(func(*wasm.VirtualMachine, uint64) {}))
	vm.AddHostFunction("spectest", "print_f64", reflect.ValueOf(func(*wasm.VirtualMachine, float64) {}))
	vm.AddHostFunction("spectest", "print_i32_f32", reflect.ValueOf(func(*wasm.VirtualMachine, uint32, float32) {}))
	vm.AddHostFunction("spectest", "print_f64_f64", reflect.ValueOf(func(*wasm.VirtualMachine, float64, float64) {}))
	// Register globals.
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
		vm.AddGlobal("spectest", g.name, g.value, g.valueType, false)
	}
	// Register table export.
	tableLimitMax := uint32(20)
	vm.AddTableInstance("spectest", "table", 0, &tableLimitMax)
	// Register table export.
	memoryLimitMax := uint32(2)
	vm.AddMemoryInstance("spectest", "memory", 1, &memoryLimitMax)
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
		t.Run(wastName, func(t *testing.T) {
			vm, err := wasm.NewVM()
			require.NoError(t, err)
			addSpectestModule(vm)

			var lastInstanceName string
			for _, c := range base.Commands {
				t.Run(fmt.Sprintf("%d", c.Line), func(t *testing.T) {
					msg := fmt.Sprintf("%s:%d", wastName, c.Line)
					switch c.CommandType {
					case "module":
						buf, err := os.ReadFile(filepath.Join(caseDir, c.Filename))
						require.NoError(t, err, msg)

						mod, err := wasm.DecodeModule(bytes.NewBuffer(buf))
						require.NoError(t, err, msg)

						lastInstanceName = c.Name
						if lastInstanceName == "" {
							lastInstanceName = c.Filename
						}
						err = vm.Instantiate(mod, lastInstanceName)
						require.NoError(t, err)
					case "register":
						name := lastInstanceName
						if c.Name != "" {
							name = c.Name
						}
						vm.Store.ModuleInstances[c.As] = vm.Store.ModuleInstances[name]
					case "assert_return", "action":
						moduleName := lastInstanceName
						if c.Action.Module != "" {
							moduleName = c.Action.Module
						}
						require.NotNil(t, vm)
						switch c.Action.ActionType {
						case "invoke":
							args, exps := c.getAssertReturnArgsExps(t)
							msg = fmt.Sprintf("%s invoke %s (%s)", msg, c.Action.Field, c.Action.Args)
							if c.Action.Module != "" {
								msg += " in module " + c.Action.Module
							}
							vals, types, err := vm.ExecExportedFunction(moduleName, c.Action.Field, args...)
							if assert.NoError(t, err, msg) &&
								assert.Equal(t, len(exps), len(vals), msg) &&
								assert.Equal(t, len(exps), len(types), msg) {
								for i, exp := range exps {
									assertValueEq(t, vals[i], exp, types[i], msg)
								}
							}
						case "get":
							// TODO:
						default:
							t.Fatalf("unsupported action type type: %v", c)
						}
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
						buf, err := os.ReadFile(filepath.Join(caseDir, c.Filename))
						require.NoError(t, err, msg)

						mod, err := wasm.DecodeModule(bytes.NewBuffer(buf))
						require.NoError(t, err, msg)

						err = vm.Instantiate(mod, "")
						require.Error(t, err)
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

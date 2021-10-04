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

	"github.com/mathetake/gasm/hostfunc"
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

		// type == "module"
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

func spectestModule() *wasm.Module {
	builder := hostfunc.NewModuleBuilder()
	builder.MustSetFunction("spectest", "print", func(*wasm.VirtualMachine) reflect.Value {
		return reflect.ValueOf(func() {})
	})
	builder.MustSetFunction("spectest", "print_i32", func(*wasm.VirtualMachine) reflect.Value {
		return reflect.ValueOf(func(v uint32) {})
	})
	builder.MustSetFunction("spectest", "print_f32", func(*wasm.VirtualMachine) reflect.Value {
		return reflect.ValueOf(func(v float32) {})
	})
	builder.MustSetFunction("spectest", "print_i64", func(*wasm.VirtualMachine) reflect.Value {
		return reflect.ValueOf(func(v uint64) {})
	})
	builder.MustSetFunction("spectest", "print_f64", func(*wasm.VirtualMachine) reflect.Value {
		return reflect.ValueOf(func(v float64) {})
	})
	builder.MustSetFunction("spectest", "print_i32_f32", func(*wasm.VirtualMachine) reflect.Value {
		return reflect.ValueOf(func(v uint32, w float32) {})
	})
	builder.MustSetFunction("spectest", "print_f64_f64", func(*wasm.VirtualMachine) reflect.Value {
		return reflect.ValueOf(func(v, w float64) {})
	})

	m := builder.Done()["spectest"]

	// Register globals export.
	for i, g := range []struct {
		name      string
		valueType wasm.ValueType
		value     interface{}
	}{
		{name: "global_i32", valueType: wasm.ValueTypeI32, value: int32(666)},
		{name: "global_i64", valueType: wasm.ValueTypeI64, value: int64(666)},
		{name: "global_f32", valueType: wasm.ValueTypeF32, value: math.Float32frombits(0x44268000)},
		{name: "global_f64", valueType: wasm.ValueTypeF64, value: math.Float64frombits(0x4084d00000000000)},
	} {
		m.SecExports[g.name] = &wasm.ExportSegment{
			Desc: &wasm.ExportDesc{Kind: wasm.ExportKindGlobal, Index: uint32(i)},
		}
		m.IndexSpace.Globals = append(m.IndexSpace.Globals, &wasm.Global{
			Type: &wasm.GlobalType{Value: g.valueType},
			Val:  g.value,
		})
	}
	// Register table export.
	m.SecExports["table"] = &wasm.ExportSegment{
		Desc: &wasm.ExportDesc{Kind: wasm.ExportKindTable, Index: uint32(0)},
	}
	tableLimitMax := uint32(20)
	m.IndexSpace.Table = append(m.IndexSpace.Table, make([]*uint32, 10))
	m.SecTables = append(m.SecTables, &wasm.TableType{ElemType: 0x70, Limit: &wasm.LimitsType{
		Min: 10,
		Max: &tableLimitMax,
	}})
	// Register table export.
	m.SecExports["memory"] = &wasm.ExportSegment{
		Desc: &wasm.ExportDesc{Kind: wasm.ExportKindMem, Index: uint32(0)},
	}
	memoryLimitMax := uint32(2)
	m.IndexSpace.Memory = append(m.IndexSpace.Memory, make([]byte, 1))
	m.SecMemory = append(m.SecMemory, &wasm.MemoryType{
		Min: 1,
		Max: &memoryLimitMax,
	})
	return m
}

func TestSpecification(t *testing.T) {
	// (assert_return (invoke "max" (f64.const -0x0p+0) (f64.const -0x0.0000000000001p-1022)) (f64.const -0x0p+0))
	v := float64(-0x0p+0)
	w := float64(-0x0.0000000000001p-1022)
	require.True(t, math.Max(v, w) == v, math.Max(v, w))
	const caseDir = "./cases"
	files, err := os.ReadDir(caseDir)
	require.NoError(t, err)

	spectestModule := spectestModule()
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
			// t.Parallel()

			modules := map[string]*wasm.Module{}
			linkableModules := map[string]*wasm.Module{"spectest": spectestModule}
			vms := map[string]*wasm.VirtualMachine{}
			var latestVM *wasm.VirtualMachine

			for _, c := range base.Commands {
				// fmt.Println(c)
				msg := fmt.Sprintf("%s:%d", wastName, c.Line)
				switch c.CommandType {
				case "module":
					buf, err := os.ReadFile(filepath.Join(caseDir, c.Filename))
					require.NoError(t, err, msg)

					mod, err := wasm.DecodeModule(bytes.NewBuffer(buf))
					require.NoError(t, err, msg)
					modules[c.Name] = mod

					latestVM, err = wasm.NewVM(mod, linkableModules)
					require.NoError(t, err, msg)
					require.NotNil(t, latestVM, msg, msg)
					vms[c.Name] = latestVM
				case "register":
					m, ok := modules[c.Name]
					require.True(t, ok, msg)
					linkableModules[c.As] = m
				case "assert_return", "action":
					vm := latestVM
					if c.Action.Module != "" {
						var ok bool
						vm, ok = vms[c.Action.Module]
						require.True(t, ok, msg)
					}
					require.NotNil(t, vm)
					switch c.Action.ActionType {
					case "invoke":
						args, exps := c.getAssertReturnArgsExps(t)
						msg = fmt.Sprintf("%s invoke %s (%s)", msg, c.Action.Field, c.Action.Args)
						vals, types, err := vm.ExecExportedFunction(c.Action.Field, args...)
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
					// TODO:
				default:
					t.Fatalf("unsupported command type: %s", c)
				}
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

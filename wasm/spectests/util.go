package spectests

import (
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/wasm"
)

type (
	Testbase struct {
		SourceFile string    `json:"source_filename"`
		Commands   []Command `json:"commands"`
	}
	Command struct {
		CommandType string `json:"type"`
		Line        int    `json:"line"`

		// Set when type == "module" || "register"
		Name string `json:"name,omitempty"`

		// Set when type == "module" || "assert_uninstantiable" || "assert_malformed"
		Filename string `json:"filename,omitempty"`

		// Set when type == "register"
		As string `json:"as,omitempty"`

		// Set when type == "assert_return" || "action"
		Action CommandAction      `json:"action,omitempty"`
		Exps   []CommandActionVal `json:"expected"`

		// Set when type == "assert_malformed"
		ModuleType string `json:"module_type"`
	}

	CommandAction struct {
		ActionType string             `json:"type"`
		Args       []CommandActionVal `json:"args"`

		// Set when ActionType == "invoke"
		Field  string `json:"field,omitempty"`
		Module string `json:"module,omitempty"`
	}

	CommandActionVal struct {
		ValType string `json:"type"`
		Value   string `json:"value"`
	}
)

func (c CommandActionVal) String() string {
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

func (c Command) String() string {
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

func (c Command) GetAssertReturnArgs() []uint64 {
	var args []uint64
	for _, arg := range c.Action.Args {
		args = append(args, arg.ToUint64())
	}
	return args
}

func (c Command) GetAssertReturnArgsExps() ([]uint64, []uint64) {
	var args, exps []uint64
	for _, arg := range c.Action.Args {
		args = append(args, arg.ToUint64())
	}
	for _, exp := range c.Exps {
		exps = append(exps, exp.ToUint64())
	}
	return args, exps
}

func (v CommandActionVal) ToUint64() uint64 {
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

func AddSpectestModule(t *testing.T, store *wasm.Store) {
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

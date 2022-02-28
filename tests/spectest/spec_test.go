package spectests

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"math"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	wasm "github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wasm/binary"
	"github.com/tetratelabs/wazero/internal/wasm/interpreter"
	"github.com/tetratelabs/wazero/internal/wasm/jit"
)

//go:embed testdata/*.wasm
//go:embed testdata/*.json
var testcases embed.FS

// TODO: complete porting this to wazero API
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

		// Set when type == "assert_trap"
		Text string `json:"text"`
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
		msg += fmt.Sprintf(", args: %v, error text:  %s", c.Action.Args, c.Text)
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

// expectedError returns the expected runtime error when the command type equals assert_trap
// which expectes engines to emit the errors corresponding command.Text field.
func (c command) expectedError() (err error) {
	if c.CommandType != "assert_trap" {
		panic("unreachable")
	}
	switch c.Text {
	case "out of bounds memory access":
		err = wasm.ErrRuntimeOutOfBoundsMemoryAccess
	case "indirect call type mismatch", "indirect call":
		err = wasm.ErrRuntimeIndirectCallTypeMismatch
	case "undefined element", "undefined":
		err = wasm.ErrRuntimeInvalidTableAccess
	case "integer overflow":
		err = wasm.ErrRuntimeIntegerOverflow
	case "invalid conversion to integer":
		err = wasm.ErrRuntimeInvalidConversionToInteger
	case "integer divide by zero":
		err = wasm.ErrRuntimeIntegerDivideByZero
	case "unreachable":
		err = wasm.ErrRuntimeUnreachable
	default:
		if strings.HasPrefix(c.Text, "uninitialized") {
			err = wasm.ErrRuntimeInvalidTableAccess
		}
	}
	return
}

func addSpectestModule(t *testing.T, store *wasm.Store) {
	// Add the host module
	spectest := &wasm.ModuleInstance{Name: "spectest", Exports: map[string]*wasm.ExportInstance{}}
	store.ModuleInstances[spectest.Name] = spectest

	var printV = func() {}
	var printI32 = func(uint32) {}
	var printF32 = func(float32) {}
	var printI64 = func(uint64) {}
	var printF64 = func(float64) {}
	var printI32F32 = func(uint32, float32) {}
	var printF64F64 = func(float64, float64) {}

	for n, v := range map[string]interface{}{
		"print":         printV,
		"print_i32":     printI32,
		"print_f32":     printF32,
		"print_i64":     printI64,
		"print_f64":     printF64,
		"print_i32_f32": printI32F32,
		"print_f64_f64": printF64F64,
	} {
		fn, err := wasm.NewGoFunc(n, v)
		require.NoError(t, err)
		_, err = store.AddHostFunction(spectest, fn)
		require.NoError(t, err, "AddHostFunction(%s)", n)
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
		require.NoError(t, store.AddGlobal(spectest, g.name, g.value, g.valueType, false), "AddGlobal(%s)", g.name)
	}

	tableLimitMax := uint32(20)
	require.NoError(t, store.AddTableInstance(spectest, "table", 10, &tableLimitMax))

	memoryLimitMax := uint32(2)
	require.NoError(t, store.AddMemoryInstance(spectest, "memory", 1, &memoryLimitMax))
}

func TestJIT(t *testing.T) {
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		t.Skip()
	}
	runTest(t, jit.NewEngine)
}

func TestInterpreter(t *testing.T) {
	runTest(t, interpreter.NewEngine)
}

func runTest(t *testing.T, newEngine func() wasm.Engine) {
	files, err := testcases.ReadDir("testdata")
	require.NoError(t, err)

	jsonfiles := make([]string, 0, len(files))
	for _, f := range files {
		filename := f.Name()
		if strings.HasSuffix(filename, ".json") {
			jsonfiles = append(jsonfiles, testdataPath(filename))
		}
	}

	// If the go:embed path resolution was wrong, this fails.
	// https://github.com/tetratelabs/wazero/issues/247
	require.Greater(t, len(jsonfiles), 1)

	for _, f := range jsonfiles {
		raw, err := testcases.ReadFile(f)
		require.NoError(t, err)

		var base testbase
		require.NoError(t, json.Unmarshal(raw, &base))

		wastName := basename(base.SourceFile)

		t.Run(wastName, func(t *testing.T) {
			store := wasm.NewStore(context.Background(), newEngine())
			addSpectestModule(t, store)

			var lastInstanceName string
			for _, c := range base.Commands {
				t.Run(fmt.Sprintf("%s/line:%d", c.CommandType, c.Line), func(t *testing.T) {
					msg := fmt.Sprintf("%s:%d %s", wastName, c.Line, c.CommandType)
					switch c.CommandType {
					case "module":
						buf, err := testcases.ReadFile(testdataPath(c.Filename))
						require.NoError(t, err, msg)

						mod, err := binary.DecodeModule(buf)
						require.NoError(t, err, msg)

						lastInstanceName = c.Name
						if lastInstanceName == "" {
							lastInstanceName = c.Filename
						}
						_, err = store.Instantiate(mod, lastInstanceName)
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
							vals, types, err := callFunction(store, moduleName, c.Action.Field, args...)
							require.NoError(t, err, msg)
							require.Equal(t, len(exps), len(vals), msg)
							require.Equal(t, len(exps), len(types), msg)
							for i, exp := range exps {
								requireValueEq(t, vals[i], exp, types[i], msg)
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
							addr, err := inst.GetExport(c.Action.Field, wasm.ExternTypeGlobal)
							require.NoError(t, err)
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
							require.Equal(t, expType, actual.Type.ValType, msg)
							require.Equal(t, exps[0], actual.Val, expType, msg)
						default:
							t.Fatalf("unsupported action type type: %v", c)
						}
					case "assert_malformed":
						if c.ModuleType == "text" {
							// We don't support direct loading of wast yet.
							t.Skip()
						}
						buf, err := testcases.ReadFile(testdataPath(c.Filename))
						require.NoError(t, err, msg)
						mod, err := binary.DecodeModule(buf)
						if err == nil {
							_, err = store.Instantiate(mod, "")
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
							_, _, err := callFunction(store, moduleName, c.Action.Field, args...)
							require.ErrorIs(t, err, c.expectedError(), msg)
						default:
							t.Fatalf("unsupported action type type: %v", c)
						}
					case "assert_invalid":
						if c.ModuleType == "text" {
							// We don't support direct loading of wast yet.
							t.Skip()
						}
						buf, err := testcases.ReadFile(testdataPath(c.Filename))
						require.NoError(t, err, msg)
						mod, err := binary.DecodeModule(buf)
						if err == nil {
							_, err = store.Instantiate(mod, "")
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
							_, _, err := callFunction(store, moduleName, c.Action.Field, args...)
							require.ErrorIs(t, err, wasm.ErrRuntimeCallStackOverflow, msg)
						default:
							t.Fatalf("unsupported action type type: %v", c)
						}
					case "assert_unlinkable":
						if c.ModuleType == "text" {
							// We don't support direct loading of wast yet.
							t.Skip()
						}
						buf, err := testcases.ReadFile(testdataPath(c.Filename))
						require.NoError(t, err, msg)
						mod, err := binary.DecodeModule(buf)
						if err == nil {
							_, err = store.Instantiate(mod, "")
						}
						require.Error(t, err, msg)
					case "assert_uninstantiable":
						buf, err := testcases.ReadFile(testdataPath(c.Filename))
						require.NoError(t, err, msg)

						mod, err := binary.DecodeModule(buf)
						require.NoError(t, err, msg)

						_, err = store.Instantiate(mod, "")
						require.Error(t, err, msg)
					default:
						t.Fatalf("unsupported command type: %s", c)
					}
				})
			}
		})
	}
}

// basename avoids filepath.Base to ensure a forward slash is used even in Windows.
// See https://pkg.go.dev/embed#hdr-Directives
func basename(path string) string {
	lastSlash := strings.LastIndexByte(path, '/')
	return path[lastSlash+1:]
}

// testdataPath avoids filepath.Join to ensure a forward slash is used even in Windows.
// See https://pkg.go.dev/embed#hdr-Directives
func testdataPath(filename string) string {
	return fmt.Sprintf("testdata/%s", filename)
}

func requireValueEq(t *testing.T, actual, expected uint64, valType wasm.ValueType, msg string) {
	switch valType {
	case wasm.ValueTypeI32:
		require.Equal(t, uint32(expected), uint32(actual), msg)
	case wasm.ValueTypeI64:
		require.Equal(t, expected, actual, msg)
	case wasm.ValueTypeF32:
		expF := math.Float32frombits(uint32(expected))
		actualF := math.Float32frombits(uint32(actual))
		if math.IsNaN(float64(expF)) { // NaN cannot be compared with themselves, so we have to use IsNaN
			require.True(t, math.IsNaN(float64(actualF)), msg)
		} else {
			require.Equal(t, expF, actualF, msg)
		}
	case wasm.ValueTypeF64:
		expF := math.Float64frombits(expected)
		actualF := math.Float64frombits(actual)
		if math.IsNaN(expF) { // NaN cannot be compared with themselves, so we have to use IsNaN
			require.True(t, math.IsNaN(actualF), msg)
		} else {
			require.Equal(t, expF, actualF, msg)
		}
	default:
		t.Fail()
	}
}

// callFunction is inlined here as the spectest needs to validate the signature was correct
// TODO: This is likely already covered with unit tests!
func callFunction(s *wasm.Store, moduleName, funcName string, params ...uint64) ([]uint64, []wasm.ValueType, error) {
	fn := s.ModuleExports(moduleName).Function(funcName)
	results, err := fn.Call(context.Background(), params...)
	return results, fn.ResultTypes(), err
}

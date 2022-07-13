package spectest

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/leb128"
	"github.com/tetratelabs/wazero/internal/moremath"
	"github.com/tetratelabs/wazero/internal/sys"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/u64"
	"github.com/tetratelabs/wazero/internal/wasm"
	binaryformat "github.com/tetratelabs/wazero/internal/wasm/binary"
	"github.com/tetratelabs/wazero/internal/wasmruntime"
	"github.com/tetratelabs/wazero/internal/watzero"
)

// testCtx is an arbitrary, non-default context. Non-nil also prevents linter errors.
var testCtx = context.WithValue(context.Background(), struct{}{}, "arbitrary")

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
		// LaneType is not empty if ValueType == "v128"
		LaneType laneType    `json:"lane_type"`
		Value    interface{} `json:"value"`
	}
)

// laneType is a type of each lane of vector value.
//
// See https://github.com/WebAssembly/wabt/blob/main/docs/wast2json.md#const
type laneType = string

const (
	laneTypeI8  laneType = "i8"
	laneTypeI16 laneType = "i16"
	laneTypeI32 laneType = "i32"
	laneTypeI64 laneType = "i64"
	laneTypeF32 laneType = "f32"
	laneTypeF64 laneType = "f64"
)

func (c commandActionVal) String() string {
	var v string
	valTypeStr := c.ValType
	switch c.ValType {
	case "i32":
		v = c.Value.(string)
	case "f32":
		str := c.Value.(string)
		if strings.Contains(str, "nan") {
			v = str
		} else {
			ret, _ := strconv.ParseUint(str, 10, 32)
			v = fmt.Sprintf("%f", math.Float32frombits(uint32(ret)))
		}
	case "i64":
		v = c.Value.(string)
	case "f64":
		str := c.Value.(string)
		if strings.Contains(str, "nan") {
			v = str
		} else {
			ret, _ := strconv.ParseUint(str, 10, 64)
			v = fmt.Sprintf("%f", math.Float64frombits(ret))
		}
	case "externref":
		if c.Value == "null" {
			v = "null"
		} else {
			original, _ := strconv.ParseUint(c.Value.(string), 10, 64)
			// In wazero, externref is opaque pointer, so "0" is considered as null.
			// So in order to treat "externref 0" in spectest non nullref, we increment the value.
			v = fmt.Sprintf("%d", original+1)
		}
	case "funcref":
		// All the in and out funcref params are null in spectest (cannot represent non-null as it depends on runtime impl).
		v = "null"
	case "v128":
		simdValues, ok := c.Value.([]interface{})
		if !ok {
			panic("BUG")
		}
		var strs []string
		for _, v := range simdValues {
			strs = append(strs, v.(string))
		}
		v = strings.Join(strs, ",")
		valTypeStr = fmt.Sprintf("v128[lane=%s]", c.LaneType)
	}
	return fmt.Sprintf("{type: %s, value: %v}", valTypeStr, v)
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
		args = append(args, arg.toUint64s()...)
	}
	return args
}

func (c command) getAssertReturnArgsExps() (args []uint64, exps []uint64) {
	for _, arg := range c.Action.Args {
		args = append(args, arg.toUint64s()...)
	}
	for _, exp := range c.Exps {
		exps = append(exps, exp.toUint64s()...)
	}
	return
}

func (c commandActionVal) toUint64s() (ret []uint64) {
	if c.ValType == "v128" {
		strValues, ok := c.Value.([]interface{})
		if !ok {
			panic("BUG")
		}
		var width, valNum int
		switch c.LaneType {
		case "i8":
			width, valNum = 8, 16
		case "i16":
			width, valNum = 16, 8
		case "i32":
			width, valNum = 32, 4
		case "i64":
			width, valNum = 64, 2
		case "f32":
			width, valNum = 32, 4
		case "f64":
			width, valNum = 64, 2
		default:
			panic("BUG")
		}
		lo, hi := buildLaneUint64(strValues, width, valNum)
		return []uint64{lo, hi}
	} else {
		return []uint64{c.toUint64()}
	}
}

func buildLaneUint64(raw []interface{}, width, valNum int) (lo, hi uint64) {
	for i := 0; i < valNum; i++ {
		str := raw[i].(string)

		var v uint64
		var err error
		if strings.Contains(str, "nan") {
			v = getNaNBits(str, width == 32)
		} else {
			v, err = strconv.ParseUint(str, 10, width)
			if err != nil {
				panic(err)
			}
		}

		if half := valNum / 2; i < half {
			lo |= v << (i * width)
		} else {
			hi |= v << ((i - half) * width)
		}
	}
	return
}

func getNaNBits(strValue string, is32bit bool) (ret uint64) {
	// Note: nan:canonical, nan:arithmetic only appears on the expected values.
	if is32bit {
		switch strValue {
		case "nan:canonical":
			ret = uint64(moremath.F32CanonicalNaNBits)
		case "nan:arithmetic":
			ret = uint64(moremath.F32ArithmeticNaNBits)
		default:
			panic("BUG")
		}
	} else {
		switch strValue {
		case "nan:canonical":
			ret = moremath.F64CanonicalNaNBits
		case "nan:arithmetic":
			ret = moremath.F64ArithmeticNaNBits
		default:
			panic("BUG")
		}
	}
	return
}

func (c commandActionVal) toUint64() (ret uint64) {
	strValue := c.Value.(string)
	if strings.Contains(strValue, "nan") {
		ret = getNaNBits(strValue, c.ValType == "f32")
	} else if c.ValType == "externref" {
		if c.Value == "null" {
			ret = 0
		} else {
			original, _ := strconv.ParseUint(strValue, 10, 64)
			// In wazero, externref is opaque pointer, so "0" is considered as null.
			// So in order to treat "externref 0" in spectest non nullref, we increment the value.
			ret = original + 1
		}
	} else if strings.Contains(c.ValType, "32") {
		ret, _ = strconv.ParseUint(strValue, 10, 32)
	} else {
		ret, _ = strconv.ParseUint(strValue, 10, 64)
	}
	return
}

// expectedError returns the expected runtime error when the command type equals assert_trap
// which expects engines to emit the errors corresponding command.Text field.
func (c command) expectedError() (err error) {
	if c.CommandType != "assert_trap" {
		panic("unreachable")
	}
	switch c.Text {
	case "out of bounds memory access":
		err = wasmruntime.ErrRuntimeOutOfBoundsMemoryAccess
	case "indirect call type mismatch", "indirect call":
		err = wasmruntime.ErrRuntimeIndirectCallTypeMismatch
	case "undefined element", "undefined", "out of bounds table access":
		err = wasmruntime.ErrRuntimeInvalidTableAccess
	case "integer overflow":
		err = wasmruntime.ErrRuntimeIntegerOverflow
	case "invalid conversion to integer":
		err = wasmruntime.ErrRuntimeInvalidConversionToInteger
	case "integer divide by zero":
		err = wasmruntime.ErrRuntimeIntegerDivideByZero
	case "unreachable":
		err = wasmruntime.ErrRuntimeUnreachable
	default:
		if strings.HasPrefix(c.Text, "uninitialized") {
			err = wasmruntime.ErrRuntimeInvalidTableAccess
		}
	}
	return
}

// addSpectestModule adds a module that drops inputs and returns globals as 666 per the default test harness.
//
// See https://github.com/WebAssembly/spec/blob/wg-1.0/test/core/imports.wast
// See https://github.com/WebAssembly/spec/blob/wg-1.0/interpreter/script/js.ml#L13-L25
func addSpectestModule(t *testing.T, s *wasm.Store, ns *wasm.Namespace) {
	w, err := watzero.Wat2Wasm(`(module $spectest
(; TODO
  (global (export "global_i32") i32)
  (global (export "global_i64") i64)
  (global (export "global_f32") f32)
  (global (export "global_f64") f64)

  (table (export "table") 10 20 funcref)
;)

;; TODO: revisit inlining after #215

  (memory 1 2)
    (export "memory" (memory 0))

;; Note: the following aren't host functions that print to console as it would clutter it. These only drop the inputs.
  (func)
     (export "print" (func 0))

  (func (param i32) local.get 0 drop)
     (export "print_i32" (func 1))

  (func (param i64) local.get 0 drop)
     (export "print_i64" (func 2))

  (func (param f32) local.get 0 drop)
     (export "print_f32" (func 3))

  (func (param f64) local.get 0 drop)
     (export "print_f64" (func 4))

  (func (param i32 f32) local.get 0 drop local.get 1 drop)
     (export "print_i32_f32" (func 5))

  (func (param f64 f64) local.get 0 drop local.get 1 drop)
     (export "print_f64_f64" (func 6))
)`)
	require.NoError(t, err)

	mod, err := binaryformat.DecodeModule(w, wasm.Features20220419, wasm.MemorySizer)
	require.NoError(t, err)

	// (global (export "global_i32") i32 (i32.const 666))
	mod.GlobalSection = append(mod.GlobalSection, &wasm.Global{
		Type: &wasm.GlobalType{ValType: wasm.ValueTypeI32},
		Init: &wasm.ConstantExpression{Opcode: wasm.OpcodeI32Const, Data: leb128.EncodeInt32(666)},
	})
	mod.ExportSection = append(mod.ExportSection, &wasm.Export{Name: "global_i32", Index: 0, Type: wasm.ExternTypeGlobal})

	// (global (export "global_i64") i64 (i32.const 666))
	mod.GlobalSection = append(mod.GlobalSection, &wasm.Global{
		Type: &wasm.GlobalType{ValType: wasm.ValueTypeI64},
		Init: &wasm.ConstantExpression{Opcode: wasm.OpcodeI64Const, Data: leb128.EncodeInt32(666)},
	})
	mod.ExportSection = append(mod.ExportSection, &wasm.Export{Name: "global_i64", Index: 1, Type: wasm.ExternTypeGlobal})

	// (global (export "global_f32") f32 (f32.const 666))
	mod.GlobalSection = append(mod.GlobalSection, &wasm.Global{
		Type: &wasm.GlobalType{ValType: wasm.ValueTypeF32},
		Init: &wasm.ConstantExpression{Opcode: wasm.OpcodeF32Const, Data: u64.LeBytes(api.EncodeF32(666))},
	})
	mod.ExportSection = append(mod.ExportSection, &wasm.Export{Name: "global_f32", Index: 2, Type: wasm.ExternTypeGlobal})

	// (global (export "global_f64") f64 (f64.const 666))
	mod.GlobalSection = append(mod.GlobalSection, &wasm.Global{
		Type: &wasm.GlobalType{ValType: wasm.ValueTypeF64},
		Init: &wasm.ConstantExpression{Opcode: wasm.OpcodeF64Const, Data: u64.LeBytes(api.EncodeF64(666))},
	})
	mod.ExportSection = append(mod.ExportSection, &wasm.Export{Name: "global_f64", Index: 3, Type: wasm.ExternTypeGlobal})

	//  (table (export "table") 10 20 funcref)
	tableLimitMax := uint32(20)
	mod.TableSection = []*wasm.Table{{Min: 10, Max: &tableLimitMax, Type: wasm.RefTypeFuncref}}
	mod.ExportSection = append(mod.ExportSection, &wasm.Export{Name: "table", Index: 0, Type: wasm.ExternTypeTable})

	maybeSetMemoryCap(mod)
	mod.BuildFunctionDefinitions()
	err = s.Engine.CompileModule(testCtx, mod)
	require.NoError(t, err)

	_, err = s.Instantiate(testCtx, ns, mod, mod.NameSection.ModuleName, sys.DefaultContext(nil), nil)
	require.NoError(t, err)
}

// maybeSetMemoryCap assigns wasm.Memory Cap to Min, which is what wazero.CompileModule would do.
func maybeSetMemoryCap(mod *wasm.Module) {
	if mem := mod.MemorySection; mem != nil {
		mem.Cap = mem.Min
	}
}

// Run runs all the test inside the testDataFS file system where all the cases are described
// via JSON files created from wast2json.
func Run(t *testing.T, testDataFS embed.FS, newEngine func(wasm.Features) wasm.Engine, enabledFeatures wasm.Features) {
	files, err := testDataFS.ReadDir("testdata")
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
	require.True(t, len(jsonfiles) > 1, "len(jsonfiles)=%d (not greater than one)", len(jsonfiles))

	for _, f := range jsonfiles {
		raw, err := testDataFS.ReadFile(f)
		require.NoError(t, err)

		var base testbase
		require.NoError(t, json.Unmarshal(raw, &base))

		wastName := basename(base.SourceFile)

		t.Run(wastName, func(t *testing.T) {
			s, ns := wasm.NewStore(enabledFeatures, newEngine(enabledFeatures))
			addSpectestModule(t, s, ns)

			var lastInstantiatedModuleName string
			for _, c := range base.Commands {
				t.Run(fmt.Sprintf("%s/line:%d", c.CommandType, c.Line), func(t *testing.T) {
					msg := fmt.Sprintf("%s:%d %s", wastName, c.Line, c.CommandType)
					switch c.CommandType {
					case "module":
						buf, err := testDataFS.ReadFile(testdataPath(c.Filename))
						require.NoError(t, err, msg)
						mod, err := binaryformat.DecodeModule(buf, enabledFeatures, wasm.MemorySizer)
						require.NoError(t, err, msg)
						require.NoError(t, mod.Validate(enabledFeatures))
						mod.AssignModuleID(buf)

						moduleName := c.Name
						if moduleName == "" {
							// Use the file name as the name.
							moduleName = c.Filename
						}

						maybeSetMemoryCap(mod)
						mod.BuildFunctionDefinitions()
						err = s.Engine.CompileModule(testCtx, mod)
						require.NoError(t, err, msg)

						_, err = s.Instantiate(testCtx, ns, mod, moduleName, nil, nil)
						lastInstantiatedModuleName = moduleName
						require.NoError(t, err)
					case "register":
						src := c.Name
						if src == "" {
							src = lastInstantiatedModuleName
						}
						ns.AliasModule(src, c.As)
						lastInstantiatedModuleName = c.As
					case "assert_return", "action":
						moduleName := lastInstantiatedModuleName
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
							vals, types, err := callFunction(ns, moduleName, c.Action.Field, args...)
							require.NoError(t, err, msg)
							require.Equal(t, len(exps), len(vals), msg)
							laneTypes := map[int]string{}
							for i, expV := range c.Exps {
								if expV.ValType == "v128" {
									laneTypes[i] = expV.LaneType
								}
							}
							matched, valuesMsg := valuesEq(vals, exps, types, laneTypes)
							require.True(t, matched, msg+"\n"+valuesMsg)
						case "get":
							_, exps := c.getAssertReturnArgsExps()
							require.Equal(t, 1, len(exps))
							msg = fmt.Sprintf("%s invoke %s (%s)", msg, c.Action.Field, c.Action.Args)
							if c.Action.Module != "" {
								msg += " in module " + c.Action.Module
							}
							module := ns.Module(moduleName)
							require.NotNil(t, module)
							global := module.ExportedGlobal(c.Action.Field)
							require.NotNil(t, global)
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
							require.Equal(t, expType, global.Type(), msg)
							require.Equal(t, exps[0], global.Get(testCtx), msg)
						default:
							t.Fatalf("unsupported action type type: %v", c)
						}
					case "assert_malformed":
						if c.ModuleType != "text" {
							// We don't support direct loading of wast yet.
							buf, err := testDataFS.ReadFile(testdataPath(c.Filename))
							require.NoError(t, err, msg)
							requireInstantiationError(t, s, ns, buf, msg)
						}
					case "assert_trap":
						moduleName := lastInstantiatedModuleName
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
							_, _, err := callFunction(ns, moduleName, c.Action.Field, args...)
							require.ErrorIs(t, err, c.expectedError(), msg)
						default:
							t.Fatalf("unsupported action type type: %v", c)
						}
					case "assert_invalid":
						if c.ModuleType == "text" {
							// We don't support direct loading of wast yet.
							t.Skip()
						}
						buf, err := testDataFS.ReadFile(testdataPath(c.Filename))
						require.NoError(t, err, msg)
						requireInstantiationError(t, s, ns, buf, msg)
					case "assert_exhaustion":
						moduleName := lastInstantiatedModuleName
						switch c.Action.ActionType {
						case "invoke":
							args := c.getAssertReturnArgs()
							msg = fmt.Sprintf("%s invoke %s (%s)", msg, c.Action.Field, c.Action.Args)
							if c.Action.Module != "" {
								msg += " in module " + c.Action.Module
							}
							_, _, err := callFunction(ns, moduleName, c.Action.Field, args...)
							require.ErrorIs(t, err, wasmruntime.ErrRuntimeCallStackOverflow, msg)
						default:
							t.Fatalf("unsupported action type type: %v", c)
						}
					case "assert_unlinkable":
						if c.ModuleType == "text" {
							// We don't support direct loading of wast yet.
							t.Skip()
						}
						buf, err := testDataFS.ReadFile(testdataPath(c.Filename))
						require.NoError(t, err, msg)
						requireInstantiationError(t, s, ns, buf, msg)
					case "assert_uninstantiable":
						buf, err := testDataFS.ReadFile(testdataPath(c.Filename))
						require.NoError(t, err, msg)
						if c.Text == "out of bounds table access" {
							// This is not actually an instantiation error, but assert_trap in the original wast, but wast2json translates it to assert_uninstantiable.
							// Anyway, this spectest case expects the error due to active element offset ouf of bounds
							// "after" instantiation while retaining function instances used for elements.
							// https://github.com/WebAssembly/spec/blob/d39195773112a22b245ffbe864bab6d1182ccb06/test/core/linking.wast#L264-L274
							//
							// In practice, such a module instance can be used for invoking functions without any issue. In addition, we have to
							// retain functions after the expected "instantiation" failure, so in wazero we choose to not raise error in that case.
							mod, err := binaryformat.DecodeModule(buf, s.EnabledFeatures, wasm.MemorySizer)
							require.NoError(t, err, msg)

							err = mod.Validate(s.EnabledFeatures)
							require.NoError(t, err, msg)

							mod.AssignModuleID(buf)

							maybeSetMemoryCap(mod)
							mod.BuildFunctionDefinitions()
							err = s.Engine.CompileModule(testCtx, mod)
							require.NoError(t, err, msg)

							_, err = s.Instantiate(testCtx, ns, mod, t.Name(), nil, nil)
							require.NoError(t, err, msg)
						} else {
							requireInstantiationError(t, s, ns, buf, msg)
						}

					default:
						t.Fatalf("unsupported command type: %s", c)
					}
				})
			}
		})
	}
}

func requireInstantiationError(t *testing.T, s *wasm.Store, ns *wasm.Namespace, buf []byte, msg string) {
	mod, err := binaryformat.DecodeModule(buf, s.EnabledFeatures, wasm.MemorySizer)
	if err != nil {
		return
	}

	err = mod.Validate(s.EnabledFeatures)
	if err != nil {
		return
	}

	mod.AssignModuleID(buf)

	maybeSetMemoryCap(mod)
	mod.BuildFunctionDefinitions()
	err = s.Engine.CompileModule(testCtx, mod)
	if err != nil {
		return
	}

	_, err = s.Instantiate(testCtx, ns, mod, t.Name(), nil, nil)
	require.Error(t, err, msg)
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

// valuesEq returns true if all the actual result matches exps which are all expressed as uint64.
// 	* actual,exps: comparison target values which are all represented as uint64, meaning that if valTypes = [V128,I32], then
//		we have actual/exp = [(lower-64bit of the first V128), (higher-64bit of the first V128), I32].
// 	* valTypes holds the wasm.ValueType(s) of the original values in Wasm.
// 	* laneTypes maps the index of valueTypes to laneType if valueTypes[i] == wasm.ValueTypeV128.
//
// Also, if matched == false this returns non-empty valuesMsg which can be used to augment the test failure message.
func valuesEq(actual, exps []uint64, valTypes []wasm.ValueType, laneTypes map[int]laneType) (matched bool, valuesMsg string) {
	matched = true

	var msgExpValuesStrs, msgActualValuesStrs []string
	var uint64RepPos int // the index to actual and exps slice.
	for i, tp := range valTypes {
		switch tp {
		case wasm.ValueTypeI32:
			msgExpValuesStrs = append(msgExpValuesStrs, fmt.Sprintf("%d", uint32(exps[uint64RepPos])))
			msgActualValuesStrs = append(msgActualValuesStrs, fmt.Sprintf("%d", uint32(actual[uint64RepPos])))
			matched = matched && uint32(exps[uint64RepPos]) == uint32(actual[uint64RepPos])
			uint64RepPos++
		case wasm.ValueTypeI64, wasm.ValueTypeExternref, wasm.ValueTypeFuncref:
			msgExpValuesStrs = append(msgExpValuesStrs, fmt.Sprintf("%d", exps[uint64RepPos]))
			msgActualValuesStrs = append(msgActualValuesStrs, fmt.Sprintf("%d", actual[uint64RepPos]))
			matched = matched && exps[uint64RepPos] == actual[uint64RepPos]
			uint64RepPos++
		case wasm.ValueTypeF32:
			a := math.Float32frombits(uint32(actual[uint64RepPos]))
			e := math.Float32frombits(uint32(exps[uint64RepPos]))
			msgExpValuesStrs = append(msgExpValuesStrs, fmt.Sprintf("%f", e))
			msgActualValuesStrs = append(msgActualValuesStrs, fmt.Sprintf("%f", a))
			matched = matched && f32Equal(e, a)
			uint64RepPos++
		case wasm.ValueTypeF64:
			e := math.Float64frombits(exps[uint64RepPos])
			a := math.Float64frombits(actual[uint64RepPos])
			msgExpValuesStrs = append(msgExpValuesStrs, fmt.Sprintf("%f", e))
			msgActualValuesStrs = append(msgActualValuesStrs, fmt.Sprintf("%f", a))
			matched = matched && f64Equal(e, a)
			uint64RepPos++
		case wasm.ValueTypeV128:
			actualLo, actualHi := actual[uint64RepPos], actual[uint64RepPos+1]
			expLo, expHi := exps[uint64RepPos], exps[uint64RepPos+1]
			switch laneTypes[i] {
			case laneTypeI8:
				msgExpValuesStrs = append(msgExpValuesStrs,
					fmt.Sprintf("i8x16(%#x, %#x, %#x, %#x, %#x, %#x, %#x, %#x, %#x, %#x, %#x, %#x, %#x, %#x, %#x, %#x)",
						byte(expLo), byte(expLo>>8), byte(expLo>>16), byte(expLo>>24),
						byte(expLo>>32), byte(expLo>>40), byte(expLo>>48), byte(expLo>>56),
						byte(expHi), byte(expHi>>8), byte(expHi>>16), byte(expHi>>24),
						byte(expHi>>32), byte(expHi>>40), byte(expHi>>48), byte(expHi>>56),
					),
				)
				msgActualValuesStrs = append(msgActualValuesStrs,
					fmt.Sprintf("i8x16(%#x, %#x, %#x, %#x, %#x, %#x, %#x, %#x, %#x, %#x, %#x, %#x, %#x, %#x, %#x, %#x)",
						byte(actualLo), byte(actualLo>>8), byte(actualLo>>16), byte(actualLo>>24),
						byte(actualLo>>32), byte(actualLo>>40), byte(actualLo>>48), byte(actualLo>>56),
						byte(actualHi), byte(actualHi>>8), byte(actualHi>>16), byte(actualHi>>24),
						byte(actualHi>>32), byte(actualHi>>40), byte(actualHi>>48), byte(actualHi>>56),
					),
				)
				matched = matched && (expLo == actualLo) && (expHi == actualHi)
			case laneTypeI16:
				msgExpValuesStrs = append(msgExpValuesStrs,
					fmt.Sprintf("i16x8(%#x, %#x, %#x, %#x, %#x, %#x, %#x, %#x)",
						uint16(expLo), uint16(expLo>>16), uint16(expLo>>32), uint16(expLo>>48),
						uint16(expHi), uint16(expHi>>16), uint16(expHi>>32), uint16(expHi>>48),
					),
				)
				msgActualValuesStrs = append(msgActualValuesStrs,
					fmt.Sprintf("i16x8(%#x, %#x, %#x, %#x, %#x, %#x, %#x, %#x)",
						uint16(actualLo), uint16(actualLo>>16), uint16(actualLo>>32), uint16(actualLo>>48),
						uint16(actualHi), uint16(actualHi>>16), uint16(actualHi>>32), uint16(actualHi>>48),
					),
				)
				matched = matched && (expLo == actualLo) && (expHi == actualHi)
			case laneTypeI32:
				msgExpValuesStrs = append(msgExpValuesStrs,
					fmt.Sprintf("i32x4(%#x, %#x, %#x, %#x)", uint32(expLo), uint32(expLo>>32), uint32(expHi), uint32(expHi>>32)),
				)
				msgActualValuesStrs = append(msgActualValuesStrs,
					fmt.Sprintf("i32x4(%#x, %#x, %#x, %#x)", uint32(actualLo), uint32(actualLo>>32), uint32(actualHi), uint32(actualHi>>32)),
				)
				matched = matched && (expLo == actualLo) && (expHi == actualHi)
			case laneTypeI64:
				msgExpValuesStrs = append(msgExpValuesStrs,
					fmt.Sprintf("i64x2(%#x, %#x)", expLo, expHi),
				)
				msgActualValuesStrs = append(msgActualValuesStrs,
					fmt.Sprintf("i64x2(%#x, %#x)", actualLo, actualHi),
				)
				matched = matched && (expLo == actualLo) && (expHi == actualHi)
			case laneTypeF32:
				msgExpValuesStrs = append(msgExpValuesStrs,
					fmt.Sprintf("f32x4(%f, %f, %f, %f)",
						math.Float32frombits(uint32(expLo)), math.Float32frombits(uint32(expLo>>32)),
						math.Float32frombits(uint32(expHi)), math.Float32frombits(uint32(expHi>>32)),
					),
				)
				msgActualValuesStrs = append(msgActualValuesStrs,
					fmt.Sprintf("f32x4(%f, %f, %f, %f)",
						math.Float32frombits(uint32(actualLo)), math.Float32frombits(uint32(actualLo>>32)),
						math.Float32frombits(uint32(actualHi)), math.Float32frombits(uint32(actualHi>>32)),
					),
				)
				matched = matched &&
					f32Equal(math.Float32frombits(uint32(expLo)), math.Float32frombits(uint32(actualLo))) &&
					f32Equal(math.Float32frombits(uint32(expLo>>32)), math.Float32frombits(uint32(actualLo>>32))) &&
					f32Equal(math.Float32frombits(uint32(expHi)), math.Float32frombits(uint32(actualHi))) &&
					f32Equal(math.Float32frombits(uint32(expHi>>32)), math.Float32frombits(uint32(actualHi>>32)))
			case laneTypeF64:
				msgExpValuesStrs = append(msgExpValuesStrs,
					fmt.Sprintf("f64x2(%f, %f)", math.Float64frombits(expLo), math.Float64frombits(expHi)),
				)
				msgActualValuesStrs = append(msgActualValuesStrs,
					fmt.Sprintf("f64x2(%f, %f)", math.Float64frombits(actualLo), math.Float64frombits(actualHi)),
				)
				matched = matched &&
					f64Equal(math.Float64frombits(expLo), math.Float64frombits(actualLo)) &&
					f64Equal(math.Float64frombits(expHi), math.Float64frombits(actualHi))
			default:
				panic("BUG")
			}
			uint64RepPos += 2
		default:
			panic("BUG")
		}
	}

	if !matched {
		valuesMsg = fmt.Sprintf("\thave [%s]\n\twant [%s]",
			strings.Join(msgActualValuesStrs, ", "),
			strings.Join(msgExpValuesStrs, ", "))
	}
	return
}

func f32Equal(expected, actual float32) (matched bool) {
	if expBit := math.Float32bits(expected); expBit == moremath.F32CanonicalNaNBits {
		matched = math.Float32bits(actual)&moremath.F32CanonicalNaNBitsMask == moremath.F32CanonicalNaNBits
	} else if expBit == moremath.F32ArithmeticNaNBits {
		b := math.Float32bits(actual)
		matched = b&moremath.F32ExponentMask == moremath.F32ExponentMask && // Indicates that exponent part equals of NaN.
			b&moremath.F32ArithmeticNaNPayloadMSB == moremath.F32ArithmeticNaNPayloadMSB
	} else if math.IsNaN(float64(expected)) { // NaN cannot be compared with themselves, so we have to use IsNaN
		matched = math.IsNaN(float64(actual))
	} else {
		matched = expected == actual
	}
	return
}

func f64Equal(expected, actual float64) (matched bool) {
	if expBit := math.Float64bits(expected); expBit == moremath.F64CanonicalNaNBits {
		matched = math.Float64bits(actual)&moremath.F64CanonicalNaNBitsMask == moremath.F64CanonicalNaNBits
	} else if expBit == moremath.F64ArithmeticNaNBits {
		b := math.Float64bits(actual)
		matched = b&moremath.F64ExponentMask == moremath.F64ExponentMask && // Indicates that exponent part equals of NaN.
			b&moremath.F64ArithmeticNaNPayloadMSB == moremath.F64ArithmeticNaNPayloadMSB
	} else if math.IsNaN(expected) { // NaN cannot be compared with themselves, so we have to use IsNaN
		matched = math.IsNaN(actual)
	} else {
		matched = expected == actual
	}
	return
}

// callFunction is inlined here as the spectest needs to validate the signature was correct
// TODO: This is likely already covered with unit tests!
func callFunction(ns *wasm.Namespace, moduleName, funcName string, params ...uint64) ([]uint64, []wasm.ValueType, error) {
	fn := ns.Module(moduleName).ExportedFunction(funcName)
	results, err := fn.Call(testCtx, params...)
	return results, fn.Definition().ResultTypes(), err
}

// requireStripCustomSections strips all the custom sections from the given binary.
func requireStripCustomSections(t *testing.T, binary []byte) []byte {
	r := bytes.NewReader(binary)
	out := bytes.NewBuffer(nil)
	_, err := io.CopyN(out, r, 8)
	require.NoError(t, err)

	for {
		sectionID, err := r.ReadByte()
		if err == io.EOF {
			break
		} else if err != nil {
			require.NoError(t, err)
		}

		sectionSize, _, err := leb128.DecodeUint32(r)
		require.NoError(t, err)

		switch sectionID {
		case wasm.SectionIDCustom:
			_, err = io.CopyN(io.Discard, r, int64(sectionSize))
			require.NoError(t, err)
		default:
			out.WriteByte(sectionID)
			out.Write(leb128.EncodeUint32(sectionSize))
			_, err := io.CopyN(out, r, int64(sectionSize))
			require.NoError(t, err)
		}
	}
	return out.Bytes()
}

// TestBinaryEncoder ensures that binary.EncodeModule produces exactly the same binaries
// for wasm.Module via binary.DecodeModule modulo custom sections for all the valid binaries in spectests.
func TestBinaryEncoder(t *testing.T, testDataFS embed.FS, enabledFeatures wasm.Features) {
	files, err := testDataFS.ReadDir("testdata")
	require.NoError(t, err)

	for _, f := range files {
		filename := f.Name()
		if strings.HasSuffix(filename, ".json") {
			raw, err := testDataFS.ReadFile(fmt.Sprintf("testdata/%s", filename))
			require.NoError(t, err)

			var base testbase
			require.NoError(t, json.Unmarshal(raw, &base))

			for _, c := range base.Commands {
				if c.CommandType == "module" {
					t.Run(c.Filename, func(t *testing.T) {
						buf, err := testDataFS.ReadFile(fmt.Sprintf("testdata/%s", c.Filename))
						require.NoError(t, err)

						buf = requireStripCustomSections(t, buf)

						mod, err := binaryformat.DecodeModule(buf, enabledFeatures, wasm.MemorySizer)
						require.NoError(t, err)

						encodedBuf := binaryformat.EncodeModule(mod)
						require.Equal(t, buf, encodedBuf)
					})
				}
			}
		}
	}
}

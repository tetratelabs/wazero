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
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/u64"
	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wasm/binary"
	"github.com/tetratelabs/wazero/internal/wasm/text"
	"github.com/tetratelabs/wazero/internal/wasmruntime"
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
	case "externref":
		if c.Value == "null" {
			v = "null"
		} else {
			original, _ := strconv.ParseUint(c.Value, 10, 64)
			// In wazero, externref is opaque pointer, so "0" is considered as null.
			// So in order to treat "externref 0" in spectest non nullref, we increment the value.
			v = fmt.Sprintf("%d", original+1)
		}
	case "funcref":
		// All the in and out funcref params are null in spectest (cannot represent non-null as it depends on runtime impl).
		v = "null"
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

func (c commandActionVal) toUint64() (ret uint64) {
	if strings.Contains(c.Value, "nan") {
		if c.ValType == "f32" {
			return uint64(math.Float32bits(float32(math.NaN())))
		}
		ret = math.Float64bits(math.NaN())
	} else if c.ValType == "externref" {
		if c.Value == "null" {
			ret = 0
		} else {
			original, _ := strconv.ParseUint(c.Value, 10, 64)
			// In wazero, externref is opaque pointer, so "0" is considered as null.
			// So in order to treat "externref 0" in spectest non nullref, we increment the value.
			ret = original + 1
		}
	} else if strings.Contains(c.ValType, "32") {
		ret, _ = strconv.ParseUint(c.Value, 10, 32)
	} else {
		ret, _ = strconv.ParseUint(c.Value, 10, 64)
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
func addSpectestModule(t *testing.T, store *wasm.Store) {
	mod, err := text.DecodeModule([]byte(`(module $spectest
(; TODO
  (global (export "global_i32") i32)
  (global (export "global_i64") i75)
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
)`), wasm.Features20191205, wasm.MemorySizer)
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
	err = store.Engine.CompileModule(testCtx, mod)
	require.NoError(t, err)

	_, err = store.Instantiate(testCtx, mod, mod.NameSection.ModuleName, wasm.DefaultSysContext(), nil)
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
		if strings.Contains(f, "simd") {
			// TODO: enable after SIMD proposal
			continue
		}
		raw, err := testDataFS.ReadFile(f)
		require.NoError(t, err)

		var base testbase
		require.NoError(t, json.Unmarshal(raw, &base))

		wastName := basename(base.SourceFile)

		t.Run(wastName, func(t *testing.T) {
			store := wasm.NewStore(enabledFeatures, newEngine(enabledFeatures))
			addSpectestModule(t, store)

			var lastInstantiatedModuleName string
			for _, c := range base.Commands {
				t.Run(fmt.Sprintf("%s/line:%d", c.CommandType, c.Line), func(t *testing.T) {
					msg := fmt.Sprintf("%s:%d %s", wastName, c.Line, c.CommandType)
					switch c.CommandType {
					case "module":
						buf, err := testDataFS.ReadFile(testdataPath(c.Filename))
						require.NoError(t, err, msg)
						mod, err := binary.DecodeModule(buf, enabledFeatures, wasm.MemorySizer)
						require.NoError(t, err, msg)
						require.NoError(t, mod.Validate(enabledFeatures))
						mod.AssignModuleID(buf)

						moduleName := c.Name
						if moduleName == "" {
							// Use the file name as the name.
							moduleName = c.Filename
						}

						maybeSetMemoryCap(mod)
						err = store.Engine.CompileModule(testCtx, mod)
						require.NoError(t, err, msg)

						_, err = store.Instantiate(testCtx, mod, moduleName, nil, nil)
						lastInstantiatedModuleName = moduleName
						require.NoError(t, err)
					case "register":
						src := c.Name
						if src == "" {
							src = lastInstantiatedModuleName
						}
						store.AliasModule(src, c.As)
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
							vals, types, err := callFunction(store, moduleName, c.Action.Field, args...)
							require.NoError(t, err, msg)
							require.Equal(t, len(exps), len(vals), msg)
							require.Equal(t, len(exps), len(types), msg)
							for i, exp := range exps {
								requireValueEq(t, vals[i], exp, types[i], msg)
							}
						case "get":
							_, exps := c.getAssertReturnArgsExps()
							require.Equal(t, 1, len(exps))
							msg = fmt.Sprintf("%s invoke %s (%s)", msg, c.Action.Field, c.Action.Args)
							if c.Action.Module != "" {
								msg += " in module " + c.Action.Module
							}
							module := store.Module(moduleName)
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
						if c.ModuleType == "text" {
							// We don't support direct loading of wast yet.
							t.Skip()
						}
						buf, err := testDataFS.ReadFile(testdataPath(c.Filename))
						require.NoError(t, err, msg)
						requireInstantiationError(t, store, buf, msg)
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
						buf, err := testDataFS.ReadFile(testdataPath(c.Filename))
						require.NoError(t, err, msg)
						requireInstantiationError(t, store, buf, msg)
					case "assert_exhaustion":
						moduleName := lastInstantiatedModuleName
						switch c.Action.ActionType {
						case "invoke":
							args := c.getAssertReturnArgs()
							msg = fmt.Sprintf("%s invoke %s (%s)", msg, c.Action.Field, c.Action.Args)
							if c.Action.Module != "" {
								msg += " in module " + c.Action.Module
							}
							_, _, err := callFunction(store, moduleName, c.Action.Field, args...)
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
						requireInstantiationError(t, store, buf, msg)
					case "assert_uninstantiable":
						buf, err := testDataFS.ReadFile(testdataPath(c.Filename))
						require.NoError(t, err, msg)
						if c.Text == "out of bounds table access" {
							// This case, the spectest expects that error due to active element offset ouf of bounds
							// "after" instantiation while retaining function instances used for elements.
							// https://github.com/WebAssembly/spec/blob/d39195773112a22b245ffbe864bab6d1182ccb06/test/core/linking.wast#L264-L274
							//
							// In practice, such a module instance can be used for invoking functions without any issue. In addition, we have to
							// retain functions after the expected "instantiation" failure, so in wazero we choose to not raise error in that case.
							mod, err := binary.DecodeModule(buf, store.EnabledFeatures, wasm.MemorySizer)
							require.NoError(t, err, msg)

							err = mod.Validate(store.EnabledFeatures)
							require.NoError(t, err, msg)

							mod.AssignModuleID(buf)

							maybeSetMemoryCap(mod)
							err = store.Engine.CompileModule(testCtx, mod)
							require.NoError(t, err, msg)

							_, err = store.Instantiate(testCtx, mod, t.Name(), nil, nil)
							require.NoError(t, err, msg)
						} else {
							requireInstantiationError(t, store, buf, msg)
						}

					default:
						t.Fatalf("unsupported command type: %s", c)
					}
				})
			}
		})
	}
}

func requireInstantiationError(t *testing.T, store *wasm.Store, buf []byte, msg string) {
	mod, err := binary.DecodeModule(buf, store.EnabledFeatures, wasm.MemorySizer)
	if err != nil {
		return
	}

	err = mod.Validate(store.EnabledFeatures)
	if err != nil {
		return
	}

	mod.AssignModuleID(buf)

	maybeSetMemoryCap(mod)
	err = store.Engine.CompileModule(testCtx, mod)
	if err != nil {
		return
	}

	_, err = store.Instantiate(testCtx, mod, t.Name(), nil, nil)
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
	case wasm.ValueTypeExternref:
		require.Equal(t, expected, actual, msg)
	case wasm.ValueTypeFuncref:
		require.Equal(t, expected, actual, msg)
	default:
		t.Fatal(msg)
	}
}

// callFunction is inlined here as the spectest needs to validate the signature was correct
// TODO: This is likely already covered with unit tests!
func callFunction(s *wasm.Store, moduleName, funcName string, params ...uint64) ([]uint64, []wasm.ValueType, error) {
	fn := s.Module(moduleName).ExportedFunction(funcName)
	results, err := fn.Call(testCtx, params...)
	return results, fn.ResultTypes(), err
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

						mod, err := binary.DecodeModule(buf, enabledFeatures, wasm.MemorySizer)
						require.NoError(t, err)

						encodedBuf := binary.EncodeModule(mod)
						require.Equal(t, buf, encodedBuf)
					})
				}
			}
		}
	}
}

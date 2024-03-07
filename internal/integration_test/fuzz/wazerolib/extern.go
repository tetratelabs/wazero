package main

import "C"
import (
	"context"
	"math"
	"reflect"
	"strings"
	"unsafe"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/internal/leb128"
	"github.com/tetratelabs/wazero/internal/testing/binaryencoding"
	"github.com/tetratelabs/wazero/internal/testing/nodiff"
	"github.com/tetratelabs/wazero/internal/wasm"
)

func main() {}

// require_no_diff ensures that the behavior is the same between the compiler and the interpreter for any given binary.
// And if there's diff, this also saves the problematic binary and wat into testdata directory.
//
//export require_no_diff
func require_no_diff(binaryPtr uintptr, binarySize int, checkMemory bool, checkLogging bool) {
	// TODO: use unsafe.Slice after flooring Go 1.20.
	var wasmBin []byte
	wasmHdr := (*reflect.SliceHeader)(unsafe.Pointer(&wasmBin))
	wasmHdr.Data = binaryPtr
	wasmHdr.Len = binarySize
	wasmHdr.Cap = binarySize

	failed := true
	defer func() {
		if failed {
			// If the test fails, we save the binary and wat into testdata directory.
			saveFailedBinary(wasmBin, "TestReRunFailedRequireNoDiffCase")
		}
	}()

	nodiff.RequireNoDiff(wasmBin, checkMemory, checkLogging, func(err error) {
		if err != nil {
			panic(err)
		}
	})

	failed = false
}

// validate accepts maybe-invalid Wasm module bytes and ensures that our validation phase works correctly
// as well as the compiler doesn't panic during compilation!
//
//export validate
func validate(binaryPtr uintptr, binarySize int) {
	// TODO: use unsafe.Slice after flooring Go 1.20.
	var wasmBin []byte
	wasmHdr := (*reflect.SliceHeader)(unsafe.Pointer(&wasmBin))
	wasmHdr.Data = binaryPtr
	wasmHdr.Len = binarySize
	wasmHdr.Cap = binarySize

	failed := true
	defer func() {
		if failed {
			// If the test fails, we save the binary and wat into testdata directory.
			saveFailedBinary(wasmBin, "TestReRunFailedValidateCase")
		}
	}()

	tryCompile(wasmBin)
	failed = false
}

//export test_signal_stack
func test_signal_stack() {
	// (module
	//  (func (export "long_loop")
	//    (loop
	//      global.get 0
	//      i32.eqz
	//      if  ;; label = @1
	//        unreachable
	//      end
	//      global.get 0
	//      i32.const 1
	//      i32.sub
	//      global.set 0
	//      br 0
	//    )
	//  )
	//  (global (;0;) (mut i32) i32.const 2147483648)
	// )
	bin := binaryencoding.EncodeModule(&wasm.Module{
		TypeSection:     []wasm.FunctionType{{}},
		FunctionSection: []wasm.Index{0},
		GlobalSection: []wasm.Global{{
			Type: wasm.GlobalType{ValType: wasm.ValueTypeI32, Mutable: true},
			Init: wasm.ConstantExpression{
				Opcode: wasm.OpcodeI32Const,
				Data:   leb128.EncodeInt32(math.MaxInt32),
			},
		}},
		ExportSection: []wasm.Export{{Type: wasm.ExternTypeFunc, Name: "long_loop", Index: 0}},
		CodeSection: []wasm.Code{
			{
				Body: []byte{
					wasm.OpcodeLoop, 0,
					wasm.OpcodeGlobalGet, 0,
					wasm.OpcodeI32Eqz,
					wasm.OpcodeIf, 0,
					wasm.OpcodeUnreachable,
					wasm.OpcodeEnd,
					wasm.OpcodeGlobalGet, 0,
					wasm.OpcodeI32Const, 1,
					wasm.OpcodeI32Sub,
					wasm.OpcodeGlobalSet, 0,
					wasm.OpcodeBr, 0,
					wasm.OpcodeEnd,
					wasm.OpcodeEnd,
				},
			},
		},
	})
	ctx := context.Background()
	config := wazero.NewRuntimeConfigCompiler()
	r := wazero.NewRuntimeWithConfig(ctx, config)
	module, err := r.Instantiate(ctx, bin)
	if err != nil {
		panic(err)
	}
	defer func() {
		if err = module.Close(ctx); err != nil {
			panic(err)
		}
	}()

	_, err = module.ExportedFunction("long_loop").Call(ctx)
	if !strings.Contains(err.Error(), "unreachable") {
		panic("long_loop should be unreachable")
	}
}

package main

import "C"
import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/internal/engine/wazevo"
	"github.com/tetratelabs/wazero/internal/leb128"
	"github.com/tetratelabs/wazero/internal/testing/binaryencoding"
	"github.com/tetratelabs/wazero/internal/wasm"
	"math"
	"os"
	"path"
	"runtime"
	"strings"
)

func main() {}

const failedCasesDir = "wazerolib/testdata"

// saveFailedBinary writes binary and wat into failedCasesDir so that it is easy to reproduce the error.
func saveFailedBinary(bin []byte, wat string, reproduceTestName string) {
	checksum := sha256.Sum256(bin)
	checkSumStr := hex.EncodeToString(checksum[:])

	dir, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	testDataDir := path.Join(dir, failedCasesDir)
	binaryPath := path.Join(testDataDir, fmt.Sprintf("%s.wasm", checkSumStr))
	f, err := os.Create(binaryPath)
	if err != nil {
		panic(err)
	}

	defer f.Close()

	_, err = f.Write(bin)
	if err != nil {
		panic(err)
	}

	var watPath string
	if len(wat) != 0 {
		watPath = path.Join(testDataDir, fmt.Sprintf("%s.wat", checkSumStr))
		watF, err := os.Create(watPath)
		if err != nil {
			panic(err)
		}

		defer watF.Close()

		_, err = watF.Write([]byte(wat))
		if err != nil {
			panic(err)
		}
		fmt.Printf(`
Failed WebAssembly Text:
%s

Failed Wasm binary has been written to %s
Failed Wasm Text has been written to %s
To reproduce the failure, execute: WASM_BINARY_PATH=%s go test -run=%s ./wazerolib/...


`, wat, binaryPath, watPath, binaryPath, reproduceTestName)
	} else {
		fmt.Printf(`
Failed WebAssembly Binary in hex: %s
Failed Wasm binary has been written to %s
To reproduce the failure, execute: WASM_BINARY_PATH=%s go test -run=%s ./wazerolib/...
`, hex.EncodeToString(bin), binaryPath, binaryPath, reproduceTestName)
	}
}

// This returns a wazevo.RuntimeConfigure whose compiler is either wazevo or the default.
func newCompilerConfig() wazero.RuntimeConfig {
	c := wazero.NewRuntimeConfigCompiler()
	if os.Getenv("WAZERO_FUZZ_WAZEVO") != "" {
		wazevo.ConfigureWazevo(c)
	}
	return c
}

//export test_signal_stack
func test_signal_stack() {
	if runtime.GOARCH != "arm64" { // TODO: amd64.
		return
	}
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
	//)
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
	config := wazero.NewRuntimeConfigInterpreter()
	wazevo.ConfigureWazevo(config)
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

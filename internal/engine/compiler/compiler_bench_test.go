package compiler

import (
	"bytes"
	"fmt"
	"testing"
	"unsafe"

	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wazeroir"
)

func BenchmarkCompiler_compileMemoryCopy(b *testing.B) {
	sizes := []uint32{5, 17, 128, 10000, 64000}

	for _, size := range sizes {
		for _, overlap := range []bool{false, true} {
			b.Run(fmt.Sprintf("%v-%v", size, overlap), func(b *testing.B) {
				env := newCompilerEnvironment()

				mem := env.memory()
				testMem := make([]byte, len(mem))
				for i := 0; i < len(mem); i++ {
					mem[i] = byte(i)
					testMem[i] = byte(i)
				}

				compiler, _ := newCompiler(&wazeroir.CompilationResult{HasMemory: true, Signature: &wasm.FunctionType{}})
				err := compiler.compilePreamble()
				requireNoError(b, err)

				var destOffset, sourceOffset uint32
				if !overlap {
					destOffset, sourceOffset = 1, 777
				} else {
					destOffset, sourceOffset = 777, 1
				}

				err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: destOffset})
				requireNoError(b, err)
				err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: sourceOffset})
				requireNoError(b, err)
				err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: size})
				requireNoError(b, err)
				err = compiler.compileMemoryCopy()
				requireNoError(b, err)
				err = compiler.(compilerImpl).compileReturnFunction()
				requireNoError(b, err)
				code, _, err := compiler.compile()
				requireNoError(b, err)

				env.execBench(b, code)

				for i := 0; i < b.N; i += 1 {
					copy(testMem[destOffset:destOffset+size], testMem[sourceOffset:sourceOffset+size])
				}

				if !bytes.Equal(mem, testMem) {
					b.FailNow()
				}
			})
		}
	}
}

func (j *compilerEnv) execBench(b *testing.B, codeSegment []byte) {
	f := j.newFunctionFrame(codeSegment)

	b.StartTimer()
	for i := 0; i < b.N; i++ {
		j.ce.callFrameStack[j.ce.globalContext.callFrameStackPointer] = callFrame{function: f}
		j.ce.globalContext.callFrameStackPointer++
		nativecall(
			uintptr(unsafe.Pointer(&codeSegment[0])),
			uintptr(unsafe.Pointer(j.ce)),
			uintptr(unsafe.Pointer(j.moduleInstance)),
		)
	}
	b.StopTimer()
}

func requireNoError(b *testing.B, err error) {
	if err != nil {
		b.Fatal(err)
	}
}

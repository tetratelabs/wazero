package compiler

import (
	"bytes"
	"fmt"
	"testing"
	"unsafe"

	"github.com/tetratelabs/wazero/internal/asm"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wazeroir"
)

func BenchmarkCompiler_compileMemoryCopy(b *testing.B) {
	sizes := []uint32{5, 17, 128, 10000, 64000}

	for _, size := range sizes {
		for _, overlap := range []bool{false, true} {
			b.Run(fmt.Sprintf("%v-%v", size, overlap), func(b *testing.B) {
				env := newCompilerEnvironment()
				buf := asm.CodeSegment{}
				defer func() {
					require.NoError(b, buf.Unmap())
				}()

				mem := env.memory()
				testMem := make([]byte, len(mem))
				for i := 0; i < len(mem); i++ {
					mem[i] = byte(i)
					testMem[i] = byte(i)
				}

				compiler := newCompiler()
				compiler.Init(&wasm.FunctionType{}, &wazeroir.CompilationResult{Memory: wazeroir.MemoryTypeStandard}, false)
				err := compiler.compilePreamble()
				require.NoError(b, err)

				var destOffset, sourceOffset uint32
				if !overlap {
					destOffset, sourceOffset = 1, 777
				} else {
					destOffset, sourceOffset = 777, 1
				}

				err = compiler.compileConstI32(operationPtr(wazeroir.NewOperationConstI32(destOffset)))
				require.NoError(b, err)
				err = compiler.compileConstI32(operationPtr(wazeroir.NewOperationConstI32(sourceOffset)))
				require.NoError(b, err)
				err = compiler.compileConstI32(operationPtr(wazeroir.NewOperationConstI32(size)))
				require.NoError(b, err)
				err = compiler.compileMemoryCopy()
				require.NoError(b, err)
				err = compiler.(compilerImpl).compileReturnFunction()

				require.NoError(b, err)
				_, err = compiler.compile(buf.NextCodeSection())
				require.NoError(b, err)

				env.execBench(b, buf.Bytes())

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

func BenchmarkCompiler_compileMemoryFill(b *testing.B) {
	sizes := []uint32{5, 17, 128, 10000, 64000}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("%v", size), func(b *testing.B) {
			env := newCompilerEnvironment()
			buf := asm.CodeSegment{}
			defer func() {
				require.NoError(b, buf.Unmap())
			}()

			compiler := newCompiler()
			compiler.Init(&wasm.FunctionType{}, &wazeroir.CompilationResult{Memory: wazeroir.MemoryTypeStandard}, false)

			var startOffset uint32 = 100
			var value uint8 = 5

			err := compiler.compilePreamble()
			require.NoError(b, err)

			err = compiler.compileConstI32(operationPtr(wazeroir.NewOperationConstI32(startOffset)))
			require.NoError(b, err)
			err = compiler.compileConstI32(operationPtr(wazeroir.NewOperationConstI32(uint32(value))))
			require.NoError(b, err)
			err = compiler.compileConstI32(operationPtr(wazeroir.NewOperationConstI32(size)))
			require.NoError(b, err)
			err = compiler.compileMemoryFill()
			require.NoError(b, err)
			err = compiler.(compilerImpl).compileReturnFunction()
			require.NoError(b, err)
			_, err = compiler.compile(buf.NextCodeSection())
			require.NoError(b, err)

			mem := env.memory()
			testMem := make([]byte, len(mem))
			for i := 0; i < len(mem); i++ {
				mem[i] = byte(i)
				testMem[i] = byte(i)
			}

			env.execBench(b, buf.Bytes())

			for i := startOffset; i < startOffset+size; i++ {
				testMem[i] = value
			}

			for i := 0; i < len(mem); i++ {
				require.Equal(b, mem[i], testMem[i], "mem != %d at offset %d", value, i)
			}
		})
	}
}

func (j *compilerEnv) execBench(b *testing.B, codeSegment []byte) {
	executable := requireExecutable(codeSegment)
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		nativecall(
			uintptr(unsafe.Pointer(&executable[0])),
			j.ce, j.moduleInstance,
		)
	}
	b.StopTimer()
}

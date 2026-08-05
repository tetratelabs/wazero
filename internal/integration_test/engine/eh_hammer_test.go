package adhoc

import (
	"context"
	"fmt"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

// TestEHParallelCompilation is a hammer test that exercises parallel compilation
// of a module with many functions, each containing a try_table. Without proper
// synchronization of catch clause table IDs, parallel workers will assign
// conflicting IDs, causing runtime panics or wrong handler dispatch.
func TestEHParallelCompilation(t *testing.T) {
	if !platform.CompilerSupported() {
		t.Skip()
	}

	const numFuncs = 32
	bin := buildEHModule(numFuncs)

	for _, workers := range []int{2, 4, 8} {
		workers := workers
		t.Run(fmt.Sprintf("workers=%d", workers), func(t *testing.T) {
			ctx := experimental.WithCompilationWorkers(context.Background(), workers)
			cfg := wazero.NewRuntimeConfigCompiler().
				WithCoreFeatures(api.CoreFeaturesV2 | experimental.CoreFeaturesExceptionHandling)
			r := wazero.NewRuntimeWithConfig(ctx, cfg)
			defer r.Close(ctx)

			mod, err := r.InstantiateWithConfig(ctx, bin,
				wazero.NewModuleConfig().WithStartFunctions())
			require.NoError(t, err)

			for i := 0; i < numFuncs; i++ {
				name := fmt.Sprintf("f%d", i)
				res, err := mod.ExportedFunction(name).Call(ctx)
				require.NoError(t, err, "function %s", name)
				require.Equal(t, uint64(i), res[0], "function %s returned wrong value", name)
			}
		})
	}
}

// buildEHModule constructs a wasm binary with `n` exported functions, each
// containing a try_table that catches a throw and returns the function index.
//
// Each function looks like:
//
//	(func (export "fN") (result i32)
//	  block $caught
//	    try_table (catch_all $caught)
//	      throw $tag
//	    end
//	    i32.const -1  ;; unreachable
//	    return
//	  end
//	  i32.const N  ;; caught: return function index
//	)
func buildEHModule(n int) []byte {
	m := &wasm.Module{
		// One type: () -> (i32)
		TypeSection: []wasm.FunctionType{
			{Results: []wasm.ValueType{wasm.ValueTypeI32}},
			{}, // tag type: () -> ()
		},
		// One tag with type 1 (empty params, empty results)
		TagSection: []wasm.Tag{{Type: 1}},
	}

	for i := 0; i < n; i++ {
		m.FunctionSection = append(m.FunctionSection, 0) // type 0: () -> (i32)
		m.ExportSection = append(m.ExportSection, wasm.Export{
			Type:  wasm.ExternTypeFunc,
			Name:  fmt.Sprintf("f%d", i),
			Index: wasm.Index(i),
		})

		// Build the function body:
		//   block void        ;; label 0 = $caught
		//     try_table (catch_all 0)
		//       throw 0
		//     end
		//     i32.const -1
		//     return
		//   end
		//   i32.const <i>
		//   end
		body := []byte{
			wasm.OpcodeBlock, 0x40, // block void
			wasm.OpcodeTryTable, 0x40, // try_table void
			1,                      // catch count = 1
			wasm.CatchKindCatchAll, // catch_all
			0,                      // label index 0 (the enclosing block)
			wasm.OpcodeThrow, 0,    // throw tag 0
			wasm.OpcodeEnd,            // end try_table
			wasm.OpcodeI32Const, 0x7f, // i32.const -1 (signed LEB128)
			wasm.OpcodeReturn,
			wasm.OpcodeEnd, // end block
		}
		// i32.const <i> — encode i as signed LEB128
		body = append(body, wasm.OpcodeI32Const)
		body = appendSleb128(body, int32(i))
		body = append(body, wasm.OpcodeEnd) // end function

		m.CodeSection = append(m.CodeSection, wasm.Code{Body: body})
	}

	return encodeModule(m)
}

// appendSleb128 appends a signed LEB128 encoding of v to b.
func appendSleb128(b []byte, v int32) []byte {
	for {
		c := byte(v & 0x7f)
		v >>= 7
		if (v == 0 && c&0x40 == 0) || (v == -1 && c&0x40 != 0) {
			return append(b, c)
		}
		b = append(b, c|0x80)
	}
}

// encodeModule encodes the wasm.Module into a valid wasm binary.
// This is a minimal encoder that handles the sections used by buildEHModule.
func encodeModule(m *wasm.Module) []byte {
	var buf []byte
	// Magic + version
	buf = append(buf, 0x00, 0x61, 0x73, 0x6d) // \0asm
	buf = append(buf, 0x01, 0x00, 0x00, 0x00) // version 1

	// Type section (id=1)
	buf = appendSection(buf, 1, func(s []byte) []byte {
		s = appendUleb128(s, uint32(len(m.TypeSection)))
		for _, ft := range m.TypeSection {
			s = append(s, 0x60) // func type
			s = appendUleb128(s, uint32(len(ft.Params)))
			s = append(s, wasm.ToApiValueType(ft.Params)...)
			s = appendUleb128(s, uint32(len(ft.Results)))
			s = append(s, wasm.ToApiValueType(ft.Results)...)
		}
		return s
	})

	// Function section (id=3)
	buf = appendSection(buf, 3, func(s []byte) []byte {
		s = appendUleb128(s, uint32(len(m.FunctionSection)))
		for _, idx := range m.FunctionSection {
			s = appendUleb128(s, idx)
		}
		return s
	})

	// Tag section (id=13)
	buf = appendSection(buf, 13, func(s []byte) []byte {
		s = appendUleb128(s, uint32(len(m.TagSection)))
		for _, tag := range m.TagSection {
			s = append(s, 0x00) // attribute byte (must be 0)
			s = appendUleb128(s, tag.Type)
		}
		return s
	})

	// Export section (id=7)
	buf = appendSection(buf, 7, func(s []byte) []byte {
		s = appendUleb128(s, uint32(len(m.ExportSection)))
		for _, exp := range m.ExportSection {
			s = appendUleb128(s, uint32(len(exp.Name)))
			s = append(s, exp.Name...)
			s = append(s, exp.Type)
			s = appendUleb128(s, exp.Index)
		}
		return s
	})

	// Code section (id=10)
	buf = appendSection(buf, 10, func(s []byte) []byte {
		s = appendUleb128(s, uint32(len(m.CodeSection)))
		for _, code := range m.CodeSection {
			// Each code entry: size + locals_count(0) + body
			funcBody := appendUleb128(nil, 0) // 0 locals
			funcBody = append(funcBody, code.Body...)
			s = appendUleb128(s, uint32(len(funcBody)))
			s = append(s, funcBody...)
		}
		return s
	})

	return buf
}

func appendSection(buf []byte, id byte, buildContent func([]byte) []byte) []byte {
	content := buildContent(nil)
	buf = append(buf, id)
	buf = appendUleb128(buf, uint32(len(content)))
	buf = append(buf, content...)
	return buf
}

func appendUleb128(b []byte, v uint32) []byte {
	for {
		c := byte(v & 0x7f)
		v >>= 7
		if v == 0 {
			return append(b, c)
		}
		b = append(b, c|0x80)
	}
}

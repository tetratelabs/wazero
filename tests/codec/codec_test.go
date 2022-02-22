//go:build amd64
// +build amd64

// Wasmtime cannot be used non-amd64 platform.
package example

import (
	"context"
	_ "embed"
	"testing"

	"github.com/bytecodealliance/wasmtime-go"
	"github.com/stretchr/testify/require"
	"github.com/wasmerio/wasmer-go/wasmer"

	"github.com/tetratelabs/wazero"
	wasi "github.com/tetratelabs/wazero/internal/wasi"
	wasm "github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wasm/binary"
	"github.com/tetratelabs/wazero/internal/wasm/text"
	publicwasi "github.com/tetratelabs/wazero/wasi"
)

// example holds the latest supported features as described in the comments of exampleText
var example = newExample()

// exampleText is different from exampleWat because the parser doesn't yet support all features.
//go:embed testdata/example.wat
var exampleText []byte

// exampleBinary is the exampleText encoded in the WebAssembly 1.0 binary format.
var exampleBinary = binary.EncodeModule(example)

func newExample() *wasm.Module {
	three := wasm.Index(3)
	i32 := wasm.ValueTypeI32
	return &wasm.Module{
		TypeSection: []*wasm.FunctionType{
			{Params: []wasm.ValueType{i32, i32}, Results: []wasm.ValueType{i32}},
			{},
			{Params: []wasm.ValueType{i32, i32, i32, i32}, Results: []wasm.ValueType{i32}},
		},
		ImportSection: []*wasm.Import{
			{
				Module: "wasi_snapshot_preview1", Name: wasi.FunctionArgsSizesGet,
				Kind:     wasm.ImportKindFunc,
				DescFunc: 0,
			}, {
				Module: "wasi_snapshot_preview1", Name: wasi.FunctionFdWrite,
				Kind:     wasm.ImportKindFunc,
				DescFunc: 2,
			},
		},
		FunctionSection: []wasm.Index{wasm.Index(1), wasm.Index(1), wasm.Index(0)},
		CodeSection: []*wasm.Code{
			{Body: []byte{wasm.OpcodeCall, 3, wasm.OpcodeEnd}},
			{Body: []byte{wasm.OpcodeEnd}},
			{Body: []byte{wasm.OpcodeLocalGet, 0, wasm.OpcodeLocalGet, 1, wasm.OpcodeI32Add, wasm.OpcodeEnd}},
		},
		MemorySection: []*wasm.MemoryType{{Min: 1, Max: &three}},
		ExportSection: map[string]*wasm.Export{
			"AddInt": {Name: "AddInt", Kind: wasm.ExportKindFunc, Index: wasm.Index(4)},
			"":       {Name: "", Kind: wasm.ExportKindFunc, Index: wasm.Index(3)},
			"mem":    {Name: "mem", Kind: wasm.ExportKindMemory, Index: wasm.Index(0)},
		},
		StartSection: &three,
		NameSection: &wasm.NameSection{
			ModuleName: "example",
			FunctionNames: wasm.NameMap{
				{Index: wasm.Index(0), Name: "wasi.args_sizes_get"},
				{Index: wasm.Index(1), Name: "wasi.fd_write"},
				{Index: wasm.Index(2), Name: "call_hello"},
				{Index: wasm.Index(3), Name: "hello"},
				{Index: wasm.Index(4), Name: "addInt"},
			},
			LocalNames: wasm.IndirectNameMap{
				{Index: wasm.Index(1), NameMap: wasm.NameMap{
					{Index: wasm.Index(0), Name: "fd"},
					{Index: wasm.Index(1), Name: "iovs_ptr"},
					{Index: wasm.Index(2), Name: "iovs_len"},
					{Index: wasm.Index(3), Name: "nwritten_ptr"},
				}},
				{Index: wasm.Index(4), NameMap: wasm.NameMap{
					{Index: wasm.Index(0), Name: "value_1"},
					{Index: wasm.Index(1), Name: "value_2"},
				}},
			},
		},
	}
}

func TestExampleUpToDate(t *testing.T) {
	t.Run("binary.DecodeModule", func(t *testing.T) {
		m, err := binary.DecodeModule(exampleBinary)
		require.NoError(t, err)
		require.Equal(t, example, m)
	})

	t.Run("text.DecodeModule", func(t *testing.T) {
		m, err := text.DecodeModule(exampleText)
		require.NoError(t, err)
		require.Equal(t, example, m)
	})

	t.Run("Executable", func(t *testing.T) {
		store := wazero.NewStore()

		// Add WASI to satisfy import tests
		_, err := wazero.ExportHostFunctions(store, publicwasi.ModuleSnapshotPreview1, wazero.WASISnapshotPreview1())
		require.NoError(t, err)

		// Decode and instantiate the module
		mod, err := wazero.DecodeModuleBinary(exampleBinary)
		require.NoError(t, err)
		exports, err := wazero.InstantiateModule(store, mod)
		require.NoError(t, err)

		// Call the add function as a smoke test
		results, err := exports.Function("AddInt")(context.Background(), uint64(1), uint64(2))
		require.NoError(t, err)
		require.Equal(t, uint64(3), results[0])
	})
}

func BenchmarkCodecExample(b *testing.B) {
	b.Run("binary.DecodeModule", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := binary.DecodeModule(exampleBinary); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("binary.EncodeModule", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = binary.EncodeModule(example)
		}
	})
	b.Run("text.DecodeModule", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := text.DecodeModule(exampleText); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("wat2wasm via text.DecodeModule->binary.EncodeModule", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if m, err := text.DecodeModule(exampleText); err != nil {
				b.Fatal(err)
			} else {
				_ = binary.EncodeModule(m)
			}
		}
	})
	// Note: We don't know if wasmer.Wat2Wasm encodes the custom name section or not.
	// Note: wasmer.Wat2Wasm calls wasmer via CGO which is eventually implemented by wasm-tools
	b.Run("wat2wasm vs wasmer.Wat2Wasm", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := wasmer.Wat2Wasm(string(exampleText))
			if err != nil {
				panic(err)
			}
		}
	})
	// Note: We don't know if wasmtime.Wat2Wasm encodes the custom name section or not.
	// Note: wasmtime.Wat2Wasm calls wasmtime via CGO which is eventually implemented by wasm-tools
	b.Run("wat2wasm vs wasmtime.Wat2Wasm", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := wasmtime.Wat2Wasm(string(exampleText))
			if err != nil {
				panic(err)
			}
		}
	})
}

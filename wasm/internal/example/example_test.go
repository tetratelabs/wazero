package example

import (
	"bytes"
	_ "embed"
	"os"
	"testing"

	"github.com/bytecodealliance/wasmtime-go"
	"github.com/stretchr/testify/require"

	wasm "github.com/tetratelabs/wazero/wasm"
	"github.com/tetratelabs/wazero/wasm/binary"
	"github.com/tetratelabs/wazero/wasm/text"
)

// example holds the latest supported features as described in the comments of exampleText
var example = newExample()

// exampleText is different from exampleWat because the parser doesn't yet support all features.
//go:embed testdata/example.wat
var exampleText []byte

// exampleBinary is derived from exampleText
//go:embed testdata/example.wasm
var exampleBinary []byte

func newExample() *wasm.Module {
	four := wasm.Index(4)
	f32, i32 := wasm.ValueTypeF32, wasm.ValueTypeI32
	return &wasm.Module{
		TypeSection: []*wasm.FunctionType{
			{Params: []wasm.ValueType{i32, i32}, Results: []wasm.ValueType{i32}},
			{},
			{Params: []wasm.ValueType{i32, i32, i32, i32}, Results: []wasm.ValueType{i32}},
			{Params: []wasm.ValueType{f32, f32}, Results: []wasm.ValueType{f32}},
		},
		ImportSection: []*wasm.Import{
			{
				Module: "wasi_snapshot_preview1", Name: "args_sizes_get",
				Kind:     wasm.ImportKindFunc,
				DescFunc: 0,
			}, {
				Module: "wasi_snapshot_preview1", Name: "fd_write",
				Kind:     wasm.ImportKindFunc,
				DescFunc: 2,
			}, {
				Module: "Math", Name: "Mul",
				Kind:     wasm.ImportKindFunc,
				DescFunc: 3,
			}, {
				Module: "Math", Name: "Add",
				Kind:     wasm.ImportKindFunc,
				DescFunc: 0,
			}, {
				Module: "", Name: "hello",
				Kind:     wasm.ImportKindFunc,
				DescFunc: 1,
			},
		},
		FunctionSection: []wasm.Index{wasm.Index(0)},
		ExportSection: map[string]*wasm.Export{
			"AddInt": {Name: "AddInt", Kind: wasm.ExportKindFunc, Index: wasm.Index(5)},
		},
		CodeSection: []*wasm.Code{
			{Body: []byte{wasm.OpcodeLocalGet, 0, wasm.OpcodeLocalGet, 1, wasm.OpcodeI32Add, wasm.OpcodeEnd}},
		},
		StartSection: &four,
		NameSection: &wasm.NameSection{
			ModuleName: "example",
			FunctionNames: wasm.NameMap{
				{Index: wasm.Index(0), Name: "runtime.args_sizes_get"},
				{Index: wasm.Index(1), Name: "runtime.fd_write"},
				{Index: wasm.Index(2), Name: "mul"},
				{Index: wasm.Index(3), Name: "add"},
				{Index: wasm.Index(4), Name: "hello"},
				{Index: wasm.Index(5), Name: "addInt"},
			},
			LocalNames: wasm.IndirectNameMap{
				{Index: wasm.Index(1), NameMap: wasm.NameMap{
					{Index: wasm.Index(0), Name: "fd"},
					{Index: wasm.Index(1), Name: "iovs_ptr"},
					{Index: wasm.Index(2), Name: "iovs_len"},
					{Index: wasm.Index(3), Name: "nwritten_ptr"},
				}},
				{Index: wasm.Index(2), NameMap: wasm.NameMap{
					{Index: wasm.Index(0), Name: "x"},
					{Index: wasm.Index(1), Name: "y"},
				}},
				{Index: wasm.Index(3), NameMap: wasm.NameMap{
					{Index: wasm.Index(0), Name: "l"},
					{Index: wasm.Index(1), Name: "r"},
				}},
				{Index: wasm.Index(5), NameMap: wasm.NameMap{
					{Index: wasm.Index(0), Name: "value_1"},
					{Index: wasm.Index(1), Name: "value_2"},
				}},
			},
		},
	}
}

func TestExampleUpToDate(t *testing.T) {
	encoded := binary.EncodeModule(example)
	// This means we changed something. Overwrite the example wasm file rather than force maintainers to use hex editor!
	if !bytes.Equal(encoded, exampleBinary) {
		require.NoError(t, os.WriteFile("testdata/example.wasm", binary.EncodeModule(example), 0600))
	}

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
	// Note: We don't know if wasmtime.Wat2Wasm encodes the custom name section or not.
	b.Run("wat2wasm via wasmtime.Wat2Wasm", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := wasmtime.Wat2Wasm(string(exampleText))
			if err != nil {
				panic(err)
			}
		}
	})
}

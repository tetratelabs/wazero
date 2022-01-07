package wat

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/wasm"
)

func TestEncodeNameSection(t *testing.T) {
	m, err := parseModule(simpleExample)
	require.NoError(t, err)

	// TIP: the below is the binary suffix of `wat2wasm --debug-names --debug-parser -v simple.wat` where simple.wat
	// contains the same text as simpleExample
	require.Equal(t, []byte{
		0x00, // module subsection ID zero
		0x07, // 7 bytes to follow
		0x06, // the module name simple is 6 characters long
		's', 'i', 'm', 'p', 'l', 'e',
		0x01, // function subsection ID one
		0x08, // 8 bytes to follow
		0x01, // one function name
		0x00, // the function index is zero
		0x05, // the function name hello is 5 characters long
		'h', 'e', 'l', 'l', 'o',
	}, encodeNameSection(m))
}

// TestEncodeNameSection_OnlyFuncName shows that we don't rely on the module name being present. For example, this isn't
// encoded in TinyGo.
func TestEncodeNameSection_OnlyFuncName(t *testing.T) {
	func0, func1 := "runtime.args_sizes_get", "runtime.fd_write" // note: no leading '$'
	i32 := wasm.ValueTypeI32
	m := &module{
		types: []*typeFunc{
			{params: []wasm.ValueType{i32, i32}, results: []wasm.ValueType{i32}},
			{params: []wasm.ValueType{i32, i32, i32, i32}, results: []wasm.ValueType{i32}},
		},
		importFuncs: []*importFunc{
			{importIndex: 0, typeIndex: 0, module: "wasi_snapshot_preview1", name: "args_sizes_get", funcName: "$" + func0},
			{importIndex: 1, typeIndex: 1, module: "wasi_snapshot_preview1", name: "fd_write", funcName: "$" + func1},
		},
	}

	expected := append(append([]byte{
		0x01, // function subsection ID one
		// length includes overhead for size in bytes of the function name count, plus index + length prefix per name
		byte(1 + 2 + 2 + len(func0) + len(func1)),
		0x02, // two function names
	},
		append([]byte{0x00 /* funcIndex */, byte(len(func0))}, func0...)...),
		append([]byte{0x01 /* funcIndex */, byte(len(func1))}, func1...)...)

	require.Equal(t, expected, encodeNameSection(m))
}

func TestEncodeNameSubsection(t *testing.T) {
	subsectionID := uint8(1)
	name := "$simple"
	require.Equal(t, []byte{
		subsectionID,
		byte(1 + 6), // 1 is the size of 6 in LEB128 encoding
		6, 's', 'i', 'm', 'p', 'l', 'e'}, encodeNameSubsection(subsectionID, encodeName(name)))
}

func TestEncodeNameMapEntry(t *testing.T) {
	index := uint32(1)
	name := "$hello"
	require.Equal(t, []byte{byte(index), 5, 'h', 'e', 'l', 'l', 'o'}, encodeNameMapEntry(index, name))
}

func TestEncodeName(t *testing.T) {
	// We expect a length (in LEB128) prefixed string encoding
	require.Equal(t, []byte{5, 'h', 'e', 'l', 'l', 'o'}, encodeName("$hello"))
}

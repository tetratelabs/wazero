package wazevo

import (
	"bytes"
	"crypto/sha256"
	"hash/crc32"
	"io"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/u32"
	"github.com/tetratelabs/wazero/internal/u64"
	"github.com/tetratelabs/wazero/internal/wasm"
)

var testVersion = "0.0.1"

func crcf(b []byte) []byte {
	c := crc32.Checksum(b, crc)
	return u32.LeBytes(c)
}

func TestSerializeCompiledModule(t *testing.T) {
	tests := []struct {
		in  *compiledModule
		exp []byte
	}{
		{
			in: &compiledModule{
				executables:     &executables{executable: []byte{1, 2, 3, 4, 5}},
				functionOffsets: []int{0},
			},
			exp: concat(
				magic,
				[]byte{byte(len(testVersion))},
				[]byte(testVersion),
				u32.LeBytes(1),              // number of functions.
				u64.LeBytes(0),              // offset.
				u64.LeBytes(5),              // length of code.
				[]byte{1, 2, 3, 4, 5},       // code.
				crcf([]byte{1, 2, 3, 4, 5}), // crc for the code.
				[]byte{0},                   // no source map.
			),
		},
		{
			in: &compiledModule{
				executables:     &executables{executable: []byte{1, 2, 3, 4, 5}},
				functionOffsets: []int{0},
			},
			exp: concat(
				magic,
				[]byte{byte(len(testVersion))},
				[]byte(testVersion),
				u32.LeBytes(1),              // number of functions.
				u64.LeBytes(0),              // offset.
				u64.LeBytes(5),              // length of code.
				[]byte{1, 2, 3, 4, 5},       // code.
				crcf([]byte{1, 2, 3, 4, 5}), // crc for the code.
				[]byte{0},                   // no source map.
			),
		},
		{
			in: &compiledModule{
				executables:     &executables{executable: []byte{1, 2, 3, 4, 5, 1, 2, 3}},
				functionOffsets: []int{0, 5},
			},
			exp: concat(
				magic,
				[]byte{byte(len(testVersion))},
				[]byte(testVersion),
				u32.LeBytes(2), // number of functions.
				// Function index = 0.
				u64.LeBytes(0), // offset.
				// Function index = 1.
				u64.LeBytes(5), // offset.
				// Executable.
				u64.LeBytes(8),                       // length of code.
				[]byte{1, 2, 3, 4, 5, 1, 2, 3},       // code.
				crcf([]byte{1, 2, 3, 4, 5, 1, 2, 3}), // crc for the code.
				[]byte{0},                            // no source map.
			),
		},
	}

	for i, tc := range tests {
		actual, err := io.ReadAll(serializeCompiledModule(testVersion, tc.in))
		require.NoError(t, err, i)
		require.Equal(t, tc.exp, actual, i)
	}
}

func concat(ins ...[]byte) (ret []byte) {
	for _, in := range ins {
		ret = append(ret, in...)
	}
	return
}

func TestDeserializeCompiledModule(t *testing.T) {
	tests := []struct {
		name                  string
		in                    []byte
		importedFunctionCount uint32
		expCompiledModule     *compiledModule
		expStaleCache         bool
		expErr                string
	}{
		{
			name:   "invalid header",
			in:     []byte{1},
			expErr: "compilationcache: invalid header length: 1",
		},
		{
			name: "invalid magic",
			in: concat(
				[]byte{'a', 'b', 'c', 'd', 'e', 'f'},
				[]byte{byte(len(testVersion))},
				[]byte(testVersion),
				u32.LeBytes(1), // number of functions.
			),
			expErr: "compilationcache: invalid magic number: got WAZEVO but want abcdef",
		},
		{
			name: "version mismatch",
			in: concat(
				magic,
				[]byte{byte(len("1233123.1.1"))},
				[]byte("1233123.1.1"),
				u32.LeBytes(1), // number of functions.
			),
			expStaleCache: true,
		},
		{
			name: "version mismatch",
			in: concat(
				magic,
				[]byte{byte(len("0.0.0"))},
				[]byte("0.0.0"),
				u32.LeBytes(1), // number of functions.
			),
			expStaleCache: true,
		},
		{
			name: "one function",
			in: concat(
				magic,
				[]byte{byte(len(testVersion))},
				[]byte(testVersion),
				u32.LeBytes(1), // number of functions.
				u64.LeBytes(0), // offset.
				// Executable.
				u64.LeBytes(5),              // size.
				[]byte{1, 2, 3, 4, 5},       // machine code.
				crcf([]byte{1, 2, 3, 4, 5}), // machine code.
				[]byte{0},                   // no source map.
			),
			expCompiledModule: &compiledModule{
				executables:     &executables{executable: []byte{1, 2, 3, 4, 5}},
				functionOffsets: []int{0},
			},
			expStaleCache: false,
			expErr:        "",
		},
		{
			name: "two functions",
			in: concat(
				magic,
				[]byte{byte(len(testVersion))},
				[]byte(testVersion),
				u32.LeBytes(2), // number of functions.
				// Function index = 0.
				u64.LeBytes(0), // offset.
				// Function index = 1.
				u64.LeBytes(7), // offset.
				// Executable.
				u64.LeBytes(10),                             // size.
				[]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},       // machine code.
				crcf([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}), // crc for machine code.
				[]byte{0}, // no source map.
			),
			importedFunctionCount: 1,
			expCompiledModule: &compiledModule{
				executables:     &executables{executable: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}},
				functionOffsets: []int{0, 7},
			},
			expStaleCache: false,
			expErr:        "",
		},
		{
			name: "reading executable offset",
			in: concat(
				[]byte(magic),
				[]byte{byte(len(testVersion))},
				[]byte(testVersion),
				u32.LeBytes(2), // number of functions.
				// Function index = 0.
				u64.LeBytes(5), // offset.
				// Function index = 1.
			),
			expErr: "compilationcache: error reading func[1] executable offset: EOF",
		},
		{
			name: "mmapping",
			in: concat(
				[]byte(magic),
				[]byte{byte(len(testVersion))},
				[]byte(testVersion),
				u32.LeBytes(2), // number of functions.
				// Function index = 0.
				u64.LeBytes(0), // offset.
				// Function index = 1.
				u64.LeBytes(5), // offset.
				// Executable.
				u64.LeBytes(5), // size of the executable.
				// Lack of machine code here.
			),
			expErr: "compilationcache: error reading executable (len=5): EOF",
		},
		{
			name: "bad crc",
			in: concat(
				magic,
				[]byte{byte(len(testVersion))},
				[]byte(testVersion),
				u32.LeBytes(1), // number of functions.
				u64.LeBytes(0), // offset.
				// Executable.
				u64.LeBytes(5),        // size.
				[]byte{1, 2, 3, 4, 5}, // machine code.
				[]byte{1, 2, 3, 4},    // crc for machine code.
			),
			expCompiledModule: &compiledModule{
				executables:     &executables{executable: []byte{1, 2, 3, 4, 5}},
				functionOffsets: []int{0},
			},
			expStaleCache: false,
			expErr:        "compilationcache: checksum mismatch (expected 1397854123, got 67305985)",
		},
		{
			name: "missing crc",
			in: concat(
				magic,
				[]byte{byte(len(testVersion))},
				[]byte(testVersion),
				u32.LeBytes(1), // number of functions.
				u64.LeBytes(0), // offset.
				// Executable.
				u64.LeBytes(5),        // size.
				[]byte{1, 2, 3, 4, 5}, // machine code.
			),
			expCompiledModule: &compiledModule{
				executables:     &executables{executable: []byte{1, 2, 3, 4, 5}},
				functionOffsets: []int{0},
			},
			expStaleCache: false,
			expErr:        "compilationcache: could not read checksum: EOF",
		},
		{
			name: "no source map presence",
			in: concat(
				magic,
				[]byte{byte(len(testVersion))},
				[]byte(testVersion),
				u32.LeBytes(1), // number of functions.
				u64.LeBytes(0), // offset.
				// Executable.
				u64.LeBytes(5),              // size.
				[]byte{1, 2, 3, 4, 5},       // machine code.
				crcf([]byte{1, 2, 3, 4, 5}), // crc for machine code.
			),
			expCompiledModule: &compiledModule{
				executables:     &executables{executable: []byte{1, 2, 3, 4, 5}},
				functionOffsets: []int{0},
			},
			expStaleCache: false,
			expErr:        "compilationcache: error reading source map presence: EOF",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			cm, staleCache, err := deserializeCompiledModule(testVersion, io.NopCloser(bytes.NewReader(tc.in)))

			if tc.expErr != "" {
				require.EqualError(t, err, tc.expErr)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expCompiledModule, cm)
			}

			require.Equal(t, tc.expStaleCache, staleCache)
		})
	}
}

func Test_fileCacheKey(t *testing.T) {
	s := sha256.New()
	s.Write([]byte("hello world"))
	m := &wasm.Module{}
	s.Sum(m.ID[:0])
	original := m.ID
	result := fileCacheKey(m)
	require.Equal(t, original, m.ID)
	require.NotEqual(t, original, result)
}

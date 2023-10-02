package wazevo

import (
	"bytes"
	"io"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/u32"
	"github.com/tetratelabs/wazero/internal/u64"
)

var testVersion = "0.0.1"

func TestSerializeCompiledModule(t *testing.T) {
	tests := []struct {
		in  *compiledModule
		exp []byte
	}{
		{
			in: &compiledModule{
				executable:      []byte{1, 2, 3, 4, 5},
				functionOffsets: []int{0},
			},
			exp: concat(
				[]byte(magic),
				[]byte{byte(len(testVersion))},
				[]byte(testVersion),
				u32.LeBytes(1),        // number of functions.
				u64.LeBytes(0),        // offset.
				u64.LeBytes(5),        // length of code.
				[]byte{1, 2, 3, 4, 5}, // code.
			),
		},
		{
			in: &compiledModule{
				executable:      []byte{1, 2, 3, 4, 5},
				functionOffsets: []int{0},
			},
			exp: concat(
				[]byte(magic),
				[]byte{byte(len(testVersion))},
				[]byte(testVersion),
				u32.LeBytes(1),        // number of functions.
				u64.LeBytes(0),        // offset.
				u64.LeBytes(5),        // length of code.
				[]byte{1, 2, 3, 4, 5}, // code.
			),
		},
		{
			in: &compiledModule{
				executable:      []byte{1, 2, 3, 4, 5, 1, 2, 3},
				functionOffsets: []int{0, 5},
			},
			exp: concat(
				[]byte(magic),
				[]byte{byte(len(testVersion))},
				[]byte(testVersion),
				u32.LeBytes(2), // number of functions.
				// Function index = 0.
				u64.LeBytes(0), // offset.
				// Function index = 1.
				u64.LeBytes(5), // offset.
				// Executable.
				u64.LeBytes(8),                 // length of code.
				[]byte{1, 2, 3, 4, 5, 1, 2, 3}, // code.
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
			name: "version mismatch",
			in: concat(
				[]byte(magic),
				[]byte{byte(len("1233123.1.1"))},
				[]byte("1233123.1.1"),
				u32.LeBytes(1), // number of functions.
			),
			expStaleCache: true,
		},
		{
			name: "version mismatch",
			in: concat(
				[]byte(magic),
				[]byte{byte(len("0.0.0"))},
				[]byte("0.0.0"),
				u32.LeBytes(1), // number of functions.
			),
			expStaleCache: true,
		},
		{
			name: "one function",
			in: concat(
				[]byte(magic),
				[]byte{byte(len(testVersion))},
				[]byte(testVersion),
				u32.LeBytes(1), // number of functions.
				u64.LeBytes(0), // offset.
				// Executable.
				u64.LeBytes(5),        // size.
				[]byte{1, 2, 3, 4, 5}, // machine code.
			),
			expCompiledModule: &compiledModule{
				executable:      []byte{1, 2, 3, 4, 5},
				functionOffsets: []int{0},
			},
			expStaleCache: false,
			expErr:        "",
		},
		{
			name: "two functions",
			in: concat(
				[]byte(magic),
				[]byte{byte(len(testVersion))},
				[]byte(testVersion),
				u32.LeBytes(2), // number of functions.
				// Function index = 0.
				u64.LeBytes(0), // offset.
				// Function index = 1.
				u64.LeBytes(7), // offset.
				// Executable.
				u64.LeBytes(10),                       // size.
				[]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, // machine code.
			),
			importedFunctionCount: 1,
			expCompiledModule: &compiledModule{
				executable:      []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
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

package compiler

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"io"
	"math"
	"testing"
	"testing/iotest"

	"github.com/tetratelabs/wazero/internal/asm"
	"github.com/tetratelabs/wazero/internal/filecache"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/u32"
	"github.com/tetratelabs/wazero/internal/u64"
	"github.com/tetratelabs/wazero/internal/wasm"
)

var testVersion = ""

func crcf(b []byte) []byte {
	c := crc32.Checksum(b, crc)
	return u32.LeBytes(c)
}

func concat(ins ...[]byte) (ret []byte) {
	for _, in := range ins {
		ret = append(ret, in...)
	}
	return
}

func makeCodeSegment(bytes ...byte) asm.CodeSegment {
	return *asm.NewCodeSegment(bytes)
}

func TestSerializeCompiledModule(t *testing.T) {
	tests := []struct {
		in  *compiledModule
		exp []byte
	}{
		{
			in: &compiledModule{
				compiledCode: &compiledCode{
					executable: makeCodeSegment(1, 2, 3, 4, 5),
				},
				functions: []compiledFunction{
					{executableOffset: 0, stackPointerCeil: 12345},
				},
			},
			exp: concat(
				[]byte(wazeroMagic),
				[]byte{byte(len(testVersion))},
				[]byte(testVersion),
				[]byte{0},                   // ensure termination.
				u32.LeBytes(1),              // number of functions.
				u64.LeBytes(12345),          // stack pointer ceil.
				u64.LeBytes(0),              // offset.
				u64.LeBytes(5),              // length of code.
				[]byte{1, 2, 3, 4, 5},       // code.
				crcf([]byte{1, 2, 3, 4, 5}), // crc of code.
			),
		},
		{
			in: &compiledModule{
				compiledCode: &compiledCode{
					executable: makeCodeSegment(1, 2, 3, 4, 5),
				},
				functions: []compiledFunction{
					{executableOffset: 0, stackPointerCeil: 12345},
				},
				ensureTermination: true,
			},
			exp: concat(
				[]byte(wazeroMagic),
				[]byte{byte(len(testVersion))},
				[]byte(testVersion),
				[]byte{1},                   // ensure termination.
				u32.LeBytes(1),              // number of functions.
				u64.LeBytes(12345),          // stack pointer ceil.
				u64.LeBytes(0),              // offset.
				u64.LeBytes(5),              // length of code.
				[]byte{1, 2, 3, 4, 5},       // code.
				crcf([]byte{1, 2, 3, 4, 5}), // crc of code.
			),
		},
		{
			in: &compiledModule{
				compiledCode: &compiledCode{
					executable: makeCodeSegment(1, 2, 3, 4, 5, 1, 2, 3),
				},
				functions: []compiledFunction{
					{executableOffset: 0, stackPointerCeil: 12345},
					{executableOffset: 5, stackPointerCeil: 0xffffffff},
				},
				ensureTermination: true,
			},
			exp: concat(
				[]byte(wazeroMagic),
				[]byte{byte(len(testVersion))},
				[]byte(testVersion),
				[]byte{1},      // ensure termination.
				u32.LeBytes(2), // number of functions.
				// Function index = 0.
				u64.LeBytes(12345), // stack pointer ceil.
				u64.LeBytes(0),     // offset.
				// Function index = 1.
				u64.LeBytes(0xffffffff), // stack pointer ceil.
				u64.LeBytes(5),          // offset.
				// Executable.
				u64.LeBytes(8),                       // length of code.
				[]byte{1, 2, 3, 4, 5, 1, 2, 3},       // code.
				crcf([]byte{1, 2, 3, 4, 5, 1, 2, 3}), // crc of code.
			),
		},
	}

	for i, tc := range tests {
		actual, err := io.ReadAll(serializeCompiledModule(testVersion, tc.in))
		require.NoError(t, err, i)
		require.Equal(t, tc.exp, actual, i)
	}
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
				[]byte(wazeroMagic),
				[]byte{byte(len("1233123.1.1"))},
				[]byte("1233123.1.1"),
				u32.LeBytes(1), // number of functions.
			),
			expStaleCache: true,
		},
		{
			name: "version mismatch",
			in: concat(
				[]byte(wazeroMagic),
				[]byte{byte(len("1"))},
				[]byte("1"),
				u32.LeBytes(1), // number of functions.
			),
			expStaleCache: true,
		},
		{
			name: "invalid crc",
			in: concat(
				[]byte(wazeroMagic),
				[]byte{byte(len(testVersion))},
				[]byte(testVersion),
				[]byte{0},          // ensure termination.
				u32.LeBytes(1),     // number of functions.
				u64.LeBytes(12345), // stack pointer ceil.
				u64.LeBytes(0),     // offset.
				// Executable.
				u64.LeBytes(5),           // size.
				[]byte{1, 2, 3, 4, 5},    // machine code.
				crcf([]byte{1, 2, 3, 4}), // crc of code.
			),
			expStaleCache: false,
			expErr:        "compilationcache: checksum mismatch (expected 1397854123, got 691047668)",
		},
		{
			name: "missing crc",
			in: concat(
				[]byte(wazeroMagic),
				[]byte{byte(len(testVersion))},
				[]byte(testVersion),
				[]byte{0},          // ensure termination.
				u32.LeBytes(1),     // number of functions.
				u64.LeBytes(12345), // stack pointer ceil.
				u64.LeBytes(0),     // offset.
				// Executable.
				u64.LeBytes(5),        // size.
				[]byte{1, 2, 3, 4, 5}, // machine code.
			),
			expStaleCache: false,
			expErr:        "compilationcache: could not read checksum: EOF",
		},
		{
			name: "one function",
			in: concat(
				[]byte(wazeroMagic),
				[]byte{byte(len(testVersion))},
				[]byte(testVersion),
				[]byte{0},          // ensure termination.
				u32.LeBytes(1),     // number of functions.
				u64.LeBytes(12345), // stack pointer ceil.
				u64.LeBytes(0),     // offset.
				// Executable.
				u64.LeBytes(5),              // size.
				[]byte{1, 2, 3, 4, 5},       // machine code.
				crcf([]byte{1, 2, 3, 4, 5}), // crc of code.
			),
			expCompiledModule: &compiledModule{
				compiledCode: &compiledCode{
					executable: makeCodeSegment(1, 2, 3, 4, 5),
				},
				functions: []compiledFunction{
					{executableOffset: 0, stackPointerCeil: 12345, index: 0},
				},
			},
			expStaleCache: false,
			expErr:        "",
		},
		{
			name: "one function with ensure termination",
			in: concat(
				[]byte(wazeroMagic),
				[]byte{byte(len(testVersion))},
				[]byte(testVersion),
				[]byte{1},                   // ensure termination.
				u32.LeBytes(1),              // number of functions.
				u64.LeBytes(12345),          // stack pointer ceil.
				u64.LeBytes(0),              // offset.
				u64.LeBytes(5),              // length of code.
				[]byte{1, 2, 3, 4, 5},       // code.
				crcf([]byte{1, 2, 3, 4, 5}), // crc of code.
			),
			expCompiledModule: &compiledModule{
				compiledCode: &compiledCode{
					executable: makeCodeSegment(1, 2, 3, 4, 5),
				},
				functions:         []compiledFunction{{executableOffset: 0, stackPointerCeil: 12345, index: 0}},
				ensureTermination: true,
			},
			expStaleCache: false,
			expErr:        "",
		},
		{
			name: "two functions",
			in: concat(
				[]byte(wazeroMagic),
				[]byte{byte(len(testVersion))},
				[]byte(testVersion),
				[]byte{0},      // ensure termination.
				u32.LeBytes(2), // number of functions.
				// Function index = 0.
				u64.LeBytes(12345), // stack pointer ceil.
				u64.LeBytes(0),     // offset.
				// Function index = 1.
				u64.LeBytes(0xffffffff), // stack pointer ceil.
				u64.LeBytes(7),          // offset.
				// Executable.
				u64.LeBytes(10),                             // size.
				[]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},       // machine code.
				crcf([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}), // crc of code.
			),
			importedFunctionCount: 1,
			expCompiledModule: &compiledModule{
				compiledCode: &compiledCode{
					executable: makeCodeSegment(1, 2, 3, 4, 5, 6, 7, 8, 9, 10),
				},
				functions: []compiledFunction{
					{executableOffset: 0, stackPointerCeil: 12345, index: 1},
					{executableOffset: 7, stackPointerCeil: 0xffffffff, index: 2},
				},
			},
			expStaleCache: false,
			expErr:        "",
		},
		{
			name: "reading stack pointer",
			in: concat(
				[]byte(wazeroMagic),
				[]byte{byte(len(testVersion))},
				[]byte(testVersion),
				[]byte{0},      // ensure termination.
				u32.LeBytes(2), // number of functions.
				// Function index = 0.
				u64.LeBytes(12345), // stack pointer ceil.
				u64.LeBytes(5),     // offset.
				// Function index = 1.
			),
			expErr: "compilationcache: error reading func[1] stack pointer ceil: EOF",
		},
		{
			name: "reading executable offset",
			in: concat(
				[]byte(wazeroMagic),
				[]byte{byte(len(testVersion))},
				[]byte(testVersion),
				[]byte{0},      // ensure termination.
				u32.LeBytes(2), // number of functions.
				// Function index = 0.
				u64.LeBytes(12345), // stack pointer ceil.
				u64.LeBytes(5),     // offset.
				// Function index = 1.
				u64.LeBytes(12345), // stack pointer ceil.
			),
			expErr: "compilationcache: error reading func[1] executable offset: EOF",
		},
		{
			name: "mmapping",
			in: concat(
				[]byte(wazeroMagic),
				[]byte{byte(len(testVersion))},
				[]byte(testVersion),
				[]byte{0},      // ensure termination.
				u32.LeBytes(2), // number of functions.
				// Function index = 0.
				u64.LeBytes(12345), // stack pointer ceil.
				u64.LeBytes(0),     // offset.
				// Function index = 1.
				u64.LeBytes(12345), // stack pointer ceil.
				u64.LeBytes(5),     // offset.
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
			cm, staleCache, err := deserializeCompiledModule(testVersion, io.NopCloser(bytes.NewReader(tc.in)),
				&wasm.Module{ImportFunctionCount: tc.importedFunctionCount})

			if tc.expCompiledModule != nil {
				require.Equal(t, len(tc.expCompiledModule.functions), len(cm.functions))
				for i := 0; i < len(cm.functions); i++ {
					require.Equal(t, cm.compiledCode, cm.functions[i].parent)
					tc.expCompiledModule.functions[i].parent = cm.compiledCode
				}
			}

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

func TestEngine_getCompiledModuleFromCache(t *testing.T) {
	valid := concat(
		[]byte(wazeroMagic),
		[]byte{byte(len(testVersion))},
		[]byte(testVersion),
		[]byte{0},      // ensure termination.
		u32.LeBytes(2), // number of functions.
		// Function index = 0.
		u64.LeBytes(12345), // stack pointer ceil.
		u64.LeBytes(0),     // offset.
		// Function index = 1.
		u64.LeBytes(0xffffffff), // stack pointer ceil.
		u64.LeBytes(5),          // offset.
		// executables.
		u64.LeBytes(10),                             // length of code.
		[]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},       // code.
		crcf([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}), // code.
	)

	tests := []struct {
		name              string
		ext               map[wasm.ModuleID][]byte
		key               wasm.ModuleID
		isHostMod         bool
		expCompiledModule *compiledModule
		expHit            bool
		expErr            string
		expDeleted        bool
	}{
		{name: "extern cache not given"},
		{
			name: "not hit",
			ext:  map[wasm.ModuleID][]byte{},
		},
		{
			name:      "host module",
			ext:       map[wasm.ModuleID][]byte{{}: valid},
			isHostMod: true,
		},
		{
			name:   "error in Cache.Get",
			ext:    map[wasm.ModuleID][]byte{{}: {}},
			expErr: "compilationcache: error reading header: EOF",
		},
		{
			name:   "error in deserialization",
			ext:    map[wasm.ModuleID][]byte{{}: {1, 2, 3}},
			expErr: "compilationcache: invalid header length: 3",
		},
		{
			name: "stale cache",
			ext: map[wasm.ModuleID][]byte{{}: concat(
				[]byte(wazeroMagic),
				[]byte{byte(len("1233123.1.1"))},
				[]byte("1233123.1.1"),
				u32.LeBytes(1), // number of functions.
			)},
			expDeleted: true,
		},
		{
			name: "hit",
			ext: map[wasm.ModuleID][]byte{
				{}: valid,
			},
			expHit: true,
			expCompiledModule: &compiledModule{
				compiledCode: &compiledCode{
					executable: makeCodeSegment(1, 2, 3, 4, 5, 6, 7, 8, 9, 10),
				},
				functions: []compiledFunction{
					{stackPointerCeil: 12345, executableOffset: 0, index: 0},
					{stackPointerCeil: 0xffffffff, executableOffset: 5, index: 1},
				},
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			m := &wasm.Module{ID: tc.key, IsHostModule: tc.isHostMod}
			if exp := tc.expCompiledModule; exp != nil {
				exp.source = m
				for i := range tc.expCompiledModule.functions {
					tc.expCompiledModule.functions[i].parent = exp.compiledCode
				}
			}

			e := engine{}
			if tc.ext != nil {
				tmp := t.TempDir()
				e.fileCache = filecache.New(tmp)
				for key, value := range tc.ext {
					err := e.fileCache.Add(key, bytes.NewReader(value))
					require.NoError(t, err)
				}
			}

			codes, hit, err := e.getCompiledModuleFromCache(m)
			if tc.expErr != "" {
				require.EqualError(t, err, tc.expErr)
			} else {
				require.NoError(t, err)
			}

			require.Equal(t, tc.expHit, hit)
			require.Equal(t, tc.expCompiledModule, codes)

			if tc.ext != nil && tc.expDeleted {
				_, hit, err := e.fileCache.Get(tc.key)
				require.NoError(t, err)
				require.False(t, hit)
			}
		})
	}
}

func TestEngine_addCompiledModuleToCache(t *testing.T) {
	t.Run("not defined", func(t *testing.T) {
		e := engine{}
		err := e.addCompiledModuleToCache(nil, nil)
		require.NoError(t, err)
	})
	t.Run("host module", func(t *testing.T) {
		tc := filecache.New(t.TempDir())
		e := engine{fileCache: tc}
		cm := &compiledModule{
			compiledCode: &compiledCode{
				executable: makeCodeSegment(1, 2, 3),
			},
			functions: []compiledFunction{{stackPointerCeil: 123}},
		}
		m := &wasm.Module{ID: sha256.Sum256(nil), IsHostModule: true} // Host module!
		err := e.addCompiledModuleToCache(m, cm)
		require.NoError(t, err)
		// Check the host module not cached.
		_, hit, err := tc.Get(m.ID)
		require.NoError(t, err)
		require.False(t, hit)
	})
	t.Run("add", func(t *testing.T) {
		tc := filecache.New(t.TempDir())
		e := engine{fileCache: tc}
		m := &wasm.Module{}
		cm := &compiledModule{
			compiledCode: &compiledCode{
				executable: makeCodeSegment(1, 2, 3),
			},
			functions: []compiledFunction{{stackPointerCeil: 123}},
		}
		err := e.addCompiledModuleToCache(m, cm)
		require.NoError(t, err)

		content, ok, err := tc.Get(m.ID)
		require.NoError(t, err)
		require.True(t, ok)
		actual, err := io.ReadAll(content)
		require.NoError(t, err)
		require.Equal(t, concat(
			[]byte(wazeroMagic),
			[]byte{byte(len(testVersion))},
			[]byte(testVersion),
			[]byte{0},
			u32.LeBytes(1),   // number of functions.
			u64.LeBytes(123), // stack pointer ceil.
			u64.LeBytes(0),   // offset.
			u64.LeBytes(3),   // size of executable.
			[]byte{1, 2, 3},
			crcf([]byte{1, 2, 3}), // code.
		), actual)
		require.NoError(t, content.Close())
	})
}

func Test_readUint64(t *testing.T) {
	tests := []struct {
		name  string
		input uint64
	}{
		{
			name:  "zero",
			input: 0,
		},
		{
			name:  "half",
			input: math.MaxUint32,
		},
		{
			name:  "max",
			input: math.MaxUint64,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			input := make([]byte, 8)
			binary.LittleEndian.PutUint64(input, tc.input)

			var b [8]byte
			n, err := readUint64(bytes.NewReader(input), &b)
			require.NoError(t, err)
			require.Equal(t, tc.input, n)

			// ensure the buffer was cleared
			var expectedB [8]byte
			require.Equal(t, expectedB, b)
		})
	}
}

func Test_readUint64_errors(t *testing.T) {
	tests := []struct {
		name        string
		input       io.Reader
		expectedErr string
	}{
		{
			name:        "zero",
			input:       bytes.NewReader([]byte{}),
			expectedErr: "EOF",
		},
		{
			name:        "not enough",
			input:       bytes.NewReader([]byte{1, 2}),
			expectedErr: "EOF",
		},
		{
			name:        "error reading",
			input:       iotest.ErrReader(errors.New("ice cream")),
			expectedErr: "ice cream",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			var b [8]byte
			_, err := readUint64(tc.input, &b)
			require.EqualError(t, err, tc.expectedErr)
		})
	}
}

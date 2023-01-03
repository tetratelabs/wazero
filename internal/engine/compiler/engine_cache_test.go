package compiler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"testing"
	"testing/iotest"

	"github.com/tetratelabs/wazero/internal/compilationcache"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/u32"
	"github.com/tetratelabs/wazero/internal/u64"
	"github.com/tetratelabs/wazero/internal/wasm"
)

var testVersion = ""

func concat(ins ...[]byte) (ret []byte) {
	for _, in := range ins {
		ret = append(ret, in...)
	}
	return
}

func TestSerializeCodes(t *testing.T) {
	tests := []struct {
		in  []*code
		exp []byte
	}{
		{
			in: []*code{{stackPointerCeil: 12345, codeSegment: []byte{1, 2, 3, 4, 5}}},
			exp: concat(
				[]byte(wazeroMagic),
				[]byte{byte(len(testVersion))},
				[]byte(testVersion),
				u32.LeBytes(1),        // number of functions.
				u64.LeBytes(12345),    // stack pointer ceil.
				u64.LeBytes(5),        // length of code.
				[]byte{1, 2, 3, 4, 5}, // code.
			),
		},
		{
			in: []*code{
				{stackPointerCeil: 12345, codeSegment: []byte{1, 2, 3, 4, 5}},
				{stackPointerCeil: 0xffffffff, codeSegment: []byte{1, 2, 3}},
			},
			exp: concat(
				[]byte(wazeroMagic),
				[]byte{byte(len(testVersion))},
				[]byte(testVersion),
				u32.LeBytes(2), // number of functions.
				// Function index = 0.
				u64.LeBytes(12345),    // stack pointer ceil.
				u64.LeBytes(5),        // length of code.
				[]byte{1, 2, 3, 4, 5}, // code.
				// Function index = 1.
				u64.LeBytes(0xffffffff), // stack pointer ceil.
				u64.LeBytes(3),          // length of code.
				[]byte{1, 2, 3},         // code.
			),
		},
	}

	for i, tc := range tests {
		actual, err := io.ReadAll(serializeCodes(testVersion, tc.in))
		require.NoError(t, err, i)
		require.Equal(t, tc.exp, actual, i)
	}
}

func TestDeserializeCodes(t *testing.T) {
	tests := []struct {
		name          string
		in            []byte
		expCodes      []*code
		expStaleCache bool
		expErr        string
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
			name: "one function",
			in: concat(
				[]byte(wazeroMagic),
				[]byte{byte(len(testVersion))},
				[]byte(testVersion),
				u32.LeBytes(1),        // number of functions.
				u64.LeBytes(12345),    // stack pointer ceil.
				u64.LeBytes(5),        // length of code.
				[]byte{1, 2, 3, 4, 5}, // code.
			),
			expCodes: []*code{
				{stackPointerCeil: 12345, codeSegment: []byte{1, 2, 3, 4, 5}},
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
				u32.LeBytes(2), // number of functions.
				// Function index = 0.
				u64.LeBytes(12345),    // stack pointer ceil.
				u64.LeBytes(5),        // length of code.
				[]byte{1, 2, 3, 4, 5}, // code.
				// Function index = 1.
				u64.LeBytes(0xffffffff), // stack pointer ceil.
				u64.LeBytes(3),          // length of code.
				[]byte{1, 2, 3},         // code.
			),
			expCodes: []*code{
				{stackPointerCeil: 12345, codeSegment: []byte{1, 2, 3, 4, 5}},
				{stackPointerCeil: 0xffffffff, codeSegment: []byte{1, 2, 3}},
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
				u32.LeBytes(2), // number of functions.
				// Function index = 0.
				u64.LeBytes(12345),    // stack pointer ceil.
				u64.LeBytes(5),        // length of code.
				[]byte{1, 2, 3, 4, 5}, // code.
				// Function index = 1.
			),
			expErr: "compilationcache: error reading func[1] stack pointer ceil: EOF",
		},
		{
			name: "reading native code size",
			in: concat(
				[]byte(wazeroMagic),
				[]byte{byte(len(testVersion))},
				[]byte(testVersion),
				u32.LeBytes(2), // number of functions.
				// Function index = 0.
				u64.LeBytes(12345),    // stack pointer ceil.
				u64.LeBytes(5),        // length of code.
				[]byte{1, 2, 3, 4, 5}, // code.
				// Function index = 1.
				u64.LeBytes(12345), // stack pointer ceil.
			),
			expErr: "compilationcache: error reading func[1] reading native code size: EOF",
		},
		{
			name: "mmapping",
			in: concat(
				[]byte(wazeroMagic),
				[]byte{byte(len(testVersion))},
				[]byte(testVersion),
				u32.LeBytes(2), // number of functions.
				// Function index = 0.
				u64.LeBytes(12345),    // stack pointer ceil.
				u64.LeBytes(5),        // length of code.
				[]byte{1, 2, 3, 4, 5}, // code.
				// Function index = 1.
				u64.LeBytes(12345), // stack pointer ceil.
				u64.LeBytes(5),     // length of code.
				// Lack of code here.
			),
			expErr: "compilationcache: error mmapping func[1] code (len=5): EOF",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			codes, staleCache, err := deserializeCodes(testVersion, io.NopCloser(bytes.NewReader(tc.in)))
			if tc.expErr != "" {
				require.EqualError(t, err, tc.expErr)
			} else {
				require.NoError(t, err)
			}

			require.Equal(t, tc.expCodes, codes)
			require.Equal(t, tc.expStaleCache, staleCache)
		})
	}
}

func TestEngine_getCodesFromCache(t *testing.T) {
	valid := concat(
		[]byte(wazeroMagic),
		[]byte{byte(len(testVersion))},
		[]byte(testVersion),
		u32.LeBytes(2), // number of functions.
		// Function index = 0.
		u64.LeBytes(12345),    // stack pointer ceil.
		u64.LeBytes(5),        // length of code.
		[]byte{1, 2, 3, 4, 5}, // code.
		// Function index = 1.
		u64.LeBytes(0xffffffff), // stack pointer ceil.
		u64.LeBytes(3),          // length of code.
		[]byte{1, 2, 3},         // code.
	)

	tests := []struct {
		name       string
		ext        map[wasm.ModuleID][]byte
		key        wasm.ModuleID
		isHostMod  bool
		expCodes   []*code
		expHit     bool
		expErr     string
		expDeleted bool
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
			expCodes: []*code{
				{stackPointerCeil: 12345, codeSegment: []byte{1, 2, 3, 4, 5}, indexInModule: 0},
				{stackPointerCeil: 0xffffffff, codeSegment: []byte{1, 2, 3}, indexInModule: 1},
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			m := &wasm.Module{ID: tc.key, IsHostModule: tc.isHostMod}
			for _, expC := range tc.expCodes {
				expC.sourceModule = m
			}

			e := engine{}
			if tc.ext != nil {
				tmp := t.TempDir()
				e.Cache = compilationcache.NewFileCache(
					context.WithValue(context.Background(), compilationcache.FileCachePathKey{}, tmp),
				)
				for key, value := range tc.ext {
					err := e.Cache.Add(key, bytes.NewReader(value))
					require.NoError(t, err)
				}
			}

			codes, hit, err := e.getCodesFromCache(m)
			if tc.expErr != "" {
				require.EqualError(t, err, tc.expErr)
			} else {
				require.NoError(t, err)
			}

			require.Equal(t, tc.expHit, hit)
			require.Equal(t, tc.expCodes, codes)

			if tc.ext != nil && tc.expDeleted {
				_, hit, err := e.Cache.Get(tc.key)
				require.NoError(t, err)
				require.False(t, hit)
			}
		})
	}
}

func TestEngine_addCodesToCache(t *testing.T) {
	t.Run("not defined", func(t *testing.T) {
		e := engine{}
		err := e.addCodesToCache(nil, nil)
		require.NoError(t, err)
	})
	t.Run("host module", func(t *testing.T) {
		tc := compilationcache.NewFileCache(context.WithValue(context.Background(),
			compilationcache.FileCachePathKey{}, t.TempDir()))
		e := engine{Cache: tc}
		codes := []*code{{stackPointerCeil: 123, codeSegment: []byte{1, 2, 3}}}
		m := &wasm.Module{ID: sha256.Sum256(nil), IsHostModule: true} // Host module!
		err := e.addCodesToCache(m, codes)
		require.NoError(t, err)
		// Check the host module not cached.
		_, hit, err := tc.Get(m.ID)
		require.NoError(t, err)
		require.False(t, hit)
	})
	t.Run("add", func(t *testing.T) {
		tc := compilationcache.NewFileCache(context.WithValue(context.Background(),
			compilationcache.FileCachePathKey{}, t.TempDir()))
		e := engine{Cache: tc}
		m := &wasm.Module{}
		codes := []*code{{stackPointerCeil: 123, codeSegment: []byte{1, 2, 3}}}
		err := e.addCodesToCache(m, codes)
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
			u32.LeBytes(1),   // number of functions.
			u64.LeBytes(123), // stack pointer ceil.
			u64.LeBytes(3),   // length of code.
			[]byte{1, 2, 3},  // code.
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

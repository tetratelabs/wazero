package compiler

import (
	"bytes"
	"fmt"
	"io"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/u32"
	"github.com/tetratelabs/wazero/internal/u64"
	"github.com/tetratelabs/wazero/internal/wasm"
)

var testVersion string

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
			expErr: "invalid header length: 1",
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
			expErr: "reading stack pointer ceil: EOF",
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
			expErr: "reading native code size: EOF",
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
			expErr: "mmaping function: EOF",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			codes, staleCache, err := deserializeCodes(testVersion, bytes.NewReader(tc.in))
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
	tests := []struct {
		name       string
		ext        *testCache
		key        wasm.ModuleID
		expCodes   []*code
		expHit     bool
		expErr     string
		expDeleted bool
	}{
		{name: "extern cache not given"},
		{
			name: "not hit",
			ext:  &testCache{caches: map[wasm.ModuleID][]byte{}},
		},
		{
			name:   "error in Cache.Get",
			ext:    &testCache{caches: map[wasm.ModuleID][]byte{{}: {}}},
			expErr: "some error from extern cache",
		},
		{
			name:   "error in deserialization",
			ext:    &testCache{caches: map[wasm.ModuleID][]byte{{}: {1, 2, 3}}},
			expErr: "invalid header length: 3",
		},
		{
			name: "stale cache",
			ext: &testCache{caches: map[wasm.ModuleID][]byte{{}: concat(
				[]byte(wazeroMagic),
				[]byte{byte(len("1233123.1.1"))},
				[]byte("1233123.1.1"),
				u32.LeBytes(1), // number of functions.
			)}},
			expDeleted: true,
		},
		{
			name: "hit",
			ext: &testCache{caches: map[wasm.ModuleID][]byte{
				{}: concat(
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
			}},
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
			m := &wasm.Module{ID: tc.key}
			for _, expC := range tc.expCodes {
				expC.sourceModule = m
			}

			e := engine{}
			if tc.ext != nil {
				e.Cache = tc.ext
			}

			codes, hit, err := e.getCodesFromCache(m)
			if tc.expErr != "" {
				require.EqualError(t, err, tc.expErr)
			} else {
				require.NoError(t, err)
			}

			require.Equal(t, tc.expHit, hit)
			require.Equal(t, tc.expCodes, codes)

			if tc.expDeleted {
				require.Equal(t, tc.ext.deleted, tc.key)
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
	t.Run("add", func(t *testing.T) {
		ext := &testCache{caches: map[wasm.ModuleID][]byte{}}
		e := engine{Cache: ext}
		m := &wasm.Module{}
		codes := []*code{{stackPointerCeil: 123, codeSegment: []byte{1, 2, 3}}}
		err := e.addCodesToCache(m, codes)
		require.NoError(t, err)

		content, ok := ext.caches[m.ID]
		require.True(t, ok)
		require.Equal(t, concat(
			[]byte(wazeroMagic),
			[]byte{byte(len(testVersion))},
			[]byte(testVersion),
			u32.LeBytes(1),   // number of functions.
			u64.LeBytes(123), // stack pointer ceil.
			u64.LeBytes(3),   // length of code.
			[]byte{1, 2, 3},  // code.
		), content)
	})
}

// testCache implements compilationcache.Cache
type testCache struct {
	caches  map[wasm.ModuleID][]byte
	deleted wasm.ModuleID
}

// Get implements compilationcache.Cache Get
func (tc *testCache) Get(key wasm.ModuleID) (content io.ReadCloser, ok bool, err error) {
	var raw []byte
	raw, ok = tc.caches[key]
	if !ok {
		return
	}

	if len(raw) == 0 {
		ok = false
		err = fmt.Errorf("some error from extern cache")
		return
	}

	content = io.NopCloser(bytes.NewReader(raw))
	return
}

// Add implements compilationcache.Cache Add
func (tc *testCache) Add(key wasm.ModuleID, content io.Reader) (err error) {
	raw, err := io.ReadAll(content)
	if err != nil {
		return err
	}
	tc.caches[key] = raw
	return
}

// Delete implements compilationcache.Cache Delete
func (tc *testCache) Delete(key wasm.ModuleID) (err error) {
	tc.deleted = key
	return
}

package assemblyscript

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"io"
	"testing"
	"testing/iotest"
	"time"
	"unicode/utf16"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

// abortWasm compiled from testdata/abort.ts
//go:embed testdata/abort.wasm
var abortWasm []byte

// randomWasm compiled from testdata/random.ts
//go:embed testdata/random.wasm
var randomWasm []byte

// traceWasm compiled from testdata/trace.ts
//go:embed testdata/trace.wasm
var traceWasm []byte

var testCtx = context.Background()

func TestAbort(t *testing.T) {
	r := wazero.NewRuntime()
	defer r.Close(testCtx)

	out := &bytes.Buffer{}

	_, err := NewModuleBuilder().
		WithAbortWriter(out).
		Instantiate(testCtx, r)
	require.NoError(t, err)

	mod, err := r.InstantiateModuleFromCode(testCtx, abortWasm)
	require.NoError(t, err)

	run := mod.ExportedFunction("run")
	_, err = run.Call(testCtx)
	require.Error(t, err)

	require.Equal(t, "error thrown at abort.ts:4:5\n", out.String())
}

func TestRandom(t *testing.T) {
	r := wazero.NewRuntime()
	defer r.Close(testCtx)

	seed := []byte{0, 1, 2, 3, 4, 5, 6, 7}

	_, err := NewModuleBuilder().
		WithSeedSource(bytes.NewReader(seed)).
		Instantiate(testCtx, r)
	require.NoError(t, err)

	mod, err := r.InstantiateModuleFromCode(testCtx, randomWasm)
	require.NoError(t, err)

	rand := mod.ExportedFunction("rand")

	var nums []float64

	for i := 0; i < 3; i++ {
		res, err := rand.Call(testCtx)
		require.NoError(t, err)
		nums = append(nums, api.DecodeF64(res[0]))
	}

	// Fixed seed so output is deterministic.
	require.Equal(t, 0.3730638488484388, nums[0])
	require.Equal(t, 0.4447017452657911, nums[1])
	require.Equal(t, 0.30214158564115867, nums[2])
}

func TestTrace(t *testing.T) {
	r := wazero.NewRuntime()
	defer r.Close(testCtx)

	out := &bytes.Buffer{}

	_, err := NewModuleBuilder().
		WithTraceWriter(out).
		Instantiate(testCtx, r)
	require.NoError(t, err)

	// _start will execute the calls to trace
	_, err = r.InstantiateModuleFromCode(testCtx, traceWasm)
	require.NoError(t, err)

	require.Equal(t, `trace: zero_implicit
trace: zero_explicit
trace: one_int 1
trace: two_int 1,2
trace: three_int 1,2,3
trace: four_int 1,2,3,4
trace: five_int 1,2,3,4,5
trace: five_dbl 1.1,2.2,3.3,4.4,5.5
`, out.String())
}

func TestReadAssemblyScriptString(t *testing.T) {
	for _, tc := range []struct {
		name     string
		memory   func(memory api.Memory)
		pointer  int
		expected string
		err      error
	}{
		{
			name: "success",
			memory: func(memory api.Memory) {
				memory.WriteUint32Le(testCtx, 0, 10)
				b := encodeUTF16("hello")
				memory.Write(testCtx, 4, b)
			},
			pointer:  4,
			expected: "hello",
		},
		{
			name: "can't read size",
			memory: func(memory api.Memory) {
				b := encodeUTF16("hello")
				memory.Write(testCtx, 0, b)
			},
			pointer: 0, // will attempt to read size from offset -4
			err:     fmt.Errorf("could not read size from memory"),
		},
		{
			name: "odd size",
			memory: func(memory api.Memory) {
				memory.WriteUint32Le(testCtx, 0, 9)
				b := encodeUTF16("hello")
				memory.Write(testCtx, 4, b)
			},
			pointer: 4,
			err:     fmt.Errorf("odd number of bytes for utf-16 string"),
		},
		{
			name: "can't read string",
			memory: func(memory api.Memory) {
				memory.WriteUint32Le(testCtx, 0, 10_000_000) // set size to too large value
				b := encodeUTF16("hello")
				memory.Write(testCtx, 4, b)
			},
			pointer: 4,
			err:     fmt.Errorf("could not read string from memory"),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := wazero.NewRuntime()
			defer r.Close(testCtx)

			mod, err := r.NewModuleBuilder("mod").
				ExportMemory("memory", 1).
				Instantiate(testCtx)
			require.NoError(t, err)

			tc.memory(mod.Memory())

			s, err := readAssemblyScriptString(testCtx, mod, uint32(tc.pointer))
			if tc.err != nil {
				require.EqualError(t, err, tc.err.Error())
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expected, s)
			}
		})
	}
}

func TestAbort_error(t *testing.T) {
	for _, tc := range []struct {
		name          string
		messageUTF16  []byte
		fileNameUTF16 []byte
	}{
		{
			name:          "bad message",
			messageUTF16:  encodeUTF16("message")[:5],
			fileNameUTF16: encodeUTF16("filename"),
		},
		{
			name:          "bad filename",
			messageUTF16:  encodeUTF16("message"),
			fileNameUTF16: encodeUTF16("filename")[:5],
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := wazero.NewRuntime()
			defer r.Close(testCtx)

			mod, err := r.NewModuleBuilder("mod").
				ExportMemory("memory", 1).
				Instantiate(testCtx)
			require.NoError(t, err)

			off := 0
			ok := mod.Memory().WriteUint32Le(testCtx, uint32(off), uint32(len(tc.messageUTF16)))
			require.True(t, ok)
			off += 4
			messageOff := off
			ok = mod.Memory().Write(testCtx, uint32(off), tc.messageUTF16)
			require.True(t, ok)
			off += len(tc.messageUTF16)
			ok = mod.Memory().WriteUint32Le(testCtx, uint32(off), uint32(len(tc.fileNameUTF16)))
			require.True(t, ok)
			off += 4
			filenameOff := off
			ok = mod.Memory().Write(testCtx, uint32(off), tc.fileNameUTF16)
			require.True(t, ok)

			out := &bytes.Buffer{}
			abort(testCtx, mod, uint32(messageOff), uint32(filenameOff), 1, 2, out)
			require.Equal(t, "", out.String()) // nothing outputed if strings fail to read.
		})
	}
}

func TestSeed_error(t *testing.T) {
	for _, tc := range []struct {
		name   string
		source io.Reader
	}{
		{
			name:   "not 8 bytes",
			source: bytes.NewReader([]byte{0, 1}),
		},
		{
			name:   "error reading",
			source: iotest.ErrReader(fmt.Errorf("error")),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cur := float64(time.Now().UnixMilli())

			val := seed(tc.source)

			require.True(t, val >= cur) // in error case, seed returns current time so this is always true
		})
	}
}

func TestTrace_error(t *testing.T) {
	r := wazero.NewRuntime()
	defer r.Close(testCtx)

	mod, err := r.NewModuleBuilder("mod").
		ExportMemory("memory", 1).
		Instantiate(testCtx)
	require.NoError(t, err)

	badMessage := encodeUTF16("hello")[:5]
	ok := mod.Memory().WriteUint32Le(testCtx, 0, uint32(len(badMessage)))
	require.True(t, ok)
	ok = mod.Memory().Write(testCtx, uint32(4), badMessage)
	require.True(t, ok)

	out := &bytes.Buffer{}
	trace(testCtx, mod, 4, 0, 0, 0, 0, 0, 0, out)
	require.Equal(t, "", out.String()) // nothing outputed if strings fail to read.
}

func encodeUTF16(s string) []byte {
	runes := utf16.Encode([]rune(s))
	b := make([]byte, len(runes)*2)
	for i, r := range runes {
		b[i*2] = byte(r)
		b[i*2+1] = byte(r >> 8)
	}
	return b
}

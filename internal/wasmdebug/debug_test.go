package wasmdebug

import (
	"errors"
	"runtime"
	"testing"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasmruntime"
)

func TestFuncName(t *testing.T) {
	tests := []struct {
		name, moduleName, funcName string
		funcIdx                    uint32
		expected                   string
	}{ // Only tests a few edge cases to show what it might end up as.
		{name: "empty", expected: ".$0"},
		{name: "empty module", funcName: "y", expected: ".y"},
		{name: "empty function", moduleName: "x", funcIdx: 255, expected: "x.$255"},
		{name: "looks like index in function", moduleName: "x", funcName: "[255]", expected: "x.[255]"},
		{name: "no special characters", moduleName: "x", funcName: "y", expected: "x.y"},
		{name: "dots in module", moduleName: "w.x", funcName: "y", expected: "w.x.y"},
		{name: "dots in function", moduleName: "x", funcName: "y.z", expected: "x.y.z"},
		{name: "spaces in module", moduleName: "w x", funcName: "y", expected: "w x.y"},
		{name: "spaces in function", moduleName: "x", funcName: "y z", expected: "x.y z"},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			funcName := FuncName(tc.moduleName, tc.funcName, tc.funcIdx)
			require.Equal(t, tc.expected, funcName)
		})
	}
}

func TestAddSignature(t *testing.T) {
	i32, i64, f32, f64 := api.ValueTypeI32, api.ValueTypeI64, api.ValueTypeF32, api.ValueTypeF64
	tests := []struct {
		name                    string
		paramTypes, resultTypes []api.ValueType
		expected                string
	}{
		{name: "v_v", expected: "x.y()"},
		{name: "i32_v", paramTypes: []api.ValueType{i32}, expected: "x.y(i32)"},
		{name: "i32f64_v", paramTypes: []api.ValueType{i32, f64}, expected: "x.y(i32,f64)"},
		{name: "f32i32f64_v", paramTypes: []api.ValueType{f32, i32, f64}, expected: "x.y(f32,i32,f64)"},
		{name: "v_i64", resultTypes: []api.ValueType{i64}, expected: "x.y() i64"},
		{name: "v_i64f32", resultTypes: []api.ValueType{i64, f32}, expected: "x.y() (i64,f32)"},
		{name: "v_f32i32f64", resultTypes: []api.ValueType{f32, i32, f64}, expected: "x.y() (f32,i32,f64)"},
		{name: "i32_i64", paramTypes: []api.ValueType{i32}, resultTypes: []api.ValueType{i64}, expected: "x.y(i32) i64"},
		{name: "i64f32_i64f32", paramTypes: []api.ValueType{i64, f32}, resultTypes: []api.ValueType{i64, f32}, expected: "x.y(i64,f32) (i64,f32)"},
		{name: "i64f32f64_f32i32f64", paramTypes: []api.ValueType{i64, f32, f64}, resultTypes: []api.ValueType{f32, i32, f64}, expected: "x.y(i64,f32,f64) (f32,i32,f64)"},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			withSignature := signature("x.y", tc.paramTypes, tc.resultTypes)
			require.Equal(t, tc.expected, withSignature)
		})
	}
}

var (
	argErr       = errors.New("invalid argument")
	rteErr       = testRuntimeErr("index out of bounds")
	i32          = api.ValueTypeI32
	i32i32i32i32 = []api.ValueType{i32, i32, i32, i32}
)

func TestErrorBuilder(t *testing.T) {
	tests := []struct {
		name         string
		build        func(ErrorBuilder) error
		expectedErr  string
		expectUnwrap error
	}{
		{
			name: "one",
			build: func(builder ErrorBuilder) error {
				builder.AddFrame("x.y", nil, nil, nil)
				return builder.FromRecovered(argErr)
			},
			expectedErr: `invalid argument (recovered by wazero)
wasm stack trace:
	x.y()`,
			expectUnwrap: argErr,
		},
		{
			name: "two",
			build: func(builder ErrorBuilder) error {
				builder.AddFrame("wasi_snapshot_preview1.fd_write", i32i32i32i32, []api.ValueType{i32}, nil)
				builder.AddFrame("x.y", nil, nil, nil)
				return builder.FromRecovered(argErr)
			},
			expectedErr: `invalid argument (recovered by wazero)
wasm stack trace:
	wasi_snapshot_preview1.fd_write(i32,i32,i32,i32) i32
	x.y()`,
			expectUnwrap: argErr,
		},
		{
			name: "wasmruntime.Error",
			build: func(builder ErrorBuilder) error {
				builder.AddFrame("wasi_snapshot_preview1.fd_write", i32i32i32i32, []api.ValueType{i32},
					[]string{"/opt/homebrew/Cellar/tinygo/0.26.0/src/runtime/runtime_tinygowasm.go:73:6"})
				builder.AddFrame("x.y", nil, nil, nil)
				return builder.FromRecovered(wasmruntime.ErrRuntimeStackOverflow)
			},
			expectedErr: `wasm error: stack overflow
wasm stack trace:
	wasi_snapshot_preview1.fd_write(i32,i32,i32,i32) i32
		/opt/homebrew/Cellar/tinygo/0.26.0/src/runtime/runtime_tinygowasm.go:73:6
	x.y()`,
			expectUnwrap: wasmruntime.ErrRuntimeStackOverflow,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			withStackTrace := tc.build(NewErrorBuilder())
			require.Equal(t, tc.expectUnwrap, errors.Unwrap(withStackTrace))
			require.EqualError(t, withStackTrace, tc.expectedErr)
		})
	}
}

func TestErrorBuilderGoRuntimeError(t *testing.T) {
	builder := NewErrorBuilder()
	builder.AddFrame("wasi_snapshot_preview1.fd_write", i32i32i32i32, []api.ValueType{i32}, nil)
	builder.AddFrame("x.y", nil, nil, nil)
	withStackTrace := builder.FromRecovered(rteErr)

	require.Equal(t, rteErr, errors.Unwrap(withStackTrace))

	errStr := withStackTrace.Error()
	require.Contains(t, errStr, `index out of bounds (recovered by wazero)
wasm stack trace:
	wasi_snapshot_preview1.fd_write(i32,i32,i32,i32) i32
	x.y()`)
	require.Contains(t, errStr, "Go runtime stack trace:")
	require.Contains(t, errStr, "goroutine ")
	require.Contains(t, errStr, "/internal/wasmdebug/debug_test.go")
}

// compile-time check to ensure testRuntimeErr implements runtime.Error.
var _ runtime.Error = testRuntimeErr("")

type testRuntimeErr string

func (e testRuntimeErr) RuntimeError() {}

func (e testRuntimeErr) Error() string {
	return string(e)
}

func Test_AddFrame_MaxFrame(t *testing.T) {
	builder := NewErrorBuilder().(*stackTrace)
	for i := 0; i < MaxFrames+10; i++ {
		builder.AddFrame("x.y", nil, nil, []string{"a.go:1:2", "b.go:3:4"})
	}
	require.Equal(t, MaxFrames, builder.frameCount)
	require.Equal(t, MaxFrames*3 /* frame + two inlined sources */ +1, len(builder.lines))
	require.Equal(t, "... maybe followed by omitted frames", builder.lines[len(builder.lines)-1])
}

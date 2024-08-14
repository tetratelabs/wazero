package assemblyscript

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"testing"
	"testing/iotest"
	"unicode/utf16"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	. "github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/experimental/logging"
	"github.com/tetratelabs/wazero/experimental/wazerotest"
	. "github.com/tetratelabs/wazero/internal/assemblyscript"
	"github.com/tetratelabs/wazero/internal/testing/proxy"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/u64"
	"github.com/tetratelabs/wazero/sys"
)

type arbitrary struct{}

// testCtx is an arbitrary, non-default context. Non-nil also prevents linter errors.
var testCtx = context.WithValue(context.Background(), arbitrary{}, "arbitrary")

func TestAbort(t *testing.T) {
	tests := []struct {
		name     string
		exporter FunctionExporter
		expected string
	}{
		{
			name:     "enabled",
			exporter: NewFunctionExporter(),
			expected: "message at filename:1:2\n",
		},
		{
			name:     "disabled",
			exporter: NewFunctionExporter().WithAbortMessageDisabled(),
			expected: "",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			var stderr bytes.Buffer
			mod, r, log := requireProxyModule(t, tc.exporter, wazero.NewModuleConfig().WithStderr(&stderr), logging.LogScopeProc)
			defer r.Close(testCtx)

			messageOff, filenameOff := writeAbortMessageAndFileName(t, mod.Memory(), encodeUTF16("message"), encodeUTF16("filename"))

			_, err := mod.ExportedFunction(AbortName).
				Call(testCtx, uint64(messageOff), uint64(filenameOff), uint64(1), uint64(2))
			require.Error(t, err)
			sysErr, ok := err.(*sys.ExitError)
			require.True(t, ok, err)
			require.Equal(t, uint32(255), sysErr.ExitCode())
			require.Equal(t, `
==> env.~lib/builtins/abort(message=4,fileName=22,lineNumber=1,columnNumber=2)
`, "\n"+log.String())

			require.Equal(t, tc.expected, stderr.String())
		})
	}
}

func TestAbort_Error(t *testing.T) {
	tests := []struct {
		name          string
		messageUTF16  []byte
		fileNameUTF16 []byte
		expectedLog   string
	}{
		{
			name:          "bad message",
			messageUTF16:  encodeUTF16("message")[:5],
			fileNameUTF16: encodeUTF16("filename"),
			expectedLog: `
==> env.~lib/builtins/abort(message=4,fileName=13,lineNumber=1,columnNumber=2)
`,
		},
		{
			name:          "bad filename",
			messageUTF16:  encodeUTF16("message"),
			fileNameUTF16: encodeUTF16("filename")[:5],
			expectedLog: `
==> env.~lib/builtins/abort(message=4,fileName=22,lineNumber=1,columnNumber=2)
`,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			var stderr bytes.Buffer
			mod, r, log := requireProxyModule(t, NewFunctionExporter(), wazero.NewModuleConfig().WithStderr(&stderr), logging.LogScopeAll)
			defer r.Close(testCtx)

			messageOff, filenameOff := writeAbortMessageAndFileName(t, mod.Memory(), tc.messageUTF16, tc.fileNameUTF16)

			_, err := mod.ExportedFunction(AbortName).
				Call(testCtx, uint64(messageOff), uint64(filenameOff), uint64(1), uint64(2))
			require.Error(t, err)
			sysErr, ok := err.(*sys.ExitError)
			require.True(t, ok, err)
			require.Equal(t, uint32(255), sysErr.ExitCode())
			require.Equal(t, tc.expectedLog, "\n"+log.String())

			require.Equal(t, "", stderr.String()) // nothing output if strings fail to read.
		})
	}
}

func TestSeed(t *testing.T) {
	tests := []struct {
		name        string
		scopes      logging.LogScopes
		expectedLog string
	}{
		{
			name:   "logs to crypto scope",
			scopes: logging.LogScopeRandom,
			expectedLog: `
==> env.~lib/builtins/seed()
<== rand=4.958153677776298e-175
`,
		},
		{
			name:        "doesn't log to filesystem scope",
			scopes:      logging.LogScopeFilesystem,
			expectedLog: "\n",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			mod, r, log := requireProxyModule(t, NewFunctionExporter(), wazero.NewModuleConfig(), tc.scopes)
			defer r.Close(testCtx)

			ret, err := mod.ExportedFunction(SeedName).Call(testCtx)
			require.NoError(t, err)
			require.Equal(t, tc.expectedLog, "\n"+log.String())

			require.Equal(t, "538c7f96b164bf1b", hex.EncodeToString(u64.LeBytes(ret[0])))
		})
	}
}

func TestSeed_error(t *testing.T) {
	tests := []struct {
		name        string
		source      io.Reader
		expectedErr string
	}{
		{
			name:   "not 8 bytes",
			source: bytes.NewReader([]byte{0, 1}),
			expectedErr: `error reading random seed: unexpected EOF (recovered by wazero)
wasm stack trace:
	env.~lib/builtins/seed() f64
	internal/testing/proxy/proxy.go.seed() f64`,
		},
		{
			name:   "error reading",
			source: iotest.ErrReader(errors.New("ice cream")),
			expectedErr: `error reading random seed: ice cream (recovered by wazero)
wasm stack trace:
	env.~lib/builtins/seed() f64
	internal/testing/proxy/proxy.go.seed() f64`,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			mod, r, log := requireProxyModule(t, NewFunctionExporter(), wazero.NewModuleConfig().WithRandSource(tc.source), logging.LogScopeAll)
			defer r.Close(testCtx)

			_, err := mod.ExportedFunction(SeedName).Call(testCtx)
			require.EqualError(t, err, tc.expectedErr)
			require.Equal(t, `
==> env.~lib/builtins/seed()
`, "\n"+log.String())
		})
	}
}

// TestFunctionExporter_Trace ensures the trace output is according to configuration.
func TestFunctionExporter_Trace(t *testing.T) {
	noArgs := []uint64{4, 0, 0, 0, 0, 0, 0}
	noArgsLog := `
==> env.~lib/builtins/trace(message=4,nArgs=0,arg0=0,arg1=0,arg2=0,arg3=0,arg4=0)
<==
`

	tests := []struct {
		name                  string
		exporter              FunctionExporter
		params                []uint64
		message               []byte
		outErr                bool
		expected, expectedLog string
	}{
		{
			name:        "disabled",
			exporter:    NewFunctionExporter(),
			params:      noArgs,
			expected:    "",
			expectedLog: noArgsLog,
		},
		{
			name:        "ToStderr",
			exporter:    NewFunctionExporter().WithTraceToStderr(),
			params:      noArgs,
			expected:    "trace: hello\n",
			expectedLog: noArgsLog,
		},
		{
			name:        "ToStdout - no args",
			exporter:    NewFunctionExporter().WithTraceToStdout(),
			params:      noArgs,
			expected:    "trace: hello\n",
			expectedLog: noArgsLog,
		},
		{
			name:     "ToStdout - one arg",
			exporter: NewFunctionExporter().WithTraceToStdout(),
			params:   []uint64{4, 1, api.EncodeF64(1), 0, 0, 0, 0},
			expected: "trace: hello 1\n",
			expectedLog: `
==> env.~lib/builtins/trace(message=4,nArgs=1,arg0=1,arg1=0,arg2=0,arg3=0,arg4=0)
<==
`,
		},
		{
			name:     "ToStdout - two args",
			exporter: NewFunctionExporter().WithTraceToStdout(),
			params:   []uint64{4, 2, api.EncodeF64(1), api.EncodeF64(2), 0, 0, 0},
			expected: "trace: hello 1,2\n",
			expectedLog: `
==> env.~lib/builtins/trace(message=4,nArgs=2,arg0=1,arg1=2,arg2=0,arg3=0,arg4=0)
<==
`,
		},
		{
			name:     "ToStdout - five args",
			exporter: NewFunctionExporter().WithTraceToStdout(),
			params: []uint64{
				4,
				5,
				api.EncodeF64(1),
				api.EncodeF64(2),
				api.EncodeF64(3.3),
				api.EncodeF64(4.4),
				api.EncodeF64(5),
			},
			expected: "trace: hello 1,2,3.3,4.4,5\n",
			expectedLog: `
==> env.~lib/builtins/trace(message=4,nArgs=5,arg0=1,arg1=2,arg2=3.3,arg3=4.4,arg4=5)
<==
`,
		},
		{
			name:        "not 8 bytes",
			exporter:    NewFunctionExporter().WithTraceToStderr(),
			message:     encodeUTF16("hello")[:5],
			params:      noArgs,
			expectedLog: noArgsLog,
		},
		{
			name:        "error writing",
			exporter:    NewFunctionExporter().WithTraceToStderr(),
			outErr:      true,
			params:      noArgs,
			expectedLog: noArgsLog,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer

			config := wazero.NewModuleConfig()
			if strings.Contains("ToStderr", tc.name) {
				config = config.WithStderr(&out)
			} else {
				config = config.WithStdout(&out)
			}
			if tc.outErr {
				config = config.WithStderr(&errWriter{err: errors.New("ice cream")})
			}

			mod, r, log := requireProxyModule(t, tc.exporter, config, logging.LogScopeAll)
			defer r.Close(testCtx)

			message := tc.message
			if message == nil {
				message = encodeUTF16("hello")
			}
			ok := mod.Memory().WriteUint32Le(0, uint32(len(message)))
			require.True(t, ok)
			ok = mod.Memory().Write(uint32(4), message)
			require.True(t, ok)

			_, err := mod.ExportedFunction(TraceName).Call(testCtx, tc.params...)
			require.NoError(t, err)
			require.Equal(t, tc.expected, out.String())
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}

func Test_readAssemblyScriptString(t *testing.T) {
	tests := []struct {
		name       string
		memory     func(api.Memory)
		offset     int
		expected   string
		expectedOk bool
	}{
		{
			name: "success",
			memory: func(memory api.Memory) {
				memory.WriteUint32Le(0, 10)
				b := encodeUTF16("hello")
				memory.Write(4, b)
			},
			offset:     4,
			expected:   "hello",
			expectedOk: true,
		},
		{
			name: "can't read size",
			memory: func(memory api.Memory) {
				b := encodeUTF16("hello")
				memory.Write(0, b)
			},
			offset:     0, // will attempt to read size from offset -4
			expectedOk: false,
		},
		{
			name: "odd size",
			memory: func(memory api.Memory) {
				memory.WriteUint32Le(0, 9)
				b := encodeUTF16("hello")
				memory.Write(4, b)
			},
			offset:     4,
			expectedOk: false,
		},
		{
			name: "can't read string",
			memory: func(memory api.Memory) {
				memory.WriteUint32Le(0, 10_000_000) // set size to too large value
				b := encodeUTF16("hello")
				memory.Write(4, b)
			},
			offset:     4,
			expectedOk: false,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			mem := wazerotest.NewFixedMemory(wazerotest.PageSize)
			tc.memory(mem)

			s, ok := readAssemblyScriptString(mem, uint32(tc.offset))
			require.Equal(t, tc.expectedOk, ok)
			require.Equal(t, tc.expected, s)
		})
	}
}

func writeAbortMessageAndFileName(t *testing.T, mem api.Memory, messageUTF16, fileNameUTF16 []byte) (uint32, uint32) {
	off := uint32(0)
	ok := mem.WriteUint32Le(off, uint32(len(messageUTF16)))
	require.True(t, ok)
	off += 4
	messageOff := off
	ok = mem.Write(off, messageUTF16)
	require.True(t, ok)
	off += uint32(len(messageUTF16))
	ok = mem.WriteUint32Le(off, uint32(len(fileNameUTF16)))
	require.True(t, ok)
	off += 4
	filenameOff := off
	ok = mem.Write(off, fileNameUTF16)
	require.True(t, ok)
	return messageOff, filenameOff
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

type errWriter struct {
	err error
}

func (w *errWriter) Write([]byte) (int, error) {
	return 0, w.err
}

func requireProxyModule(t *testing.T, fns FunctionExporter, config wazero.ModuleConfig, scopes logging.LogScopes) (api.Module, api.Closer, *bytes.Buffer) {
	var log bytes.Buffer

	// Set context to one that has an experimental listener
	ctx := WithFunctionListenerFactory(testCtx,
		proxy.NewLoggingListenerFactory(&log, scopes))

	r := wazero.NewRuntime(ctx)

	builder := r.NewHostModuleBuilder("env")
	fns.ExportFunctions(builder)

	envCompiled, err := builder.Compile(ctx)
	require.NoError(t, err)

	_, err = r.InstantiateModule(ctx, envCompiled, config)
	require.NoError(t, err)

	proxyBin := proxy.NewModuleBinary("env", envCompiled)

	proxyCompiled, err := r.CompileModule(ctx, proxyBin)
	require.NoError(t, err)

	mod, err := r.InstantiateModule(ctx, proxyCompiled, config)
	require.NoError(t, err)
	return mod, r, &log
}

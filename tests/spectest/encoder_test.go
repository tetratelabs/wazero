package spectests

import (
	"strings"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wasm/binary"
)

func TestBinaryEncoder(t *testing.T) {
	files, err := testcases.ReadDir("testdata")
	require.NoError(t, err)

	for _, f := range files {
		filename := f.Name()
		if strings.HasSuffix(filename, ".wasm") {
			t.Run(filename, func(t *testing.T) {
				buf, err := testcases.ReadFile(testdataPath(filename))
				require.NoError(t, err)

				mod, err := binary.DecodeModule(buf, wasm.Features20191205, wasm.MemoryMaxPages)
				require.NoError(t, err)

				encodedBuf := binary.EncodeModule(mod)
				require.Equal(t, buf, encodedBuf)
			})
		}
	}
}

package spectests

import (
	"encoding/json"
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
		if strings.HasSuffix(filename, ".json") {
			raw, err := testcases.ReadFile(testdataPath(filename))
			require.NoError(t, err)

			var base testbase
			require.NoError(t, json.Unmarshal(raw, &base))

			for _, c := range base.Commands {
				if c.CommandType == "module" {
					t.Run(filename, func(t *testing.T) {
						buf, err := testcases.ReadFile(testdataPath(c.Filename))
						require.NoError(t, err)

						mod, err := binary.DecodeModule(buf, wasm.Features20191205, wasm.MemoryMaxPages)
						require.NoError(t, err)

						encodedBuf := binary.EncodeModule(mod)
						require.Equal(t, buf, encodedBuf)
					})
				}
			}
		}
	}
}

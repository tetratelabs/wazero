package spectests

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/heeus/hwazero/internal/leb128"
	"github.com/heeus/hwazero/internal/testing/require"
	"github.com/heeus/hwazero/internal/wasm"
	"github.com/heeus/hwazero/internal/wasm/binary"
)

// requireStripCustomSections strips all the custom sections from the given binary.
func requireStripCustomSections(t *testing.T, binary []byte) []byte {
	r := bytes.NewReader(binary)
	out := bytes.NewBuffer(nil)
	_, err := io.CopyN(out, r, 8)
	require.NoError(t, err)

	for {
		sectionID, err := r.ReadByte()
		if err == io.EOF {
			break
		} else if err != nil {
			require.NoError(t, err)
		}

		sectionSize, _, err := leb128.DecodeUint32(r)
		require.NoError(t, err)

		switch sectionID {
		case wasm.SectionIDCustom:
			_, err = io.CopyN(io.Discard, r, int64(sectionSize))
			require.NoError(t, err)
		default:
			out.WriteByte(sectionID)
			out.Write(leb128.EncodeUint32(sectionSize))
			_, err := io.CopyN(out, r, int64(sectionSize))
			require.NoError(t, err)
		}
	}
	return out.Bytes()
}

// TestBinaryEncoder ensures that binary.EncodeModule produces exactly the same binaries
// for wasm.Module via binary.DecodeModule modulo custom sections for all the valid binaries in spectests.
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
					t.Run(c.Filename, func(t *testing.T) {
						buf, err := testcases.ReadFile(testdataPath(c.Filename))
						require.NoError(t, err)

						buf = requireStripCustomSections(t, buf)

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

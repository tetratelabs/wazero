package spectests

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/tetratelabs/wazero/internal/leb128"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wasm/binary"
)

// stripCustomSections strips all the custom sections from the given binary.
func stripCustomSections(binary []byte) ([]byte, error) {
	r := bytes.NewReader(binary)
	out := bytes.NewBuffer(nil)
	io.CopyN(out, r, 8)

	for {
		sectionID, err := r.ReadByte()
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, fmt.Errorf("read section id: %w", err)
		}

		sectionSize, _, err := leb128.DecodeUint32(r)
		if err != nil {
			return nil, fmt.Errorf("get size of section %s: %v", wasm.SectionIDName(sectionID), err)
		}

		switch sectionID {
		case wasm.SectionIDCustom:
			if _, err = io.CopyN(io.Discard, r, int64(sectionSize)); err != nil {
				return nil, errors.New("failed to ignore custom section")
			}
		default:
			out.WriteByte(sectionID)
			out.Write(leb128.EncodeUint32(sectionSize))
			if _, err := io.CopyN(out, r, int64(sectionSize)); err != nil {
				return nil, fmt.Errorf("failed to copy %s section", wasm.SectionIDName(sectionID))
			}
		}
	}
	return out.Bytes(), nil
}

// TestBinaryEncoder ensures that binary.Encoder produces exactly the same binaries
// after ecoding them module custom sections for all the valid binaries in spectests.
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

						buf, err = stripCustomSections(buf)
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

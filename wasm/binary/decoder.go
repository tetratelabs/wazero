package binary

import (
	"bytes"
	"fmt"
	"io"

	"github.com/tetratelabs/wazero/wasm"
	"github.com/tetratelabs/wazero/wasm/internal/leb128"
)

// DecodeModule implements wasm.DecodeModule for the WebAssembly 1.0 (MVP) Binary Format
// See https://www.w3.org/TR/wasm-core-1/#binary-format%E2%91%A0
func DecodeModule(binary []byte) (*wasm.Module, error) {
	r := bytes.NewReader(binary)

	// Magic number.
	buf := make([]byte, 4)
	if _, err := io.ReadFull(r, buf); err != nil || !bytes.Equal(buf, magic) {
		return nil, ErrInvalidMagicNumber
	}

	// Version.
	if _, err := io.ReadFull(r, buf); err != nil || !bytes.Equal(buf, version) {
		return nil, ErrInvalidVersion
	}

	m := &wasm.Module{}
	for {
		// TODO: except custom sections, all others are required to be in order, but we aren't checking yet.
		// See https://www.w3.org/TR/wasm-core-1/#modules%E2%91%A0%E2%93%AA
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

		sectionContentStart := r.Len()
		switch sectionID {
		case wasm.SectionIDCustom:
			// First, validate the section and determine if the section for this name has already been set
			name, nameSize, decodeErr := decodeUTF8(r, "custom section name")
			if decodeErr != nil {
				err = decodeErr
				break
			} else if sectionSize < nameSize {
				err = fmt.Errorf("malformed custom section %s", name)
				break
			} else if name == "name" && m.NameSection != nil {
				err = fmt.Errorf("redundant custom section %s", name)
				break
			}

			// Now, either decode the NameSection or skip an unsupported one
			limit := sectionSize - nameSize
			if name == "name" {
				m.NameSection, err = decodeNameSection(r, uint64(limit))
			} else {
				// Note: Not Seek because it doesn't err when given an offset past EOF. Rather, it leads to undefined state.
				if _, err = io.CopyN(io.Discard, r, int64(limit)); err != nil {
					return nil, fmt.Errorf("failed to skip name[%s]: %w", name, err)
				}
			}

		case wasm.SectionIDType:
			m.TypeSection, err = decodeTypeSection(r)
		case wasm.SectionIDImport:
			m.ImportSection, err = decodeImportSection(r)
		case wasm.SectionIDFunction:
			m.FunctionSection, err = decodeFunctionSection(r)
		case wasm.SectionIDTable:
			m.TableSection, err = decodeTableSection(r)
		case wasm.SectionIDMemory:
			m.MemorySection, err = decodeMemorySection(r)
		case wasm.SectionIDGlobal:
			m.GlobalSection, err = decodeGlobalSection(r)
		case wasm.SectionIDExport:
			m.ExportSection, err = decodeExportSection(r)
		case wasm.SectionIDStart:
			m.StartSection, err = decodeStartSection(r)
		case wasm.SectionIDElement:
			m.ElementSection, err = decodeElementSection(r)
		case wasm.SectionIDCode:
			m.CodeSection, err = decodeCodeSection(r)
		case wasm.SectionIDData:
			m.DataSection, err = decodeDataSection(r)
		default:
			err = ErrInvalidSectionID
		}

		readBytes := sectionContentStart - r.Len()
		if err == nil && int(sectionSize) != readBytes {
			err = fmt.Errorf("invalid section length: expected to be %d but got %d", sectionSize, readBytes)
		}

		if err != nil {
			return nil, fmt.Errorf("section %s: %v", wasm.SectionIDName(sectionID), err)
		}
	}

	functionCount, codeCount := m.SectionElementCount(wasm.SectionIDFunction), m.SectionElementCount(wasm.SectionIDCode)
	if functionCount != codeCount {
		return nil, fmt.Errorf("function and code section have inconsistent lengths: %d != %d", functionCount, codeCount)
	}
	return m, nil
}

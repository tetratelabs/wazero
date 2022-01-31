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
		sectionID := make([]byte, 1)
		if _, err := io.ReadFull(r, sectionID); err == io.EOF {
			break
		} else if err != nil {
			return nil, fmt.Errorf("read section id: %w", err)
		}

		sectionSize, _, err := leb128.DecodeUint32(r)
		if err != nil {
			return nil, fmt.Errorf("get size of section for id=%d: %v", sectionID[0], err)
		}

		sectionContentStart := r.Len()
		switch sectionID[0] {
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
			} else if _, ok := m.CustomSections[name]; ok {
				err = fmt.Errorf("redundant custom section %s", name)
				break
			}

			// Now, either decode the NameSection or store an unsupported one
			// TODO: Do we care to store something we don't use? We could also skip it!
			data, dataErr := readCustomSectionData(r, sectionSize-nameSize)
			if dataErr != nil {
				err = dataErr
			} else if name == "name" {
				m.NameSection, err = decodeNameSection(data)
			} else {
				if m.CustomSections == nil {
					m.CustomSections = map[string][]byte{name: data}
				} else {
					m.CustomSections[name] = data
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
			return nil, fmt.Errorf("section ID %d: %v", sectionID[0], err)
		}
	}

	functionCount, codeCount := len(m.FunctionSection), len(m.CodeSection)
	if functionCount != codeCount {
		return nil, fmt.Errorf("function and code section have inconsistent lengths: %d != %d", functionCount, codeCount)
	}
	return m, nil
}

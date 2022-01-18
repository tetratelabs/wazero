package binary

import (
	"bytes"
	"fmt"
	"io"

	"github.com/tetratelabs/wazero/wasm"
	"github.com/tetratelabs/wazero/wasm/leb128"
)

// TODO: maybe io.ByteReader
type reader struct {
	binary []byte
	read   int
	buffer *bytes.Buffer
}

func (r *reader) Read(p []byte) (n int, err error) {
	n, err = r.buffer.Read(p)
	r.read += n
	return
}

// DecodeModule implements wasm.DecodeModule for the WebAssembly 1.0 (MVP) Binary Format
// See https://www.w3.org/TR/wasm-core-1/#binary-format%E2%91%A0
func DecodeModule(binary []byte) (*wasm.Module, error) {
	r := &reader{binary: binary, buffer: bytes.NewBuffer(binary)}

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

		sectionContentStart := r.read
		switch sectionID[0] {
		case SectionIDCustom:
			// First, validate the section and determine if the section for this name has already been set
			name, dataSize, decodeErr := decodeCustomSectionNameAndDataSize(r, sectionSize)
			if decodeErr != nil {
				err = decodeErr
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
			data, dataErr := readCustomSectionData(r, dataSize)
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
		case SectionIDType:
			m.TypeSection, err = decodeTypeSection(r)
		case SectionIDImport:
			m.ImportSection, err = decodeImportSection(r)
		case SectionIDFunction:
			m.FunctionSection, err = decodeFunctionSection(r)
		case SectionIDTable:
			m.TableSection, err = decodeTableSection(r)
		case SectionIDMemory:
			m.MemorySection, err = decodeMemorySection(r)
		case SectionIDGlobal:
			m.GlobalSection, err = decodeGlobalSection(r)
		case SectionIDExport:
			m.ExportSection, err = decodeExportSection(r)
		case SectionIDStart:
			m.StartSection, err = decodeStartSection(r)
		case SectionIDElement:
			m.ElementSection, err = decodeElementSection(r)
		case SectionIDCode:
			m.CodeSection, err = decodeCodeSection(r)
		case SectionIDData:
			m.DataSection, err = decodeDataSection(r)
		default:
			err = ErrInvalidSectionID
		}

		if err == nil && sectionContentStart+int(sectionSize) != r.read {
			err = fmt.Errorf("invalid section length: expected to be %d but got %d", sectionSize, r.read-sectionContentStart)
		}

		if err != nil {
			return nil, fmt.Errorf("section ID %d: %v", sectionID[0], err)
		}
	}

	if len(m.FunctionSection) != len(m.CodeSection) {
		return nil, fmt.Errorf("function and code section have inconsistent lengths")
	}
	return m, nil
}

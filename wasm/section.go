package wasm

import (
	"fmt"
	"io"
	"unicode/utf8"

	"github.com/tetratelabs/wazero/wasm/leb128"
)

type SectionID = byte

const (
	// SectionIDCustom includes the standard defined NameSection and possibly others not defined in the standard.
	SectionIDCustom   SectionID = 0
	SectionIDType     SectionID = 1
	SectionIDImport   SectionID = 2
	SectionIDFunction SectionID = 3
	SectionIDTable    SectionID = 4
	SectionIDMemory   SectionID = 5
	SectionIDGlobal   SectionID = 6
	SectionIDExport   SectionID = 7
	SectionIDStart    SectionID = 8
	SectionIDElement  SectionID = 9
	SectionIDCode     SectionID = 10
	SectionIDData     SectionID = 11
)

// writeSections appends this module's sections into the buffer in the order required by the specification, custom
// section first.
// See https://www.w3.org/TR/wasm-core-1/#modules%E2%91%A0%E2%93%AA
func (m *Module) encodeSections(buffer []byte) (bytes []byte) {
	bytes = buffer
	for name, data := range m.CustomSections {
		bytes = append(bytes, encodeCustomSection(name, data)...)
	}
	if len(m.TypeSection) > 0 {
		bytes = append(bytes, encodeTypeSection(m.TypeSection)...)
	}
	if len(m.ImportSection) > 0 {
		panic("TODO: ImportSection")
	}
	if len(m.FunctionSection) > 0 {
		panic("TODO: FunctionSection")
	}
	if len(m.TableSection) > 0 {
		panic("TODO: TableSection")
	}
	if len(m.MemorySection) > 0 {
		panic("TODO: MemorySection")
	}
	if len(m.GlobalSection) > 0 {
		panic("TODO: GlobalSection")
	}
	if len(m.ExportSection) > 0 {
		panic("TODO: ExportSection")
	}
	if m.StartSection != nil {
		panic("TODO: StartSection")
	}
	if len(m.ElementSection) > 0 {
		panic("TODO: ElementSection")
	}
	if len(m.CodeSection) > 0 {
		panic("TODO: CodeSection")
	}
	if len(m.DataSection) > 0 {
		panic("TODO: DataSection")
	}
	// >> The name section should appear only once in a module, and only after the data section.
	// See https://www.w3.org/TR/wasm-core-1/#binary-namesec
	if m.NameSection != nil {
		bytes = append(bytes, encodeCustomSection("name", m.NameSection.EncodeData())...)
	}
	return
}

// encodeCustomSection encodes the opaque bytes for the given name as a SectionIDCustom
// See https://www.w3.org/TR/wasm-core-1/#binary-customsec
func encodeCustomSection(name string, data []byte) []byte {
	// The contents of a custom section is the non-empty name followed by potentially empty opaque data
	contents := append(encodeSizePrefixed([]byte(name)), data...)
	return encodeSection(SectionIDCustom, contents)
}

// encodeSection encodes the sectionID, the size of its contents in bytes, followed by the contents.
// See https://www.w3.org/TR/wasm-core-1/#sections%E2%91%A0
func encodeSection(sectionID SectionID, contents []byte) []byte {
	return append([]byte{sectionID}, encodeSizePrefixed(contents)...)
}

// encodeTypeSection encodes a SectionIDType with any types in the Module.TypeSection
// See https://www.w3.org/TR/wasm-core-1/#type-section%E2%91%A0
func encodeTypeSection(ts []*FunctionType) []byte {
	typeCount := len(ts)
	data := leb128.EncodeUint32(uint32(typeCount))
	for _, ft := range ts {
		// Function types are encoded by the byte 0x60 followed by the respective vectors of parameter and result types.
		// See https://www.w3.org/TR/wasm-core-1/#function-types%E2%91%A4
		data = append(data, 0x60)
		data = append(data, encodeValTypes(ft.Params)...)
		data = append(data, encodeValTypes(ft.Results)...)
	}

	// Finally, make the header
	dataSize := leb128.EncodeUint32(uint32(len(data)))
	header := append([]byte{SectionIDType}, dataSize...)
	return append(header, data...)
}

func (m *Module) readSections(r *reader) error {
	for {
		sectionID := make([]byte, 1)
		if _, err := io.ReadFull(r, sectionID); err == io.EOF {
			return nil
		} else if err != nil {
			return fmt.Errorf("read section id: %w", err)
		}

		sectionSize, _, err := leb128.DecodeUint32(r)
		if err != nil {
			return fmt.Errorf("get size of section for id=%d: %v", SectionID(sectionID[0]), err)
		}

		sectionContentStart := r.read
		switch sectionID[0] {
		case SectionIDCustom:
			err = m.readSectionCustom(r, int(sectionSize))
		case SectionIDType:
			err = m.readSectionTypes(r)
		case SectionIDImport:
			err = m.readSectionImports(r)
		case SectionIDFunction:
			err = m.readSectionFunctions(r)
		case SectionIDTable:
			err = m.readSectionTables(r)
		case SectionIDMemory:
			err = m.readSectionMemories(r)
		case SectionIDGlobal:
			err = m.readSectionGlobals(r)
		case SectionIDExport:
			err = m.readSectionExports(r)
		case SectionIDStart:
			err = m.readSectionStart(r)
		case SectionIDElement:
			err = m.readSectionElement(r)
		case SectionIDCode:
			err = m.readSectionCodes(r)
		case SectionIDData:
			err = m.readSectionData(r)
		default:
			err = ErrInvalidSectionID
		}

		if err == nil && sectionContentStart+int(sectionSize) != r.read {
			err = fmt.Errorf("invalid section length: expected to be %d but got %d", sectionSize, r.read-sectionContentStart)
		}

		if err != nil {
			return fmt.Errorf("section ID %d: %v", sectionID[0], err)
		}
	}
}

// readSectionCustom reads a custom section into Module.CustomSections unless it is the "name" section, which decodes
// into Module.NameSection.
func (m *Module) readSectionCustom(r *reader, sectionSize int) error {
	nameLen, nameLenSize, err := leb128.DecodeUint32(r)
	if err != nil {
		return fmt.Errorf("cannot read custom section name length")
	}

	nameBuf := make([]byte, nameLen)
	_, err = io.ReadFull(r, nameBuf)
	if err != nil {
		return fmt.Errorf("cannot read custom section name: %v", err)
	}
	if !utf8.Valid(nameBuf) {
		return fmt.Errorf("custom section name must be valid utf8")
	}
	name := string(nameBuf)

	dataSize := sectionSize - int(nameLenSize) - int(nameLen)
	if dataSize < 0 {
		return fmt.Errorf("malformed custom section %s", name)
	}
	data := make([]byte, dataSize)
	_, err = io.ReadFull(r, data)
	if err != nil {
		return fmt.Errorf("cannot read custom section data: %v", err)
	}

	// Decode any known custom sections after checking for redundant keys
	if name == "name" {
		if m.NameSection != nil {
			return fmt.Errorf("malformed custom section %s", name)
		}
		if m.NameSection, err = DecodeNameSection(data); err != nil {
			return err
		}
	} else if _, ok := m.CustomSections[name]; ok {
		return fmt.Errorf("malformed custom section %s", name)
	} else if m.CustomSections == nil {
		m.CustomSections = map[string][]byte{name: data}
	} else {
		m.CustomSections[name] = data
	}
	return nil
}

func (m *Module) readSectionTypes(r *reader) error {
	vs, _, err := leb128.DecodeUint32(r)
	if err != nil {
		return fmt.Errorf("get size of vector: %v", err)
	}

	m.TypeSection = make([]*FunctionType, vs)
	for i := range m.TypeSection {
		m.TypeSection[i], err = readFunctionType(r)
		if err != nil {
			return fmt.Errorf("read %d-th function type: %v", i, err)
		}
	}
	return nil
}

func (m *Module) readSectionImports(r *reader) error {
	vs, _, err := leb128.DecodeUint32(r)
	if err != nil {
		return fmt.Errorf("get size of vector: %v", err)
	}

	m.ImportSection = make([]*Import, vs)
	for i := range m.ImportSection {
		m.ImportSection[i], err = readImport(r)
		if err != nil {
			return fmt.Errorf("read import: %v", err)
		}
	}
	return nil
}

func (m *Module) readSectionFunctions(r *reader) error {
	vs, _, err := leb128.DecodeUint32(r)
	if err != nil {
		return fmt.Errorf("get size of vector: %v", err)
	}

	m.FunctionSection = make([]uint32, vs)
	for i := range m.FunctionSection {
		m.FunctionSection[i], _, err = leb128.DecodeUint32(r)
		if err != nil {
			return fmt.Errorf("get type index: %v", err)
		}
	}
	return nil
}

func (m *Module) readSectionTables(r *reader) error {
	vs, _, err := leb128.DecodeUint32(r)
	if err != nil {
		return fmt.Errorf("get size of vector: %v", err)
	}

	m.TableSection = make([]*TableType, vs)
	for i := range m.TableSection {
		m.TableSection[i], err = readTableType(r)
		if err != nil {
			return fmt.Errorf("read table type: %v", err)
		}
	}
	return nil
}

func (m *Module) readSectionMemories(r *reader) error {
	vs, _, err := leb128.DecodeUint32(r)
	if err != nil {
		return fmt.Errorf("get size of vector: %v", err)
	}

	m.MemorySection = make([]*MemoryType, vs)
	for i := range m.MemorySection {
		m.MemorySection[i], err = readMemoryType(r)
		if err != nil {
			return fmt.Errorf("read memory type: %v", err)
		}
	}
	return nil
}

func (m *Module) readSectionGlobals(r *reader) error {
	vs, _, err := leb128.DecodeUint32(r)
	if err != nil {
		return fmt.Errorf("get size of vector: %v", err)
	}

	m.GlobalSection = make([]*Global, vs)
	for i := range m.GlobalSection {
		m.GlobalSection[i], err = readGlobal(r)
		if err != nil {
			return fmt.Errorf("read global segment: %v ", err)
		}
	}
	return nil
}

func (m *Module) readSectionExports(r *reader) error {
	vs, _, err := leb128.DecodeUint32(r)
	if err != nil {
		return fmt.Errorf("get size of vector: %v", err)
	}

	m.ExportSection = make(map[string]*Export, vs)
	for i := uint32(0); i < vs; i++ {
		expDesc, err := readExport(r)
		if err != nil {
			return fmt.Errorf("read export: %v", err)
		}
		if _, ok := m.ExportSection[expDesc.Name]; ok {
			return fmt.Errorf("duplicate export name: %s", expDesc.Name)
		}
		m.ExportSection[expDesc.Name] = expDesc
	}
	return nil
}

func (m *Module) readSectionStart(r *reader) error {
	vs, _, err := leb128.DecodeUint32(r)
	if err != nil {
		return fmt.Errorf("get size of vector: %v", err)
	}

	m.StartSection = &vs
	return nil
}

func (m *Module) readSectionElement(r *reader) error {
	vs, _, err := leb128.DecodeUint32(r)
	if err != nil {
		return fmt.Errorf("get size of vector: %v", err)
	}

	m.ElementSection = make([]*ElementSegment, vs)
	for i := range m.ElementSection {
		m.ElementSection[i], err = readElementSegment(r)
		if err != nil {
			return fmt.Errorf("read element: %v", err)
		}
	}
	return nil
}

func (m *Module) readSectionCodes(r *reader) error {
	vs, _, err := leb128.DecodeUint32(r)
	if err != nil {
		return fmt.Errorf("get size of vector: %v", err)
	}
	m.CodeSection = make([]*Code, vs)

	for i := range m.CodeSection {
		m.CodeSection[i], err = readCode(r)
		if err != nil {
			return fmt.Errorf("read %d-th code segment: %v", i, err)
		}
	}
	return nil
}

func (m *Module) readSectionData(r *reader) error {
	vs, _, err := leb128.DecodeUint32(r)
	if err != nil {
		return fmt.Errorf("get size of vector: %v", err)
	}

	m.DataSection = make([]*DataSegment, vs)
	for i := range m.DataSection {
		m.DataSection[i], err = readDataSegment(r)
		if err != nil {
			return fmt.Errorf("read data segment: %v", err)
		}
	}
	return nil
}

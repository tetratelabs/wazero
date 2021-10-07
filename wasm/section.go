package wasm

import (
	"errors"
	"fmt"
	"io"
	"unicode/utf8"

	"github.com/mathetake/gasm/wasm/leb128"
)

type SectionID byte

const (
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

func (m *Module) readSections(r *Reader) error {
	for {
		b := make([]byte, 1)
		if _, err := io.ReadFull(r, b); err == io.EOF {
			return nil
		} else if err != nil {
			return fmt.Errorf("read section id: %w", err)
		}

		ss, _, err := leb128.DecodeUint32(r)
		if err != nil {
			return fmt.Errorf("get size of section for id=%d: %v", SectionID(b[0]), err)
		}

		sectionContentStart := r.read
		switch SectionID(b[0]) {
		case SectionIDCustom:
			err = m.readSectionCustom(r, int(ss))
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
			err = errors.New("invalid section id")
		}

		if sectionContentStart+int(ss) != r.read {
			err = fmt.Errorf("invalid section length: expected to be %d but got %d", ss, r.read-sectionContentStart)
		}

		if err != nil {
			return fmt.Errorf("section ID %d: %v", b[0], err)
		}
	}
}

func (m *Module) readSectionCustom(r *Reader, sectionSize int) error {
	nameLen, nameLenBytes, err := leb128.DecodeUint32(r)
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
	contentLen := int(sectionSize) - int(nameLenBytes) - int(nameLen)
	if contentLen < 0 {
		return fmt.Errorf("malformed custom section %s", string(nameBuf))
	}
	contents := make([]byte, contentLen)
	_, err = io.ReadFull(r, contents)
	if err != nil {
		return fmt.Errorf("cannot read custom section contents: %v", err)
	}
	m.CustomSections[string(nameBuf)] = contents
	return nil
}

func (m *Module) readSectionTypes(r *Reader) error {
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

func (m *Module) readSectionImports(r *Reader) error {
	vs, _, err := leb128.DecodeUint32(r)
	if err != nil {
		return fmt.Errorf("get size of vector: %v", err)
	}

	m.ImportSection = make([]*ImportSegment, vs)
	for i := range m.ImportSection {
		m.ImportSection[i], err = readImportSegment(r)
		if err != nil {
			return fmt.Errorf("read import: %v", err)
		}
	}
	return nil
}

func (m *Module) readSectionFunctions(r *Reader) error {
	vs, _, err := leb128.DecodeUint32(r)
	if err != nil {
		return fmt.Errorf("get size of vector: %v", err)
	}

	m.FunctionSection = make([]uint32, vs)
	for i := range m.FunctionSection {
		m.FunctionSection[i], _, err = leb128.DecodeUint32(r)
		if err != nil {
			return fmt.Errorf("get typeidx: %v", err)
		}
	}
	return nil
}

func (m *Module) readSectionTables(r *Reader) error {
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

func (m *Module) readSectionMemories(r *Reader) error {
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

func (m *Module) readSectionGlobals(r *Reader) error {
	vs, _, err := leb128.DecodeUint32(r)
	if err != nil {
		return fmt.Errorf("get size of vector: %v", err)
	}

	m.GlobalSection = make([]*GlobalSegment, vs)
	for i := range m.GlobalSection {
		m.GlobalSection[i], err = readGlobalSegment(r)
		if err != nil {
			return fmt.Errorf("read global segment: %v ", err)
		}
	}
	return nil
}

func (m *Module) readSectionExports(r *Reader) error {
	vs, _, err := leb128.DecodeUint32(r)
	if err != nil {
		return fmt.Errorf("get size of vector: %v", err)
	}

	m.ExportSection = make(map[string]*ExportSegment, vs)
	for i := uint32(0); i < vs; i++ {
		expDesc, err := readExportSegment(r)
		if err != nil {
			return fmt.Errorf("read export: %v", err)
		}

		m.ExportSection[expDesc.Name] = expDesc
	}
	return nil
}

func (m *Module) readSectionStart(r *Reader) error {
	vs, _, err := leb128.DecodeUint32(r)
	if err != nil {
		return fmt.Errorf("get size of vector: %v", err)
	}

	m.StartSection = &vs
	return nil
}

func (m *Module) readSectionElement(r *Reader) error {
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

func (m *Module) readSectionCodes(r *Reader) error {
	vs, _, err := leb128.DecodeUint32(r)
	if err != nil {
		return fmt.Errorf("get size of vector: %v", err)
	}
	m.CodeSection = make([]*CodeSegment, vs)

	for i := range m.CodeSection {
		m.CodeSection[i], err = readCodeSegment(r)
		if err != nil {
			return fmt.Errorf("read code segment: %v", err)
		}
	}
	return nil
}

func (m *Module) readSectionData(r *Reader) error {
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

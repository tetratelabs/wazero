package wasm

import (
	"bytes"
	"fmt"
	"math"

	"github.com/tetratelabs/wazero/internal/leb128"
)

// Table describes the limits of (function) elements in a table.
type Table = limitsType

// ElementSegment are initialization instructions for a TableInstance
//
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#syntax-elem
type ElementSegment struct {
	// OffsetExpr returns the table element offset to apply to Init indices.
	// Note: This can be validated prior to instantiation unless it includes OpcodeGlobalGet (an imported global).
	OffsetExpr *ConstantExpression

	// Init indices are table elements relative to the result of OffsetExpr. The values are positions in the function
	// index namespace that initialize the corresponding element.
	Init []Index
}

// TableInstance represents a table of (ElemTypeFuncref) elements in a module.
//
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#table-instances%E2%91%A0
type TableInstance struct {
	// Table holds the Engine-specific compiled functions mapped by element index.
	Table []interface{}

	// Min is the minimum (function) elements in this table and cannot grow to accommodate ElementSegment.
	Min uint32

	// Max if present is the maximum (function) elements in this table, or nil if unbounded.
	Max *uint32
}

// validatedElementSegment is like ElementSegment except the inputs are expanded and validated based on defining module.
//
// Note: The global imported at globalIdx may have an offset value that is out-of-bounds for the corresponding table.
type validatedElementSegment struct {
	// opcode is OpcodeGlobalGet or OpcodeI32Const
	opcode Opcode

	// arg is the only argument to opcode, which when applied results in the offset to add to init indices.
	//  * OpcodeGlobalGet: position in the global index namespace of an imported Global ValueTypeI32 holding the offset.
	//  * OpcodeI32Const: a constant ValueTypeI32 offset.
	arg uint32

	// init are a range of table elements whose values are positions in the function index namespace. This range
	// replaces any values in TableInstance.Table at an offset arg which is a constant if opcode == OpcodeI32Const or
	// derived from a globalIdx if opcode == OpcodeGlobalGet
	init []Index
}

// validateTable ensures any ElementSegment is valid. This caches results via Module.validatedElementSegments.
// Note: limitsType are validated by decoders, so not re-validated here.
func (m *Module) validateTable() ([]*validatedElementSegment, error) {
	if m.validatedElementSegments != nil {
		return m.validatedElementSegments, nil
	}

	t := m.TableSection
	imported := false
	for _, im := range m.ImportSection {
		if im.Type == ExternTypeTable {
			t = im.DescTable
			imported = true
			break
		}
	}

	elementCount := m.SectionElementCount(SectionIDElement)
	if elementCount > 0 && t == nil {
		return nil, fmt.Errorf("%s was defined, but not table", SectionIDName(SectionIDElement))
	}

	ret := make([]*validatedElementSegment, 0, elementCount)

	// Create bounds checks as these can err prior to instantiation
	funcCount := m.importCount(ExternTypeFunc) + m.SectionElementCount(SectionIDFunction)

	// Now, we have to figure out which table elements can be resolved before instantiation and also fail early if there
	// are any imported globals that are known to be invalid by their declarations.
	for i, elem := range m.ElementSection {
		idx := Index(i)

		initCount := uint32(len(elem.Init))

		// Any offset applied is to the element, not the function index: validate here if the funcidx is sound.
		for ei, funcIdx := range elem.Init {
			if funcIdx >= funcCount {
				return nil, fmt.Errorf("%s[%d].init[%d] funcidx %d out of range", SectionIDName(SectionIDElement), idx, ei, funcIdx)
			}
		}

		// global.get needs to be discovered during initialization
		oc := elem.OffsetExpr.Opcode
		if oc == OpcodeGlobalGet {
			globalIdx, _, err := leb128.DecodeUint32(bytes.NewReader(elem.OffsetExpr.Data))
			if err != nil {
				return nil, fmt.Errorf("%s[%d] couldn't read global.get parameter: %w", SectionIDName(SectionIDElement), idx, err)
			} else if err = m.verifyImportGlobalI32(SectionIDElement, idx, globalIdx); err != nil {
				return nil, err
			}

			if initCount == 0 {
				continue // Per https://github.com/WebAssembly/spec/issues/1427 init can be no-op, but validate anyway!
			}

			ret = append(ret, &validatedElementSegment{oc, globalIdx, elem.Init})
		} else if oc == OpcodeI32Const {
			o, _, err := leb128.DecodeInt32(bytes.NewReader(elem.OffsetExpr.Data))
			if err != nil {
				return nil, fmt.Errorf("%s[%d] couldn't read i32.const parameter: %w", SectionIDName(SectionIDElement), idx, err)
			}
			offset := Index(o)

			// Per https://github.com/WebAssembly/spec/blob/wg-1.0/test/core/elem.wast#L117 we must pass if imported
			// table has set its min=0. Per https://github.com/WebAssembly/spec/blob/wg-1.0/test/core/elem.wast#L142, we
			// have to do fail if module-defined min=0.
			if !imported {
				if err = checkSegmentBounds(t.Min, uint64(initCount)+uint64(offset), idx); err != nil {
					return nil, err
				}
			}

			if initCount == 0 {
				continue // Per https://github.com/WebAssembly/spec/issues/1427 init can be no-op, but validate anyway!
			}

			ret = append(ret, &validatedElementSegment{oc, offset, elem.Init})
		} else {
			return nil, fmt.Errorf("%s[%d] has an invalid const expression: %s", SectionIDName(SectionIDElement), idx, InstructionName(oc))
		}
	}

	m.validatedElementSegments = ret
	return ret, nil
}

// buildTable returns a TableInstance if the module defines or imports a table.
//  * importedTable: returned as `table` unmodified, if non-nil.
//  * importedGlobals: include all instantiated, imported globals.
//
// If the result `init` is non-nil, it is the `tableInit` parameter of Engine.NewModuleEngine.
//
// Note: An error is only possible when an ElementSegment.OffsetExpr is out of range of the TableInstance.Min.
func (m *Module) buildTable(importedTable *TableInstance, importedGlobals []*GlobalInstance) (table *TableInstance, init map[Index]Index, err error) {
	// The module defining the table is the one that sets its Min/Max etc.
	if m.TableSection != nil {
		t := m.TableSection
		table = &TableInstance{Table: make([]interface{}, t.Min), Min: t.Min, Max: t.Max}
	} else {
		table = importedTable
	}
	if table == nil {
		return // no table
	}

	elementSegments := m.validatedElementSegments
	if len(elementSegments) == 0 {
		return
	}

	init = make(map[Index]Index, table.Min)
	for elemI, elem := range elementSegments {
		var offset uint32
		if elem.opcode == OpcodeGlobalGet {
			global := importedGlobals[elem.arg]
			offset = uint32(global.Val)
		} else {
			offset = elem.arg // constant
		}

		// Check to see if we are out-of-bounds
		initCount := uint64(len(elem.init))
		if err = checkSegmentBounds(table.Min, uint64(offset)+initCount, Index(elemI)); err != nil {
			return
		}

		for i, funcidx := range elem.init {
			init[offset+uint32(i)] = funcidx
		}
	}
	return
}

// checkSegmentBounds fails if the capacity needed for an ElementSegment.Init is larger than limitsType.Min
//
// WebAssembly 1.0 (20191205) doesn't forbid growing to accommodate element segments, and spectests are inconsistent.
// For example, the spectests enforce elements within Table limitsType.Min, but ignore Import.DescTable min. What this
// means is we have to delay offset checks on imported tables until we link to them.
// Ex. https://github.com/WebAssembly/spec/blob/wg-1.0/test/core/elem.wast#L117 wants pass on min=0 for import
// Ex. https://github.com/WebAssembly/spec/blob/wg-1.0/test/core/elem.wast#L142 wants fail on min=0 module-defined
func checkSegmentBounds(min uint32, requireMin uint64, idx Index) error { // uint64 in case offset was set to -1
	if requireMin > uint64(min) {
		return fmt.Errorf("%s[%d].init exceeds min table size", SectionIDName(SectionIDElement), idx)
	}
	return nil
}

func (m *Module) verifyImportGlobalI32(sectionID SectionID, sectionIdx Index, idx uint32) error {
	ig := uint32(math.MaxUint32) // +1 == 0
	for i, im := range m.ImportSection {
		if im.Type == ExternTypeGlobal {
			ig++
			if ig == idx {
				if im.DescGlobal.ValType != ValueTypeI32 {
					return fmt.Errorf("%s[%d] (global.get %d): import[%d].global.ValType != i32", SectionIDName(sectionID), sectionIdx, idx, i)
				}
				return nil
			}
		}
	}
	return fmt.Errorf("%s[%d] (global.get %d): out of range of imported globals", SectionIDName(sectionID), sectionIdx, idx)
}

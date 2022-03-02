package internalwasm

import (
	"bytes"
	"fmt"

	"github.com/tetratelabs/wazero/internal/ieee754"
	"github.com/tetratelabs/wazero/internal/leb128"
	publicwasm "github.com/tetratelabs/wazero/wasm"
)

// DecodeModule parses the configured source into a Module. This function returns when the source is exhausted or
// an error occurs. The result can be initialized for use via Store.Instantiate.
//
// Here's a description of the return values:
// * result is the module parsed or nil on error
// * err is a FormatError invoking the parser, dangling block comments or unexpected characters.
// See binary.DecodeModule and text.DecodeModule
type DecodeModule func(source []byte) (result *Module, err error)

// EncodeModule encodes the given module into a byte slice depending on the format of the implementation.
// See binary.EncodeModule
type EncodeModule func(m *Module) (bytes []byte)

// Module is a WebAssembly binary representation.
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#modules%E2%91%A8
//
// Differences from the specification:
// * The NameSection is decoded, so not present as a key "name" in CustomSections.
// * The ExportSection is represented as a map for lookup convenience.
type Module struct {
	// TypeSection contains the unique FunctionType of functions imported or defined in this module.
	//
	// Note: Currently, there is no type ambiguity in the index as WebAssembly 1.0 only defines function type.
	// In the future, other types may be introduced to support features such as module linking.
	//
	// Note: In the Binary Format, this is SectionIDType.
	//
	// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#types%E2%91%A0%E2%91%A0
	TypeSection []*FunctionType

	// ImportSection contains imported functions, tables, memories or globals required for instantiation
	// (Store.Instantiate).
	//
	// Note: there are no unique constraints relating to the two-level namespace of Import.Module and Import.Name.
	//
	// Note: In the Binary Format, this is SectionIDImport.
	//
	// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#import-section%E2%91%A0
	ImportSection []*Import

	// FunctionSection contains the index in TypeSection of each function defined in this module.
	//
	// Note: The function Index namespace begins with imported functions and ends with those defined in this module.
	// For example, if there are two imported functions and one defined in this module, the function Index 3 is defined
	// in this module at FunctionSection[0].
	//
	// Note: FunctionSection is index correlated with the CodeSection. If given the same position, ex. 2, a function
	// type is at TypeSection[FunctionSection[2]], while its locals and body are at CodeSection[2].
	//
	// Note: In the Binary Format, this is SectionIDFunction.
	//
	// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#function-section%E2%91%A0
	FunctionSection []Index

	// TableSection contains each table defined in this module.
	//
	// Note: The table Index namespace begins with imported tables and ends with those defined in this module.
	// For example, if there are two imported tables and one defined in this module, the table Index 3 is defined in
	// this module at TableSection[0].
	//
	// Note: Version 1.0 (20191205) of the WebAssembly spec allows at most one table definition per module, so the
	// length of the TableSection can be zero or one, and can only be one if there is no imported table.
	//
	// Note: In the Binary Format, this is SectionIDTable.
	//
	// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#table-section%E2%91%A0
	TableSection []*TableType

	// MemorySection contains each memory defined in this module.
	//
	// Note: The memory Index namespace begins with imported memories and ends with those defined in this module.
	// For example, if there are two imported memories and one defined in this module, the memory Index 3 is defined in
	// this module at TableSection[0].
	//
	// Note: Version 1.0 (20191205) of the WebAssembly spec allows at most one memory definition per module, so the length of
	// the MemorySection can be zero or one, and can only be one if there is no imported memory.
	//
	// Note: In the Binary Format, this is SectionIDMemory.
	//
	// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#memory-section%E2%91%A0
	MemorySection []*MemoryType

	// GlobalSection contains each global defined in this module.
	//
	// Global indexes are offset by any imported globals because the global index space begins with imports, followed by
	// ones defined in this module. For example, if there are two imported globals and three defined in this module, the
	// global at index 3 is defined in this module at GlobalSection[0].
	//
	// Note: In the Binary Format, this is SectionIDGlobal.
	//
	// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#global-section%E2%91%A0
	GlobalSection []*Global

	// ExportSection contains each export defined in this module.
	//
	// Note: In the Binary Format, this is SectionIDExport.
	//
	// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#exports%E2%91%A0
	ExportSection map[string]*Export

	// StartSection is the index of a function to call before returning from Store.Instantiate.
	//
	// Note: The index here is not the position in the FunctionSection, rather in the function index namespace, which
	// begins with imported functions.
	//
	// Note: In the Binary Format, this is SectionIDStart.
	//
	// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#start-section%E2%91%A0
	StartSection *Index

	// Note: In the Binary Format, this is SectionIDElement.
	ElementSection []*ElementSegment

	// CodeSection is index-correlated with FunctionSection and contains each function's locals and body.
	//
	// Note: In the Binary Format, this is SectionIDCode.
	//
	// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#code-section%E2%91%A0
	CodeSection []*Code

	// Note: In the Binary Format, this is SectionIDData.
	DataSection []*DataSegment

	// NameSection is set when the SectionIDCustom "name" was successfully decoded from the binary format.
	//
	// Note: This is the only SectionIDCustom defined in the WebAssembly 1.0 (20191205) Binary Format.
	// Others are skipped as they are not used in wazero.
	//
	// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#name-section%E2%91%A0
	// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#custom-section%E2%91%A0
	NameSection *NameSection
}

// TypeOfFunction returns the internalwasm.SectionIDType index for the given function namespace index or nil.
// Note: The function index namespace is preceded by imported functions.
// TODO: Returning nil should be impossible when decode results are validated. Validate decode before backfilling tests.
func (m *Module) TypeOfFunction(funcIdx Index) *FunctionType {
	typeSectionLength := uint32(len(m.TypeSection))
	if typeSectionLength == 0 {
		return nil
	}
	funcImportCount := Index(0)
	for i, im := range m.ImportSection {
		if im.Type == ExternTypeFunc {
			if funcIdx == Index(i) {
				if im.DescFunc >= typeSectionLength {
					return nil
				}
				return m.TypeSection[im.DescFunc]
			}
			funcImportCount++
		}
	}
	funcSectionIdx := funcIdx - funcImportCount
	if funcSectionIdx >= uint32(len(m.FunctionSection)) {
		return nil
	}
	typeIdx := m.FunctionSection[funcSectionIdx]
	if typeIdx >= typeSectionLength {
		return nil
	}
	return m.TypeSection[typeIdx]
}

func (m *Module) Validate() error {
	if err := m.validateStartSection(); err != nil {
		return err
	}

	functions, globals, memories, tables := m.allDeclarations()

	// The wazero specific limitation described at RATIONALE.md.
	const maximumGlobals = 1 << 27
	if err := m.validateGlobals(globals, maximumGlobals); err != nil {
		return err
	}

	if err := m.validateTables(tables, globals); err != nil {
		return err
	}

	if err := m.validateMemories(memories, globals); err != nil {
		return err
	}

	if err := m.validateFunctions(functions, globals, memories, tables); err != nil {
		return err
	}

	if err := m.validateExports(functions, globals, memories, tables); err != nil {
		return err
	}

	return nil
}

func (m *Module) validateStartSection() error {
	// Check the start function is valid.
	// TODO: this should be verified during decode so that errors have the correct source positions
	if m.StartSection != nil {
		startIndex := *m.StartSection
		ft := m.TypeOfFunction(startIndex)
		if ft == nil { // TODO: move this check to decoder so that a module can never be decoded invalidly
			return fmt.Errorf("start function has an invalid type")
		}
		if len(ft.Params) > 0 || len(ft.Results) > 0 {
			return fmt.Errorf("start function must have an empty (nullary) signature: %s", ft.String())
		}
	}
	return nil
}

func (m *Module) validateGlobals(globals []*GlobalType, maxGlobals int) error {
	if len(globals) > maxGlobals {
		return fmt.Errorf("too many globals in a module")
	}

	// Global's initialization constant expression can only reference the imported globals.
	// See the note on https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#constant-expressions%E2%91%A0
	importedGlobals := globals[:m.ImportGlobalCount()]
	for _, g := range m.GlobalSection {
		if err := validateConstExpression(importedGlobals, g.Init, g.Type.ValType); err != nil {
			return err
		}
	}
	return nil
}

func (m *Module) validateFunctions(functions []Index, globals []*GlobalType, memories []*MemoryType, tables []*TableType) error {
	// The wazero specific limitation described at RATIONALE.md.
	const maximumValuesOnStack = 1 << 27

	for codeIndex, typeIndex := range m.FunctionSection {
		if typeIndex >= m.SectionElementCount(SectionIDType) {
			return fmt.Errorf("function type index out of range")
		} else if uint32(codeIndex) >= m.SectionElementCount(SectionIDCode) {
			return fmt.Errorf("code index out of range")
		}

		if err := validateFunction(
			m.TypeSection[typeIndex],
			m.CodeSection[codeIndex].Body,
			m.CodeSection[codeIndex].LocalTypes,
			functions, globals, memories, tables, m.TypeSection, maximumValuesOnStack); err != nil {
			idx := m.SectionElementCount(SectionIDFunction) - 1
			return fmt.Errorf("invalid function (%d/%d): %v", codeIndex, idx, err)
		}
	}
	return nil
}

func (m *Module) validateTables(tables []*TableType, globals []*GlobalType) error {
	if len(tables) > 1 {
		return fmt.Errorf("multiple tables are not supported")
	}

	for _, elem := range m.ElementSection {
		if int(elem.TableIndex) >= len(tables) {
			return fmt.Errorf("table index out of range")
		}
		err := validateConstExpression(globals, elem.OffsetExpr, ValueTypeI32)
		if err != nil {
			return fmt.Errorf("invalid const expression for element: %w", err)
		}
	}
	return nil
}

func (m *Module) validateMemories(memories []*MemoryType, globals []*GlobalType) error {
	if len(memories) > 1 {
		return fmt.Errorf("multiple memories are not supported")
	} else if len(m.DataSection) > 0 && len(memories) == 0 {
		return fmt.Errorf("unknown memory")
	}

	for _, d := range m.DataSection {
		if d.MemoryIndex != 0 {
			return fmt.Errorf("memory index must be zero")
		}
		if err := validateConstExpression(globals, d.OffsetExpression, ValueTypeI32); err != nil {
			return fmt.Errorf("calculate offset: %w", err)
		}
	}
	return nil
}

func (m *Module) validateExports(functions []Index, globals []*GlobalType, memories []*MemoryType, tables []*TableType) error {
	for name, exp := range m.ExportSection {
		index := exp.Index
		switch exp.Type {
		case ExternTypeFunc:
			if index >= uint32(len(functions)) {
				return fmt.Errorf("unknown function for export[%s]", name)
			}
		case ExternTypeGlobal:
			if index >= uint32(len(globals)) {
				return fmt.Errorf("unknown global for export[%s]", name)
			}
		case ExternTypeMemory:
			if index != 0 || len(memories) == 0 {
				return fmt.Errorf("unknown memory for export[%s]", name)
			}
		case ExternTypeTable:
			if index >= uint32(len(tables)) {
				return fmt.Errorf("unknown table for export[%s]", name)
			}
		}
	}
	return nil
}

func validateConstExpression(globals []*GlobalType, expr *ConstantExpression, expectedType ValueType) (err error) {
	var actualType ValueType
	r := bytes.NewBuffer(expr.Data)
	switch expr.Opcode {
	case OpcodeI32Const:
		_, _, err = leb128.DecodeInt32(r)
		if err != nil {
			return fmt.Errorf("read i32: %w", err)
		}
		actualType = ValueTypeI32
	case OpcodeI64Const:
		_, _, err = leb128.DecodeInt64(r)
		if err != nil {
			return fmt.Errorf("read i64: %w", err)
		}
		actualType = ValueTypeI64
	case OpcodeF32Const:
		_, err = ieee754.DecodeFloat32(r)
		if err != nil {
			return fmt.Errorf("read f32: %w", err)
		}
		actualType = ValueTypeF32
	case OpcodeF64Const:
		_, err = ieee754.DecodeFloat64(r)
		if err != nil {
			return fmt.Errorf("read f64: %w", err)
		}
		actualType = ValueTypeF64
	case OpcodeGlobalGet:
		id, _, err := leb128.DecodeUint32(r)
		if err != nil {
			return fmt.Errorf("read index of global: %w", err)
		}
		if uint32(len(globals)) <= id {
			return fmt.Errorf("global index out of range")
		}
		actualType = globals[id].ValType
	default:
		return fmt.Errorf("invalid opcode for const expression: 0x%x", expr.Opcode)
	}

	if actualType != expectedType {
		return fmt.Errorf("const expression type mismatch")
	}
	return nil
}

func (m *Module) buildGlobalInstances(importedGlobals []*GlobalInstance) (globals []*GlobalInstance) {
	for _, gs := range m.GlobalSection {
		var gv uint64
		// Global's initialization constant expression can only reference the imported globals.
		// See the note on https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#constant-expressions%E2%91%A0
		switch v := executeConstExpression(importedGlobals, gs.Init).(type) {
		case int32:
			gv = uint64(v)
		case int64:
			gv = uint64(v)
		case float32:
			gv = publicwasm.EncodeF32(v)
		case float64:
			gv = publicwasm.EncodeF64(v)
		}
		globals = append(globals, &GlobalInstance{
			Type: gs.Type,
			Val:  gv,
		})
	}
	return
}

func (m *Module) buildFunctionInstances() (functions []*FunctionInstance) {
	var functionNames NameMap
	if m.NameSection != nil {
		functionNames = m.NameSection.FunctionNames
	}

	importCount := m.ImportFuncCount()
	n, nLen := 0, len(functionNames)
	for codeIndex := range m.FunctionSection {
		funcIdx := Index(importCount + uint32(len(functions)))
		// Seek to see if there's a better name than "unknown"
		name := "unknown"
		for ; n < nLen; n++ {
			next := functionNames[n]
			if next.Index > funcIdx {
				break // we have function names, but starting at a later index
			} else if next.Index == funcIdx {
				name = next.Name
				break
			}
		}

		f := &FunctionInstance{
			Name:         name,
			FunctionKind: FunctionKindWasm,
			Body:         m.CodeSection[codeIndex].Body,
			LocalTypes:   m.CodeSection[codeIndex].LocalTypes,
		}
		functions = append(functions, f)
	}
	return
}

func (module *Module) buildMemoryInstance() (mem *MemoryInstance) {
	for _, memSec := range module.MemorySection {
		mem = &MemoryInstance{
			Buffer: make([]byte, MemoryPagesToBytesNum(memSec.Min)),
			Min:    memSec.Min,
			Max:    memSec.Max,
		}
	}
	return
}

func (module *Module) buildTableInstance() (table *TableInstance) {
	for _, tableSeg := range module.TableSection {
		table = newTableInstance(tableSeg.Limit.Min, tableSeg.Limit.Max)
	}
	return
}

// Index is the offset in an index namespace, not necessarily an absolute position in a Module section. This is because
// index namespaces are often preceded by a corresponding type in the Module.ImportSection.
//
// For example, the function index namespace starts with any ExternTypeFunc in the Module.ImportSection followed by
// the Module.FunctionSection
//
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#binary-index
type Index = uint32

// FunctionType is a possibly empty function signature.
//
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#function-types%E2%91%A0
type FunctionType struct {
	// Params are the possibly empty sequence of value types accepted by a function with this signature.
	Params []ValueType

	// Results are the possibly empty sequence of value types returned by a function with this signature.
	//
	// Note: In WebAssembly 1.0 (20191205), there can be at most one result.
	// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#result-types%E2%91%A0
	Results []ValueType
}

func (t *FunctionType) String() (ret string) {
	for _, b := range t.Params {
		ret += ValueTypeName(b)
	}
	if len(t.Params) == 0 {
		ret += "null"
	}
	ret += "_"
	for _, b := range t.Results {
		ret += ValueTypeName(b)
	}
	if len(t.Results) == 0 {
		ret += "null"
	}
	return
}

// Import is the binary representation of an import indicated by Type
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#binary-import
type Import struct {
	Type ExternType
	// Module is the possibly empty primary namespace of this import
	Module string
	// Module is the possibly empty secondary namespace of this import
	Name string
	// DescFunc is the index in Module.TypeSection when Type equals ExternTypeFunc
	DescFunc Index
	// DescTable is the inlined TableType when Type equals ExternTypeTable
	DescTable *TableType
	// DescMem is the inlined MemoryType when Type equals ExternTypeMemory
	DescMem *MemoryType
	// DescGlobal is the inlined GlobalType when Type equals ExternTypeGlobal
	DescGlobal *GlobalType
}

type LimitsType struct {
	Min uint32
	Max *uint32
}

type TableType struct {
	ElemType byte
	Limit    *LimitsType
}

type MemoryType = LimitsType

type GlobalType struct {
	ValType ValueType
	Mutable bool
}

type Global struct {
	Type *GlobalType
	Init *ConstantExpression
}

type ConstantExpression struct {
	Opcode Opcode
	Data   []byte
}

// Export is the binary representation of an export indicated by Type
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#binary-export
type Export struct {
	Type ExternType
	// Name is what the host refers to this definition as.
	Name string
	// Index is the index of the definition to export, the index namespace is by Type
	// Ex. If ExternTypeFunc, this is a position in the function index namespace.
	Index Index
}

type ElementSegment struct {
	TableIndex Index
	OffsetExpr *ConstantExpression
	Init       []uint32
}

// Code is an entry in the Module.CodeSection containing the locals and body of the function.
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#binary-code
type Code struct {
	// LocalTypes are any function-scoped variables in insertion order.
	// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#binary-local
	LocalTypes []ValueType
	// Body is a sequence of expressions ending in OpcodeEnd
	// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#binary-expr
	Body []byte
}

type DataSegment struct {
	MemoryIndex      Index // supposed to be zero
	OffsetExpression *ConstantExpression
	Init             []byte
}

// NameSection represent the known custom name subsections defined in the WebAssembly Binary Format
//
// Note: This can be nil if no names were decoded for any reason including configuration.
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#name-section%E2%91%A0
type NameSection struct {
	// ModuleName is the symbolic identifier for a module. Ex. math
	//
	// Note: This can be empty for any reason including configuration.
	ModuleName string

	// FunctionNames is an association of a function index to its symbolic identifier. Ex. add
	//
	// * the key (idx) is in the function namespace, where module defined functions are preceded by imported ones.
	// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#functions%E2%91%A7
	//
	// Ex. Assuming the below text format is the second import, you would expect FunctionNames[1] = "mul"
	//	(import "Math" "Mul" (func $mul (param $x f32) (param $y f32) (result f32)))
	//
	// Note: FunctionNames are only used for debugging. At runtime, functions are called based on raw numeric index.
	// Note: This can be nil for any reason including configuration.
	FunctionNames NameMap

	// LocalNames contains symbolic names for function parameters or locals that have one.
	//
	// Note: In the Text Format, function local names can inherit parameter names from their type. Ex.
	//  * (module (import (func (param $x i32) (param i32))) (func (type 0))) = [{0, {x,0}}]
	//  * (module (import (func (param i32) (param $y i32))) (func (type 0) (local $z i32))) = [0, [{y,1},{z,2}]]
	//  * (module (func (param $x i32) (local $y i32) (local $z i32))) = [{x,0},{y,1},{z,2}]
	//
	// Note: LocalNames are only used for debugging. At runtime, locals are called based on raw numeric index.
	// Note: This can be nil for any reason including configuration.
	LocalNames IndirectNameMap
}

// NameMap associates an index with any associated names.
//
// Note: Often the index namespace bridges multiple sections. For example, the function index namespace starts with any
// ExternTypeFunc in the Module.ImportSection followed by the Module.FunctionSection
//
// Note: NameMap is unique by NameAssoc.Index, but NameAssoc.Name needn't be unique.
// Note: When encoding in the Binary format, this must be ordered by NameAssoc.Index
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#binary-namemap
type NameMap []*NameAssoc

type NameAssoc struct {
	Index Index
	Name  string
}

// IndirectNameMap associates an index with an association of names.
//
// Note: IndirectNameMap is unique by NameMapAssoc.Index, but NameMapAssoc.NameMap needn't be unique.
// Note: When encoding in the Binary format, this must be ordered by NameMapAssoc.Index
// https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#binary-indirectnamemap
type IndirectNameMap []*NameMapAssoc

type NameMapAssoc struct {
	Index   Index
	NameMap NameMap
}

// allDeclarations returns all declarations for functions, globals, memories and tables in a module including imported ones.
func (m *Module) allDeclarations() (functions []Index, globals []*GlobalType, memories []*MemoryType, tables []*TableType) {
	for _, imp := range m.ImportSection {
		switch imp.Type {
		case ExternTypeFunc:
			functions = append(functions, imp.DescFunc)
		case ExternTypeGlobal:
			globals = append(globals, imp.DescGlobal)
		case ExternTypeMemory:
			memories = append(memories, imp.DescMem)
		case ExternTypeTable:
			tables = append(tables, imp.DescTable)
		}
	}

	functions = append(functions, m.FunctionSection...)
	for _, g := range m.GlobalSection {
		globals = append(globals, g.Type)
	}
	memories = append(memories, m.MemorySection...)
	tables = append(tables, m.TableSection...)
	return
}

// SectionID identifies the sections of a Module in the WebAssembly 1.0 (20191205) Binary Format.
//
// Note: these are defined in the wasm package, instead of the binary package, as a key per section is needed regardless
// of format, and deferring to the binary type avoids confusion.
//
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#sections%E2%91%A0
type SectionID = byte

const (
	// SectionIDCustom includes the standard defined NameSection and possibly others not defined in the standard.
	SectionIDCustom SectionID = iota // don't add anything not in https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#sections%E2%91%A0
	SectionIDType
	SectionIDImport
	SectionIDFunction
	SectionIDTable
	SectionIDMemory
	SectionIDGlobal
	SectionIDExport
	SectionIDStart
	SectionIDElement
	SectionIDCode
	SectionIDData
)

// SectionIDName returns the canonical name of a module section.
// https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#sections%E2%91%A0
func SectionIDName(sectionID SectionID) string {
	switch sectionID {
	case SectionIDCustom:
		return "custom"
	case SectionIDType:
		return "type"
	case SectionIDImport:
		return "import"
	case SectionIDFunction:
		return "function"
	case SectionIDTable:
		return "table"
	case SectionIDMemory:
		return "memory"
	case SectionIDGlobal:
		return "global"
	case SectionIDExport:
		return "export"
	case SectionIDStart:
		return "start"
	case SectionIDElement:
		return "element"
	case SectionIDCode:
		return "code"
	case SectionIDData:
		return "data"
	}
	return "unknown"
}

// ValueType is an alias of wasm.ValueType defined to simplify imports.
type ValueType = publicwasm.ValueType

const (
	ValueTypeI32 = publicwasm.ValueTypeI32
	ValueTypeI64 = publicwasm.ValueTypeI64
	ValueTypeF32 = publicwasm.ValueTypeF32
	ValueTypeF64 = publicwasm.ValueTypeF64
)

// ValueTypeName is an alias of wasm.ValueTypeName defined to simplify imports.
func ValueTypeName(t ValueType) string {
	return publicwasm.ValueTypeName(t)
}

// ExternType classifies imports and exports with their respective types.
//
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#import-section%E2%91%A0
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#export-section%E2%91%A0
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#external-types%E2%91%A0
type ExternType = byte

const (
	ExternTypeFunc   ExternType = 0x00
	ExternTypeTable  ExternType = 0x01
	ExternTypeMemory ExternType = 0x02
	ExternTypeGlobal ExternType = 0x03
)

// The below are exported to consolidate parsing behavior for external types.
const (
	// ExternTypeFuncName is the name of the WebAssembly 1.0 (20191205) Text Format field for ExternTypeFunc.
	ExternTypeFuncName = "func"
	// ExternTypeTableName is the name of the WebAssembly 1.0 (20191205) Text Format field for ExternTypeTable.
	ExternTypeTableName = "table"
	// ExternTypeMemoryName is the name of the WebAssembly 1.0 (20191205) Text Format field for ExternTypeMemory.
	ExternTypeMemoryName = "memory"
	// ExternTypeGlobalName is the name of the WebAssembly 1.0 (20191205) Text Format field for ExternTypeGlobal.
	ExternTypeGlobalName = "global"
)

// ExternTypeName returns the name of the WebAssembly 1.0 (20191205) Text Format field of the given type.
//
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#imports⑤
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#exports%E2%91%A4
func ExternTypeName(et ExternType) string {
	switch et {
	case ExternTypeFunc:
		return ExternTypeFuncName
	case ExternTypeTable:
		return ExternTypeTableName
	case ExternTypeMemory:
		return ExternTypeMemoryName
	case ExternTypeGlobal:
		return ExternTypeGlobalName
	}
	return fmt.Sprintf("%#x", et)
}

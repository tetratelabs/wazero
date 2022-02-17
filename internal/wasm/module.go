package internalwasm

import (
	"fmt"
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
// See https://www.w3.org/TR/wasm-core-1/#modules%E2%91%A8
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
	// See https://www.w3.org/TR/wasm-core-1/#types%E2%91%A0%E2%91%A0
	TypeSection []*FunctionType

	// ImportSection contains imported functions, tables, memories or globals required for instantiation
	// (Store.Instantiate).
	//
	// Note: there are no unique constraints relating to the two-level namespace of Import.Module and Import.Name.
	//
	// Note: In the Binary Format, this is SectionIDImport.
	//
	// See https://www.w3.org/TR/wasm-core-1/#import-section%E2%91%A0
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
	// See https://www.w3.org/TR/wasm-core-1/#function-section%E2%91%A0
	FunctionSection []Index

	// TableSection contains each table defined in this module.
	//
	// Note: The table Index namespace begins with imported tables and ends with those defined in this module.
	// For example, if there are two imported tables and one defined in this module, the table Index 3 is defined in
	// this module at TableSection[0].
	//
	// Note: Version 1.0 (MVP) of the WebAssembly spec allows at most one table definition per module, so the length of
	// the TableSection can be zero or one, and can only be one if there is no ImportKindTable.
	//
	// Note: In the Binary Format, this is SectionIDTable.
	//
	// See https://www.w3.org/TR/wasm-core-1/#table-section%E2%91%A0
	TableSection []*TableType

	// MemorySection contains each memory defined in this module.
	//
	// Note: The memory Index namespace begins with imported memories and ends with those defined in this module.
	// For example, if there are two imported memories and one defined in this module, the memory Index 3 is defined in
	// this module at TableSection[0].
	//
	// Note: Version 1.0 (MVP) of the WebAssembly spec allows at most one memory definition per module, so the length of
	// the MemorySection can be zero or one, and can only be one if there is no ImportKindMemory.
	//
	// Note: In the Binary Format, this is SectionIDMemory.
	//
	// See https://www.w3.org/TR/wasm-core-1/#memory-section%E2%91%A0
	MemorySection []*MemoryType

	// GlobalSection contains each global defined in this module.
	//
	// Global indexes are offset by any imported globals because the global index space begins with imports, followed by
	// ones defined in this module. For example, if there are two imported globals and three defined in this module, the
	// global at index 3 is defined in this module at GlobalSection[0].
	//
	// Note: In the Binary Format, this is SectionIDGlobal.
	//
	// See https://www.w3.org/TR/wasm-core-1/#global-section%E2%91%A0
	GlobalSection []*Global

	// ExportSection contains each export defined in this module.
	//
	// Note: In the Binary Format, this is SectionIDExport.
	//
	// See https://www.w3.org/TR/wasm-core-1/#exports%E2%91%A0
	ExportSection map[string]*Export

	// StartSection is the index of a function to call before returning from Store.Instantiate.
	//
	// Note: The index here is not the position in the FunctionSection, rather in the function index namespace, which
	// begins with imported functions.
	//
	// Note: In the Binary Format, this is SectionIDStart.
	//
	// See https://www.w3.org/TR/wasm-core-1/#start-section%E2%91%A0
	StartSection *Index

	// Note: In the Binary Format, this is SectionIDElement.
	ElementSection []*ElementSegment

	// CodeSection is index-correlated with FunctionSection and contains each function's locals and body.
	//
	// Note: In the Binary Format, this is SectionIDCode.
	//
	// See https://www.w3.org/TR/wasm-core-1/#code-section%E2%91%A0
	CodeSection []*Code

	// Note: In the Binary Format, this is SectionIDData.
	DataSection []*DataSegment

	// NameSection is set when the SectionIDCustom "name" was successfully decoded from the binary format.
	//
	// Note: This is the only SectionIDCustom defined in the WebAssembly 1.0 (MVP) Binary Format.
	// Others are skipped as they are not used in wazero.
	//
	// See https://www.w3.org/TR/wasm-core-1/#name-section%E2%91%A0
	// See https://www.w3.org/TR/wasm-core-1/#custom-section%E2%91%A0
	NameSection *NameSection
}

// TypeOfFunction returns the wasm.SectionIDType index for the given function namespace index or nil.
// Note: The function index namespace is preceded by imported functions.
func (m *Module) TypeOfFunction(funcIdx Index) *FunctionType {
	typeSectionLength := uint32(len(m.TypeSection))
	if typeSectionLength == 0 {
		return nil
	}
	funcImportCount := Index(0)
	for i, im := range m.ImportSection {
		if im.Kind == ImportKindFunc {
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

// Index is the offset in an index namespace, not necessarily an absolute position in a Module section. This is because
// index namespaces are often preceded by a corresponding type in the Module.ImportSection.
//
// For example, the function index namespace starts with any ImportKindFunc in the Module.TypeSection followed by the
// Module.FunctionSection
//
// See https://www.w3.org/TR/wasm-core-1/#binary-index
type Index = uint32

// FunctionType is a possibly empty function signature.
//
// See https://www.w3.org/TR/wasm-core-1/#function-types%E2%91%A0
type FunctionType struct {
	// Params are the possibly empty sequence of value types accepted by a function with this signature.
	Params []ValueType

	// Results are the possibly empty sequence of value types returned by a function with this signature.
	//
	// Note: In WebAssembly 1.0 (MVP), there can be at most one result.
	// See https://www.w3.org/TR/wasm-core-1/#result-types%E2%91%A0
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

// Import is the binary representation of an import indicated by Kind
// See https://www.w3.org/TR/wasm-core-1/#binary-import
type Import struct {
	Kind ImportKind
	// Module is the possibly empty primary namespace of this import
	Module string
	// Module is the possibly empty secondary namespace of this import
	Name string
	// DescFunc is the index in Module.TypeSection when Kind equals ImportKindFunc
	DescFunc Index
	// DescTable is the inlined TableType when Kind equals ImportKindTable
	DescTable *TableType
	// DescMem is the inlined MemoryType when Kind equals ImportKindMemory
	DescMem *MemoryType
	// DescGlobal is the inlined GlobalType when Kind equals ImportKindGlobal
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

// Export is the binary representation of an export indicated by Kind
// See https://www.w3.org/TR/wasm-core-1/#binary-export
type Export struct {
	Kind ExportKind
	// Name is what the host refers to this definition as.
	Name string
	// Index is the index of the definition to export, the index namespace is by Kind
	// Ex. If ExportKindFunc, this is an index to ModuleInstance.Functions
	Index Index // TODO: can you export an import? If so, rewrite the doc ^^ to say "index namespace"
}

type ElementSegment struct {
	TableIndex Index
	OffsetExpr *ConstantExpression
	Init       []uint32
}

// Code is an entry in the Module.CodeSection containing the locals and body of the function.
// See https://www.w3.org/TR/wasm-core-1/#binary-code
type Code struct {
	// LocalTypes are any function-scoped variables in insertion order.
	// See https://www.w3.org/TR/wasm-core-1/#binary-local
	LocalTypes []ValueType
	// Body is a sequence of expressions ending in OpcodeEnd
	// See https://www.w3.org/TR/wasm-core-1/#binary-expr
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
// See https://www.w3.org/TR/wasm-core-1/#name-section%E2%91%A0
type NameSection struct {
	// ModuleName is the symbolic identifier for a module. Ex. math
	//
	// Note: This can be empty for any reason including configuration.
	ModuleName string

	// FunctionNames is an association of a function index to its symbolic identifier. Ex. add
	//
	// * the key (idx) is in the function namespace, where module defined functions are preceded by imported ones.
	// See https://www.w3.org/TR/wasm-core-1/#functions%E2%91%A7
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
// ImportKindFunc in the Module.TypeSection followed by the Module.FunctionSection
//
// Note: NameMap is unique by NameAssoc.Index, but NameAssoc.Name needn't be unique.
// Note: When encoding in the Binary format, this must be ordered by NameAssoc.Index
// See https://www.w3.org/TR/wasm-core-1/#binary-namemap
type NameMap []*NameAssoc

type NameAssoc struct {
	Index Index
	Name  string
}

// IndirectNameMap associates an index with an association of names.
//
// Note: IndirectNameMap is unique by NameMapAssoc.Index, but NameMapAssoc.NameMap needn't be unique.
// Note: When encoding in the Binary format, this must be ordered by NameMapAssoc.Index
// https://www.w3.org/TR/wasm-core-1/#binary-indirectnamemap
type IndirectNameMap []*NameMapAssoc

type NameMapAssoc struct {
	Index   Index
	NameMap NameMap
}

// allDeclarations returns all declarations for functions, globals, memories and tables in a module including imported ones.
func (m *Module) allDeclarations() (functions []Index, globals []*GlobalType, memories []*MemoryType, tables []*TableType) {
	for _, imp := range m.ImportSection {
		switch imp.Kind {
		case ImportKindFunc:
			functions = append(functions, imp.DescFunc)
		case ImportKindGlobal:
			globals = append(globals, imp.DescGlobal)
		case ImportKindMemory:
			memories = append(memories, imp.DescMem)
		case ImportKindTable:
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

// SectionElementCount returns the count of elements in a given section ID
//
// For example...
// * SectionIDType returns the count of FunctionType
// * SectionIDCustom returns one if the NameSection is present
// * SectionIDExport returns the count of unique export names
func (m *Module) SectionElementCount(sectionID SectionID) uint32 { // element as in vector elements!
	switch sectionID {
	case SectionIDCustom:
		if m.NameSection != nil {
			return 1
		}
		return 0
	case SectionIDType:
		return uint32(len(m.TypeSection))
	case SectionIDImport:
		return uint32(len(m.ImportSection))
	case SectionIDFunction:
		return uint32(len(m.FunctionSection))
	case SectionIDTable:
		return uint32(len(m.TableSection))
	case SectionIDMemory:
		return uint32(len(m.MemorySection))
	case SectionIDGlobal:
		return uint32(len(m.GlobalSection))
	case SectionIDExport:
		return uint32(len(m.ExportSection))
	case SectionIDStart:
		if m.StartSection != nil {
			return 1
		}
		return 0
	case SectionIDElement:
		return uint32(len(m.ElementSection))
	case SectionIDCode:
		return uint32(len(m.CodeSection))
	case SectionIDData:
		return uint32(len(m.DataSection))
	default:
		panic(fmt.Errorf("BUG: unknown section: %d", sectionID))
	}
}

// SectionID identifies the sections of a Module in the WebAssembly 1.0 (MVP) Binary Format.
//
// Note: these are defined in the wasm package, instead of the binary package, as a key per section is needed regardless
// of format, and deferring to the binary type avoids confusion.
//
// See https://www.w3.org/TR/wasm-core-1/#sections%E2%91%A0
type SectionID = byte

const (
	// SectionIDCustom includes the standard defined NameSection and possibly others not defined in the standard.
	SectionIDCustom SectionID = iota // don't add anything not in https://www.w3.org/TR/wasm-core-1/#sections%E2%91%A0
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
// https://www.w3.org/TR/wasm-core-1/#sections%E2%91%A0
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

// ValueType is the binary encoding of a type such as i32
// See https://www.w3.org/TR/wasm-core-1/#binary-valtype
//
// Note: This is a type alias as it is easier to encode and decode in the binary format.
type ValueType = byte

const (
	ValueTypeI32 ValueType = 0x7f
	ValueTypeI64 ValueType = 0x7e
	ValueTypeF32 ValueType = 0x7d
	ValueTypeF64 ValueType = 0x7c
)

// ValueTypeName returns the type name of the given ValueType as a string.
// These type names match the names used in the WebAssembly text format.
// Note that ValueTypeName returns "unknown", if an undefined ValueType value is passed.
func ValueTypeName(t ValueType) string {
	switch t {
	case ValueTypeI32:
		return "i32"
	case ValueTypeI64:
		return "i64"
	case ValueTypeF32:
		return "f32"
	case ValueTypeF64:
		return "f64"
	}
	return "unknown"
}

// ImportKind indicates which import description is present
// See https://www.w3.org/TR/wasm-core-1/#import-section%E2%91%A0
type ImportKind = byte

const (
	ImportKindFunc   ImportKind = 0x00
	ImportKindTable  ImportKind = 0x01
	ImportKindMemory ImportKind = 0x02
	ImportKindGlobal ImportKind = 0x03
)

// ExportKind indicates which index Export.Index points to
// See https://www.w3.org/TR/wasm-core-1/#export-section%E2%91%A0
type ExportKind = byte

const (
	ExportKindFunc   ExportKind = 0x00
	ExportKindTable  ExportKind = 0x01
	ExportKindMemory ExportKind = 0x02
	ExportKindGlobal ExportKind = 0x03
)

// ExportKindName returns the canonical name of the exportdesc.
// https://www.w3.org/TR/wasm-core-1/#syntax-exportdesc
func ExportKindName(ek ExportKind) string {
	switch ek {
	case ExportKindFunc:
		return "func"
	case ExportKindTable:
		return "table"
	case ExportKindMemory:
		return "mem"
	case ExportKindGlobal:
		return "global"
	}
	return "unknown"
}

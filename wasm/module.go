package wasm

// DecodeModule parses the configured source into a wasm.Module. This function returns when the source is exhausted or
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
	// See https://www.w3.org/TR/wasm-core-1/#type-section%E2%91%A0
	TypeSection []*FunctionType
	// ImportSection contains imported functions, tables, memories or globals required for instantiation
	// (Store.Instantiate).
	//
	// Note: there are no unique constraints relating to the two-level namespace of Import.Module and Import.Name.
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
	// See https://www.w3.org/TR/wasm-core-1/#function-section%E2%91%A0
	FunctionSection []Index

	// TableSection contains each table defined in this module.
	//
	// Note: The table Index namespace begins with imported tables and ends with those defined in this module.
	// For example, if there are two imported tables and one defined in this module, the table Index 3 is defined in
	// this module at TableSection[0].
	//
	// Note: Version 1.0 (MVP) of the WebAssembly spec allows at most one table definition per module, so the length of
	// the TableSection can be zero or one.
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
	// the MemorySection can be zero or one.
	//
	// See https://www.w3.org/TR/wasm-core-1/#memory-section%E2%91%A0
	MemorySection []*MemoryType

	// GlobalSection contains each global defined in this module.
	//
	// Global indexes are offset by any imported globals because the global index space begins with imports, followed by
	// ones defined in this module. For example, if there are two imported globals and three defined in this module, the
	// global at index 3 is defined in this module at GlobalSection[0].
	//
	// See https://www.w3.org/TR/wasm-core-1/#global-section%E2%91%A0
	GlobalSection []*Global

	ExportSection map[string]*Export

	// StartSection is the index of a function to call before returning from Store.Instantiate.
	//
	// Note: The index here is not the position in the FunctionSection, rather in the function index namespace, which
	// begins with imported functions.
	//
	// See https://www.w3.org/TR/wasm-core-1/#start-section%E2%91%A0
	StartSection *Index

	ElementSection []*ElementSegment

	// CodeSection is index-correlated with FunctionSection and contains each function's locals and body.
	//
	// See https://www.w3.org/TR/wasm-core-1/#code-section%E2%91%A0
	CodeSection []*Code

	DataSection []*DataSegment

	// NameSection is set when the custom section "name" was successfully decoded from the binary format.
	//
	// Note: This is the only custom section defined in the WebAssembly 1.0 (MVP) Binary Format. Others are in
	// CustomSections
	//
	// See https://www.w3.org/TR/wasm-core-1/#name-section%E2%91%A0
	NameSection *NameSection

	// CustomSections is set when at least one non-standard, or otherwise unsupported custom section was found in the
	// binary format.
	//
	// Note: This never contains a "name" because that is standard and parsed into the NameSection.
	//
	// See https://www.w3.org/TR/wasm-core-1/#custom-section%E2%91%A0
	CustomSections map[string][]byte
}

// Index is the offset in an index namespace, not necessarily an absolute position in a Module section. This is because
// index namespaces are often preceded by a corresponding type in the Module.ImportSection.
//
// For example, the function index namespace starts with any ImportKindFunc in the Module.TypeSection followed by the
// Module.FunctionSection
//
// See https://www.w3.org/TR/wasm-core-1/#binary-index
type Index = uint32

type FunctionType struct {
	Params, Results []ValueType
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

func valueTypeName(t ValueType) (ret string) {
	switch t {
	case ValueTypeI32:
		ret = "i32"
	case ValueTypeI64:
		ret = "i64"
	case ValueTypeF32:
		ret = "f32"
	case ValueTypeF64:
		ret = "f64"
	}
	return
}

func (t *FunctionType) String() (ret string) {
	for _, b := range t.Params {
		ret += valueTypeName(b)
	}
	if len(t.Params) == 0 {
		ret += "null"
	}
	ret += "_"
	for _, b := range t.Results {
		ret += valueTypeName(b)
	}
	if len(t.Results) == 0 {
		ret += "null"
	}
	return
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

// ExportKind indicates which index Export.Index points to
// See https://www.w3.org/TR/wasm-core-1/#export-section%E2%91%A0
type ExportKind = byte

const (
	ExportKindFunc   ExportKind = 0x00
	ExportKindTable  ExportKind = 0x01
	ExportKindMemory ExportKind = 0x02
	ExportKindGlobal ExportKind = 0x03
)

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

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
	// Function indexes are offset by any imported functions because the function index space begins with imports,
	// followed by ones defined in this module. Moreover, the FunctionSection is index correlated with the CodeSection.
	//
	// For example, if there are two imported functions and three defined in this module, we expect the CodeSection to
	// have a length of three and the function at index 3 to be defined in this module. Its type would be at
	// TypeSection[FunctionSection[0]], while its locals and body are at CodeSection[0].
	//
	// See https://www.w3.org/TR/wasm-core-1/#function-section%E2%91%A0
	FunctionSection []uint32
	// TableSection contains each table defined in this module.
	//
	// Table indexes are offset by any imported tables because the table index space begins with imports, followed by
	// ones defined in this module. For example, if there are two imported tables and one defined in this module, the
	// table at index 3 is defined in this module at TableSection[0].
	//
	// Note: Version 1.0 (MVP) of the WebAssembly spec allows at most one table definition per module, so the length of
	// the TableSection can be zero or one.
	//
	// See https://www.w3.org/TR/wasm-core-1/#table-section%E2%91%A0
	TableSection []*TableType
	// MemorySection contains each memory defined in this module.
	//
	// Memory indexes are offset by any imported memories because the memory index space begins with imports, followed
	// by ones defined in this module. For example, if there are two imported memories and one defined in this module,
	// the memory at index 3 is defined in this module at MemorySection[0].
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
	// Note: The function index space begins with any ImportKindFunc in ImportSection, then the FunctionSection.
	// For example, if there are two imported functions and three defined in this module, the index space is five.
	// Note: This is a pointer to avoid conflating no start section with the valid index zero.
	//
	// See https://www.w3.org/TR/wasm-core-1/#start-section%E2%91%A0
	StartSection   *uint32
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
	DescFunc uint32
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
	Index uint32
}

type ElementSegment struct {
	TableIndex uint32
	OffsetExpr *ConstantExpression
	Init       []uint32
}

type Code struct {
	NumLocals  uint32
	LocalTypes []ValueType
	Body       []byte
}

type DataSegment struct {
	MemoryIndex      uint32 // supposed to be zero
	OffsetExpression *ConstantExpression
	Init             []byte
}

// NameSection represent the known custom name subsections defined in the WebAssembly Binary Format
// See https://www.w3.org/TR/wasm-core-1/#name-section%E2%91%A0
// See https://github.com/tetratelabs/wazero/issues/134 about adding this to Module
type NameSection struct {
	// ModuleName is the symbolic identifier for a module. Ex. math
	// The corresponding subsection is subsectionIDModuleName.
	ModuleName string
	// FunctionNames is an association of a function index to its symbolic identifier. Ex. add
	// The corresponding subsection is subsectionIDFunctionNames.
	//
	// * the key (idx) is in the function namespace, where module defined functions are preceded by imported ones.
	//
	// Ex. Assuming the below text format is the second import, you would expect FunctionNames[1] = "mul"
	//	(import "Math" "Mul" (func $mul (param $x f32) (param $y f32) (result f32)))
	//
	// Note: FunctionNames are a map because the specification requires function indices to be unique. These are sorted
	// during EncodeData
	FunctionNames map[uint32]string

	// LocalNames is an association of a function index to any locals which have a symbolic identifier. Ex. add x
	// The corresponding subsection is subsectionIDLocalNames.
	//
	// * the key (funcIndex) is in the function namespace, where module defined functions are preceded by imported ones.
	// * the second key (idx) is in the local namespace, where parameters precede any function locals.
	//
	// Ex. Assuming the below text format is the second import, you would expect LocalNames[1][1] = "y"
	//	(import "Math" "Mul" (func $mul (param $x f32) (param $y f32) (result f32)))
	//
	// Note: LocalNames are a map because the specification requires both function and local indices to be unique. These
	// are sorted during EncodeData
	LocalNames map[uint32]map[uint32]string
}

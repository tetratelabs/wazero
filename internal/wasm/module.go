package wasm

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/ieee754"
	"github.com/tetratelabs/wazero/internal/leb128"
)

// DecodeModule parses the configured source into a Module. This function returns when the source is exhausted or
// an error occurs. The result can be initialized for use via Store.Instantiate.
//
// Here's a description of the return values:
// * result is the module parsed or nil on error
// * err is a FormatError invoking the parser, dangling block comments or unexpected characters.
// See binary.DecodeModule and text.DecodeModule
type DecodeModule func(source []byte, enabledFeatures Features, memoryMaxPages uint32) (result *Module, err error)

// EncodeModule encodes the given module into a byte slice depending on the format of the implementation.
// See binary.EncodeModule
type EncodeModule func(m *Module) (bytes []byte)

// Module is a WebAssembly binary representation.
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#modules%E2%91%A8
//
// Differences from the specification:
// * NameSection is the only key ("name") decoded from the SectionIDCustom.
// * ExportSection is represented as a map for lookup convenience.
// * HostFunctionSection is a custom section that contains any go `func`s. It may be present when CodeSection is not.
type Module struct {
	// TypeSection contains the unique FunctionType of functions imported or defined in this module.
	//
	// Note: Currently, there is no type ambiguity in the index as WebAssembly 1.0 only defines function type.
	// In the future, other types may be introduced to support Features such as module linking.
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
	TableSection *Table

	// MemorySection contains each memory defined in this module.
	//
	// Note: The memory Index namespace begins with imported memories and ends with those defined in this module.
	// For example, if there are two imported memories and one defined in this module, the memory Index 3 is defined in
	// this module at TableSection[0].
	//
	// Note: Version 1.0 (20191205) of the WebAssembly spec allows at most one memory definition per module, so the
	// length of the MemorySection can be zero or one, and can only be one if there is no imported memory.
	//
	// Note: In the Binary Format, this is SectionIDMemory.
	//
	// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#memory-section%E2%91%A0
	MemorySection *Memory

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
	// When present, the HostFunctionSection must be nil.
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

	// HostFunctionSection is index-correlated with FunctionSection and contains a host function defined in Go.
	// When present, the CodeSection must be nil.
	//
	// Note: This section currently has no serialization format, so is not encodable.
	// See https://www.w3.org/TR/wasm-core-1/#host-functions%E2%91%A2
	HostFunctionSection []*reflect.Value

	// elementSegments are built on Validate when SectionIDElement is non-empty and all inputs are valid.
	//
	// Note: elementSegments retain Module.ElementSection order. Since an ElementSegment can overlap with another, order
	// preservation ensures a consistent initialization result.
	// See https://www.w3.org/TR/wasm-core-1/#table-instances%E2%91%A0
	validatedElementSegments []*validatedElementSegment
}

// The wazero specific limitation described at RATIONALE.md.
// TL;DR; We multiply by 8 (to get offsets in bytes) and the multiplication result must be less than 32bit max
const (
	MaximumGlobals       = uint32(1 << 27)
	MaximumFunctionIndex = uint32(1 << 27)
)

// TypeOfFunction returns the wasm.SectionIDType index for the given function namespace index or nil.
// Note: The function index namespace is preceded by imported functions.
// TODO: Returning nil should be impossible when decode results are validated. Validate decode before back-filling tests.
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

func (m *Module) Validate(enabledFeatures Features) error {
	if err := m.validateStartSection(); err != nil {
		return err
	}

	if m.SectionElementCount(SectionIDCode) > 0 && m.SectionElementCount(SectionIDHostFunction) > 0 {
		return errors.New("cannot mix functions and host functions in the same module")
	}

	functions, globals, memory, table, err := m.allDeclarations()
	if err != nil {
		return err
	}

	if err = m.validateImports(enabledFeatures); err != nil {
		return err
	}

	if err = m.validateGlobals(globals, MaximumGlobals); err != nil {
		return err
	}

	if err = m.validateMemory(memory, globals); err != nil {
		return err
	}

	if m.CodeSection != nil {
		if err = m.validateFunctions(enabledFeatures, functions, globals, memory, table, MaximumFunctionIndex); err != nil {
			return err
		}
	} else {
		if err = m.validateHostFunctions(); err != nil {
			return err
		}
	}

	if _, err = m.validateTable(); err != nil {
		return err
	}

	if err = m.validateExports(enabledFeatures, functions, globals, memory, table); err != nil {
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
			return fmt.Errorf("invalid start function: func[%d] has an invalid type", startIndex)
		}
		if len(ft.Params) > 0 || len(ft.Results) > 0 {
			return fmt.Errorf("invalid start function: func[%d] must have an empty (nullary) signature: %s", startIndex, ft)
		}
	}
	return nil
}

func (m *Module) validateGlobals(globals []*GlobalType, maxGlobals uint32) error {
	if uint32(len(globals)) > maxGlobals {
		return fmt.Errorf("too many globals in a module")
	}

	// Global initialization constant expression can only reference the imported globals.
	// See the note on https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#constant-expressions%E2%91%A0
	importedGlobals := globals[:m.ImportGlobalCount()]
	for _, g := range m.GlobalSection {
		if err := validateConstExpression(importedGlobals, g.Init, g.Type.ValType); err != nil {
			return err
		}
	}
	return nil
}

func (m *Module) validateFunctions(enabledFeatures Features, functions []Index, globals []*GlobalType, memory *Memory, table *Table, maximumFunctionIndex uint32) error {
	if uint32(len(functions)) > maximumFunctionIndex {
		return fmt.Errorf("too many functions in a store")
	}

	functionCount := m.SectionElementCount(SectionIDFunction)
	codeCount := m.SectionElementCount(SectionIDCode)
	if functionCount == 0 && codeCount == 0 {
		return nil
	}

	typeCount := m.SectionElementCount(SectionIDType)
	if codeCount != functionCount {
		return fmt.Errorf("code count (%d) != function count (%d)", codeCount, functionCount)
	}

	// The wazero specific limitation described at RATIONALE.md.
	const maximumValuesOnStack = 1 << 27
	for idx, typeIndex := range m.FunctionSection {
		if typeIndex >= typeCount {
			return fmt.Errorf("invalid %s: type section index %d out of range", m.funcDesc(SectionIDFunction, Index(idx)), typeIndex)
		}

		if err := validateFunction(
			enabledFeatures,
			m.TypeSection[typeIndex],
			m.CodeSection[idx].Body,
			m.CodeSection[idx].LocalTypes,
			functions, globals, memory, table, m.TypeSection, maximumValuesOnStack); err != nil {

			return fmt.Errorf("invalid %s: %w", m.funcDesc(SectionIDFunction, Index(idx)), err)
		}
	}
	return nil
}

func (m *Module) funcDesc(sectionID SectionID, sectionIndex Index) string {
	// Try to improve the error message by collecting any exports:
	var exportNames []string
	funcIdx := sectionIndex + m.importCount(ExternTypeFunc)
	for _, e := range m.ExportSection {
		if e.Index == funcIdx && e.Type == ExternTypeFunc {
			exportNames = append(exportNames, fmt.Sprintf("%q", e.Name))
		}
	}
	sectionIDName := SectionIDName(sectionID)
	if exportNames == nil {
		return fmt.Sprintf("%s[%d]", sectionIDName, sectionIndex)
	}
	sort.Strings(exportNames) // go map keys do not iterate consistently
	return fmt.Sprintf("%s[%d] export[%s]", sectionIDName, sectionIndex, strings.Join(exportNames, ","))
}

func (m *Module) validateMemory(memory *Memory, globals []*GlobalType) error {
	if len(m.DataSection) > 0 && memory == nil {
		return fmt.Errorf("unknown memory")
	}

	for _, d := range m.DataSection {
		if err := validateConstExpression(globals, d.OffsetExpression, ValueTypeI32); err != nil {
			return fmt.Errorf("calculate offset: %w", err)
		}
	}
	return nil
}

func (m *Module) validateImports(enabledFeatures Features) error {
	for _, i := range m.ImportSection {
		switch i.Type {
		case ExternTypeGlobal:
			if !i.DescGlobal.Mutable {
				continue
			}
			if err := enabledFeatures.Require(FeatureMutableGlobal); err != nil {
				return fmt.Errorf("invalid import[%q.%q] global: %w", i.Module, i.Name, err)
			}
		}
	}
	return nil
}

func (m *Module) validateExports(enabledFeatures Features, functions []Index, globals []*GlobalType, memory *Memory, table *Table) error {
	for name, exp := range m.ExportSection {
		index := exp.Index
		switch exp.Type {
		case ExternTypeFunc:
			if index >= uint32(len(functions)) {
				return fmt.Errorf("unknown function for export[%q]", name)
			}
		case ExternTypeGlobal:
			if index >= uint32(len(globals)) {
				return fmt.Errorf("unknown global for export[%q]", name)
			}
			if !globals[index].Mutable {
				continue
			}
			if err := enabledFeatures.Require(FeatureMutableGlobal); err != nil {
				return fmt.Errorf("invalid export[%q] global[%d]: %w", name, index, err)
			}
		case ExternTypeMemory:
			if index > 0 || memory == nil {
				return fmt.Errorf("memory for export[%q] out of range", name)
			}
		case ExternTypeTable:
			if index > 0 || table == nil {
				return fmt.Errorf("table for export[%q] out of range", name)
			}
		}
	}
	return nil
}

func validateConstExpression(globals []*GlobalType, expr *ConstantExpression, expectedType ValueType) (err error) {
	var actualType ValueType
	r := bytes.NewReader(expr.Data)
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

func (m *Module) buildGlobals(importedGlobals []*GlobalInstance) (globals []*GlobalInstance) {
	for _, gs := range m.GlobalSection {
		var gv uint64
		switch v := executeConstExpression(importedGlobals, gs.Init).(type) {
		case int32:
			gv = uint64(v)
		case int64:
			gv = uint64(v)
		case float32:
			gv = api.EncodeF32(v)
		case float64:
			gv = api.EncodeF64(v)
		}
		globals = append(globals, &GlobalInstance{Type: gs.Type, Val: gv})
	}
	return
}

func (m *Module) buildFunctions() (functions []*FunctionInstance) {
	var functionNames NameMap
	if m.NameSection != nil {
		functionNames = m.NameSection.FunctionNames
	}

	importCount := m.ImportFuncCount()
	n, nLen := 0, len(functionNames)
	for codeIndex, typeIndex := range m.FunctionSection {
		funcIdx := importCount + uint32(len(functions))
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
			Name:       name,
			Kind:       FunctionKindWasm,
			Type:       m.TypeSection[typeIndex],
			Body:       m.CodeSection[codeIndex].Body,
			LocalTypes: m.CodeSection[codeIndex].LocalTypes,
			Index:      importCount + uint32(codeIndex),
		}
		functions = append(functions, f)
	}
	return
}

func (m *Module) buildMemory() (mem *MemoryInstance) {
	memSec := m.MemorySection
	if memSec != nil {
		mem = &MemoryInstance{
			Buffer: make([]byte, MemoryPagesToBytesNum(memSec.Min)),
			Min:    memSec.Min,
			Max:    memSec.Max,
		}
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

	// string is cached as it is used both for String and key
	string string
}

// EqualsSignature returns true if the function type has the same parameters and results.
func (t *FunctionType) EqualsSignature(params []ValueType, results []ValueType) bool {
	return bytes.Equal(t.Params, params) && bytes.Equal(t.Results, results)
}

// key gets or generates the key for Store.typeIDs. Ex. "i32_v" for one i32 parameter and no (void) result.
func (t *FunctionType) key() string {
	if t.string != "" {
		return t.string
	}
	var ret string
	for _, b := range t.Params {
		ret += ValueTypeName(b)
	}
	if len(t.Params) == 0 {
		ret += "v"
	}
	ret += "_"
	for _, b := range t.Results {
		ret += ValueTypeName(b)
	}
	if len(t.Results) == 0 {
		ret += "v"
	}
	t.string = ret
	return ret
}

// String implements fmt.Stringer.
func (t *FunctionType) String() string {
	return t.key()
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
	// DescTable is the inlined Table when Type equals ExternTypeTable
	DescTable *Table
	// DescMem is the inlined Memory when Type equals ExternTypeMemory
	DescMem *Memory
	// DescGlobal is the inlined GlobalType when Type equals ExternTypeGlobal
	DescGlobal *GlobalType
}

type limitsType struct {
	Min uint32
	Max *uint32
}

// Memory describes the limits of pages (64KB) in a memory.
type Memory struct {
	Min, Max uint32
}

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
func (m *Module) allDeclarations() (functions []Index, globals []*GlobalType, memory *Memory, table *Table, err error) {
	for _, imp := range m.ImportSection {
		switch imp.Type {
		case ExternTypeFunc:
			functions = append(functions, imp.DescFunc)
		case ExternTypeGlobal:
			globals = append(globals, imp.DescGlobal)
		case ExternTypeMemory:
			memory = imp.DescMem
		case ExternTypeTable:
			table = imp.DescTable
		}
	}

	functions = append(functions, m.FunctionSection...)
	for _, g := range m.GlobalSection {
		globals = append(globals, g.Type)
	}
	if m.MemorySection != nil {
		if memory != nil { // shouldn't be possible due to Validate
			err = errors.New("at most one table allowed in module")
			return
		}
		memory = m.MemorySection
	}
	if m.TableSection != nil {
		if table != nil { // shouldn't be possible due to Validate
			err = errors.New("at most one table allowed in module")
			return
		}
		table = m.TableSection
	}
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

// SectionIDHostFunction is a pseudo-section ID for host functions.
//
// Note: This is not defined in the WebAssembly 1.0 (20191205) Binary Format.
const SectionIDHostFunction = SectionID(0xff)

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
	case SectionIDHostFunction:
		return "host_function"
	}
	return "unknown"
}

// ValueType is an alias of api.ValueType defined to simplify imports.
type ValueType = api.ValueType

const (
	ValueTypeI32 = api.ValueTypeI32
	ValueTypeI64 = api.ValueTypeI64
	ValueTypeF32 = api.ValueTypeF32
	ValueTypeF64 = api.ValueTypeF64
)

// ValueTypeName is an alias of api.ValueTypeName defined to simplify imports.
func ValueTypeName(t ValueType) string {
	return api.ValueTypeName(t)
}

// ElemType is fixed to ElemTypeFuncref until post 20191205 reference type is implemented.
type ElemType = byte

const (
	ElemTypeFuncref ElemType = 0x70
)

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

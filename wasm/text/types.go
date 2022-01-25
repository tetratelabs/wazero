package text

import (
	"fmt"

	"github.com/tetratelabs/wazero/wasm"
)

// module corresponds to the text format of a WebAssembly module, and is an intermediate representation prior to
// wasm.Module. This is primarily needed to resolve symbolic indexes like "$main" to raw numeric ones.
//
// Note: nothing is required per specification. Ex `(module)` is valid!
//
// See https://www.w3.org/TR/wasm-core-1/#functions%E2%91%A7
type module struct {
	// types are the wasm.Module TypeSection
	types []*wasm.FunctionType

	// importFuncs are imports describing functions added in insertion order. Ex (import... (func...))
	importFuncs []*importFunc

	// code are wasm.Module CodeSection, holding the function locals and body
	//
	// Note: the type usage of this function is in module.typeUses at the index in module.funcs offset by the length of
	// module.importFuncs. For example, if this function is module.funcs[2] and there are 3 module.importFuncs. The type use
	// is at module.typeUses[5]
	code []*wasm.Code

	// typeUses comprise the function index namespace of importFuncs followed by funcs
	typeUses []*typeUse

	// exportFuncs are exports describing functions added in insertion order. Ex (export... (func...))
	exportFuncs []*exportFunc

	// startFunction is the index of the function to call during wasm.Store Instantiate. When index.ID is set, it must
	// match a wasm.NameAssoc Name in wasm.NameSection FunctionNames
	//
	// See https://www.w3.org/TR/wasm-core-1/#start-function%E2%91%A4
	startFunction *index

	// names are the wasm.Module NameSection
	//
	// * ModuleName: ex. "test" if (module $test)
	// * FunctionNames: nil when no importFunc or function had a name
	// * LocalNames: nil when no importFuncs or function had named (param) fields.
	//
	// Note: LocalNames will be incomplete until the end of parsing because types can be declared after a function that
	// uses them. typeUses are analyzed later for this reason.
	// See https://www.w3.org/TR/wasm-core-1/#modules%E2%91%A0%E2%91%A2
	names *wasm.NameSection
}

type inlinedTypeFunc struct {
	typeFunc *wasm.FunctionType

	// line is the line in the source where the typeFunc was defined.
	line uint32

	// col is the column on the line where the typeFunc was defined.
	col uint32
}

// index is symbolic ID, such as "$main", or its equivalent numeric value, such as "2".
//
// Note: line and col used for lazy validation of index. These are attached to an error if later found to be invalid
// (ex an unknown function or out-of-bound index).
//
// https://www.w3.org/TR/wasm-core-1/#indices%E2%91%A4
type index struct {
	// ID is set when its corresponding token is tokenID to a symbolic identifier index. Ex. main
	//
	// Note: This must be checked for a corresponding index element name, as it is possible it doesn't exist.
	// Ex. This is $t0 from (import "Math" "PI" (func (type $t0))), but (type $t0 (func ...)) does not exist.
	ID string

	// numeric is set when its corresponding token is tokenUN is a wasm.Index. Ex. 3
	//
	// Note: To avoid conflating unset with the valid index zero, only read this value when ID is unset.
	// Note: This must be checked for range as there's a possible out-of-bonds condition.
	// Ex. This is 32 from (import "Math" "PI" (func (type 32))), but there are only 10 types defined in the module.
	numeric wasm.Index

	// line is the line in the source where the index was defined.
	line uint32

	// col is the column on the line where the index was defined.
	col uint32
}

// idContext is an association of symbolic IDs to numeric indices.
//
// The Web Assembly Text Format allows use of symbolic identifiers, ex "$main", instead of numeric indices for most
// sections, notably types, functions and parameters. For example, if a function is defined with the tokenID "$main",
// the start section can use that symbolic ID instead of its numeric offset in the code section. These IDs require two
// features, uniqueness and lookup, as implemented with a map. The key is stripped of the leading '$' to match other
// tools, as described in stripDollar
//
// See https://www.w3.org/TR/wasm-core-1/#text-context
type idContext map[string]wasm.Index

// setID ensures the given tokenID is unique within this context and raises an error if not. The resulting mapping is
// stripped of the leading '$' to match other tools, as described in stripDollar.
func (ctx idContext) setID(idToken []byte, idx uint32) (string, error) {
	id := string(stripDollar(idToken))
	if _, ok := ctx[id]; ok {
		return id, fmt.Errorf("duplicate ID %s", idToken)
	}
	ctx[id] = idx
	return id, nil
}

// importFunc corresponds to the text format of a WebAssembly function import.
//
// Note: the type usage of this import is at module.typeUses the same index as module.importFuncs
// Note: nothing is required per specification. Ex `(import "" "" (func))` is valid!
//
// See https://www.w3.org/TR/wasm-core-1/#imports%E2%91%A0
type importFunc struct {
	// module is the possibly empty module name to import. Ex. "" or "Math"
	//
	// Note: This is not necessarily the module.name
	module string

	// name is the possibly empty entity name to import. Ex. "" or "PI"
	//
	// Note this is not necessarily a wasm.NameAssoc Name in wasm.NameSection FunctionNames
	name string
}

// typeUse corresponds to the text format of an indexed or inlined type signature.
//
// Note: nothing is required per specification. Ex `(func)` is valid!
//
// See https://www.w3.org/TR/wasm-core-1/#type-uses%E2%91%A0
type typeUse struct {
	// typeIndex is the optional index in module.types for the function signature. If index.ID is set, it must match
	// typeFunc.name.
	//
	// See https://www.w3.org/TR/wasm-core-1/#text-typeuse
	typeIndex *index

	// typeInlined is set if there are any "param" or "result" fields. When set and typeIndex is also set, the signature
	// in module.types must exist and match this.
	//
	// See https://www.w3.org/TR/wasm-core-1/#abbreviations%E2%91%A6
	typeInlined *inlinedTypeFunc
}

// exportFunc corresponds to the text format of a WebAssembly function export.
//
// See https://www.w3.org/TR/wasm-core-1/#exports%E2%91%A0
type exportFunc struct {
	// name is the possibly empty entity name to export. Ex. "" or "PI"
	//
	// Note: This is not necessarily the same in a wasm.NameSection FunctionNames
	name string

	// exportIndex is the zero-based index in module.exports. This is needed because exports are not always functions.
	exportIndex wasm.Index

	// funcIndex is the index of the function to export. When index.ID is set, it must match a wasm.NameAssoc Name in
	// wasm.NameSection FunctionNames
	//
	// See https://www.w3.org/TR/wasm-core-1/#start-function%E2%91%A4
	funcIndex *index
}

// FormatError allows control over the format of errors parsing the WebAssembly Text Format.
type FormatError struct {
	// Line is the source line number determined by unescaped '\n' characters of the error or EOF
	Line uint32
	// Col is the UTF-8 column number of the error or EOF
	Col uint32
	// Context is where symbolically the error occurred. Ex "imports[1].func"
	Context string
	cause   error
}

func (e *FormatError) Error() string {
	if e.Context == "" { // error starting the file
		return fmt.Sprintf("%d:%d: %v", e.Line, e.Col, e.cause)
	}
	return fmt.Sprintf("%d:%d: %v in %s", e.Line, e.Col, e.cause, e.Context)
}

func (e *FormatError) Unwrap() error {
	return e.cause
}

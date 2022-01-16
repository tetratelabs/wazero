package wat

import (
	"bytes"
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
	// name is optional. For example, "test".
	// See https://www.w3.org/TR/wasm-core-1/#modules%E2%91%A0%E2%91%A2
	//
	// Note: The name may also be stored in the wasm.Module CustomSection under the key "name" subsection 0.
	// See https://www.w3.org/TR/wasm-core-1/#binary-namesec
	name string

	// types are the unique function signatures of this module added in insertion order. Ex. (type... (func...))
	//
	// Note: Currently, there is no type ambiguity in the index as WebAssembly 1.0 only defines function type.
	// In the future, other types may be introduced to support features such as module linking.
	//
	// See https://www.w3.org/TR/wasm-core-1/#types%E2%91%A0%E2%91%A0
	types []*typeFunc

	// typeParamNames include any parameter names for the corresponding index of types.
	typeParamNames []*typeParamNames

	// importFuncs are imports describing functions added in insertion order. Ex (import... (func...))
	importFuncs []*importFunc

	// startFunction is the index of the function to call during wasm.Store Instantiate. When a tokenID, this must match
	// importFunc.funcName.
	//
	// See https://www.w3.org/TR/wasm-core-1/#start-function%E2%91%A4
	startFunction *index
}

// typeParamNames include any parameter names for the corresponding index of module.types.
type typeParamNames struct {
	index      uint32
	paramNames paramNames
}

type inlinedTypeFunc struct {
	typeFunc *typeFunc

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
	// ID is set when its corresponding token is tokenID to a symbolic identifier index. Ex. $main
	//
	// Note: This must be checked for a corresponding index element name, as it is possible it doesn't exist.
	// Ex. This is $t0 from (import "Math" "PI" (func (type $t0))), but (type $t0 (func ...)) does not exist.
	ID string

	// numeric is set when its corresponding token is tokenUN to a numeric index. Ex. 3
	//
	// Note: To avoid conflating unset with the valid index zero, only read this value when ID is unset.
	// Note: This must be checked for range as there's a possible out-of-bonds condition.
	// Ex. This is 32 from (import "Math" "PI" (func (type 32))), but there are only 10 types defined in the module.
	numeric uint32

	// line is the line in the source where the index was defined.
	line uint32

	// col is the column on the line where the index was defined.
	col uint32
}

// typeFunc corresponds to the text format of a WebAssembly type use.
//
// Note: nothing is required per specification. Ex `(type (func))` is valid!
//
// See https://www.w3.org/TR/wasm-core-1/#text-functype
type typeFunc struct {
	// name is only set when explicitly defined in module.types. Ex. v_v
	//
	// name is only used for debugging. At runtime, types are called based on raw numeric index. The type index space
	// begins those explicitly defined in module.types, followed by any inlined ones.
	name string // TODO: presumably, this must be unique as it is a symbolic identifier?

	// params are the possibly empty sequence of value types accepted by a function with this signature.
	//
	// Note: In WebAssembly 1.0 (MVP), there can be at most one result.
	// See https://www.w3.org/TR/wasm-core-1/#result-types%E2%91%A0
	params []wasm.ValueType

	// result is the value type of the signature or zero if there is none.
	//
	// Note: We use this shortcut instead of a slice because in WebAssembly 1.0 (MVP), there can be at most one result.
	// See https://www.w3.org/TR/wasm-core-1/#result-types%E2%91%A0
	result wasm.ValueType
}

// funcTypeEquals allows you to compare signatures ignoring names
func funcTypeEquals(t *typeFunc, params []wasm.ValueType, result wasm.ValueType) bool {
	return bytes.Equal(t.params, params) && t.result == result
}

// paramNames are the possibly empty association of names that correspond with params. The index is to params and
// the name will never be empty. Ex. x is the name of (param $x i32)
//
// paramNames are only used for debugging. At runtime, parameters are called based on raw numeric index.
//
// Note: paramNames may be stored in the wasm.Module CustomSection under the key "name" subsection 2 (locals). For
// example, `wat2wasm --debug-names` will do this.
// See https://www.w3.org/TR/wasm-core-1/#binary-namesec
type paramNames []*paramNameIndex // type is only to re-use documentation

type paramNameIndex struct {
	// index is in currentParams, not necessarily related to currentParamField due to abbreviated format.
	index uint32
	// name is the tokenID of the param field.
	name []byte
}

// importFunc corresponds to the text format of a WebAssembly function import.
//
// Note: nothing is required per specification. Ex `(import "" "" (func))` is valid!
//
// See https://www.w3.org/TR/wasm-core-1/#imports%E2%91%A0
type importFunc struct {
	// importIndex is the zero-based index in module.imports. This is needed because imports are not always functions.
	importIndex uint32

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

	// paramNames include any parameter names for typeInlined.
	paramNames paramNames

	// module is the possibly empty module name to import. Ex. "" or "Math"
	//
	// Note: This is not necessarily the module.name
	module string

	// name is the possibly empty entity name to import. Ex. "" or "PI"
	//
	// Note: This is not necessarily the funcName
	name string

	// funcName is optional. Ex. main
	//
	// funcName is only used for debugging. At runtime, functions are called based on raw numeric index. The function
	// index space begins with imported functions, followed by any defined in this module.
	// See https://www.w3.org/TR/wasm-core-1/#functions%E2%91%A7
	//
	// Note: funcName may be stored in the wasm.Module CustomSection under the key "name" subsection 1. For example,
	// `wat2wasm --debug-names` will do this.
	// See https://www.w3.org/TR/wasm-core-1/#binary-namesec
	funcName string // TODO: presumably, this must be unique as it is a symbolic identifier?
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

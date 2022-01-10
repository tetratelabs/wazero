package wat

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
	// name is optional and starts with '$'. For example, "$test".
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
	typeFuncs []*typeFunc

	// importFuncs are imports describing functions added in insertion order. Ex (import... (func...))
	importFuncs []*importFunc

	startFunction *startFunction
}

// startFunction is the function to call during wasm.Store Instantiate.
//
// Note: line and col used for lazy validation of index. These are attached to an error if later found to be invalid
// (ex an unknown function or out-of-bound index).
//
// See https://www.w3.org/TR/wasm-core-1/#start-function%E2%91%A4
type startFunction struct {
	// index is a importFunc.name, such as "$main", or its equivalent numeric index in module.importFuncs, such as "2".
	//
	// See wasm.Module StartSection for more.
	index string

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
	// name starts with '$'. For example, "$v_v", and only set when explicitly defined in module.typeFuncs
	//
	// name is only used for debugging. At runtime, types are called based on raw numeric index. The type index space
	// begins those explicitly defined in module.typeFuncs, followed by any inlined ones.
	name string

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

// importFunc corresponds to the text format of a WebAssembly function import.
//
// Note: nothing is required per specification. Ex `(import "" "" (func))` is valid!
//
// See https://www.w3.org/TR/wasm-core-1/#imports%E2%91%A0
type importFunc struct {
	// importIndex is the zero-based index in module.imports. This is needed because imports are not always functions.
	importIndex uint32

	// typeIndex is a importFunc.name, such as "$main", or its equivalent numeric index in module.importFuncs, such as
	// "2". If typeInlined is also present, the signature in module.importFuncs must exist and match that type.
	//
	// See https://www.w3.org/TR/wasm-core-1/#text-typeuse
	typeIndex []byte

	// typeInlined is set if there are any "param" or "result" fields.
	// See https://www.w3.org/TR/wasm-core-1/#abbreviations%E2%91%A6
	typeInlined *typeFunc

	// module is the possibly empty module name to import. Ex. "" or "Math"
	//
	// Note: This is not necessarily the module.name, so it does not need to begin with '$'!
	module string

	// name is the possibly empty entity name to import. Ex. "" or "PI"
	//
	// Note: This is not necessarily the funcName, so it does not need to begin with '$'!
	name string

	// funcName starts with '$'. For example, "$main".
	//
	// funcName is only used for debugging. At runtime, functions are called based on raw numeric index. The function
	// index space begins with imported functions, followed by any defined in this module.
	// See https://www.w3.org/TR/wasm-core-1/#functions%E2%91%A7
	//
	// Note: funcName may be stored in the wasm.Module CustomSection under the key "name" subsection 1. For example,
	// `wat2wasm --debug-names` will do this.
	// See https://www.w3.org/TR/wasm-core-1/#binary-namesec
	funcName string
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

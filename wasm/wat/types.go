package wat

import "fmt"

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

	// imports are added in insertion order.
	imports []*_import

	// startFunction is the function to call during wasm.Store Instantiate. The value is a importFunc.name, such as
	// "$main", or its equivalent raw numeric index, such as "2".
	//
	// Note: When in raw numeric form, this is relative to imports. See wasm.Module StartSection for more.
	//
	// See https://www.w3.org/TR/wasm-core-1/#start-function%E2%91%A4
	startFunction string
}

// importFunc corresponds to the text format of a WebAssembly function import description.
//
// Note: nothing is required per specification. Ex `(func)` is valid!
//
// See https://www.w3.org/TR/wasm-core-1/#imports%E2%91%A0
type importFunc struct {
	// name is optional and starts with '$'. For example, "$main".
	//
	// This name is only used for debugging. At runtime, functions are called based on raw numeric index. The function
	// index space begins with imported functions, followed by any defined in this module.
	// See https://www.w3.org/TR/wasm-core-1/#functions%E2%91%A7
	//
	// Note: The name may also be stored in the wasm.Module CustomSection under the key "name" subsection 1.
	// See https://www.w3.org/TR/wasm-core-1/#binary-namesec
	name string

	// TODO: typeuse https://www.w3.org/TR/wasm-core-1/#text-typeuse
}

// _import corresponds to the text format of a WebAssembly import.
//
// See https://www.w3.org/TR/wasm-core-1/#imports%E2%91%A0
type _import struct { // note: this is named _import because import is reserved in golang
	// module is the possibly empty module name to import. Ex. "" or "Math"
	//
	// Note: This is not necessarily the module.name, so it does not need to begin with '$'!
	module string
	// name is the possibly empty entity name to import. Ex. "" or "PI"
	//
	// Note: This is not necessarily the entity name defined in this module, so it does not need to begin with '$'!
	name string
	// importFunc is set when the "import" field is
	importFunc *importFunc // TODO: oneOf func, table, mem, global
}

// formatError allows control over the format of formatError.Error
type formatError struct {
	// line is the source line number determined by unescaped '\n' characters of the error or EOF
	line int
	// Col is the UTF-8 column number of the error or EOF
	col int
	// Context is where symbolically the error occurred. Ex "imports[1].func"
	context string
	cause   error
}

func (e *formatError) Error() string {
	return fmt.Sprintf("%d:%d: %v in %s", e.line, e.col, e.cause, e.context)
}

func (e *formatError) Unwrap() error {
	return e.cause
}

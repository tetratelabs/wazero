package wat

import "fmt"

// textModule corresponds to the text format of a WebAssembly module, and is an intermediate representation prior to
// wasm.Module.
//
// Note: nothing is required per specification. Ex `(module)` is valid!
//
// See https://www.w3.org/TR/wasm-core-1/#functions%E2%91%A7
type textModule struct {
	// name is optional and starts with '$'. For example, "$test".
	// See https://www.w3.org/TR/wasm-core-1/#modules%E2%91%A0%E2%91%A2
	//
	// Note: The name may also be stored in the wasm.Module CustomSection under the key "name" subsection 0.
	// See https://www.w3.org/TR/wasm-core-1/#binary-namesec
	name string

	// imports are the module textImport added in insertion order.
	imports []*textImport

	// startFunction is the function to call during wasm.Store Instantiate. The value is a textFunc.name, such as "$main",
	// or its equivalent raw numeric index, such as "2".
	//
	// Note: When in raw numeric form, this is relative to Import functions.
	// See https://www.w3.org/TR/wasm-core-1/#start-function%E2%91%A4
	startFunction string
}

// textFunc corresponds to the text format of a WebAssembly textFunc.
//
// Note: nothing is required per specification. Ex `(func)` is valid!
//
// See https://www.w3.org/TR/wasm-core-1/#functions%E2%91%A7
type textFunc struct {
	// name is optional and starts with '$'. For example, "$main".
	//
	// This name is only used for debugging. At runtime, functions are called based on raw numeric index. The function
	// index space begins with imported functions, followed by any defined in this module.
	// See https://www.w3.org/TR/wasm-core-1/#functions%E2%91%A7
	//
	// Note: The name may also be stored in the wasm.Module CustomSection under the key "name" subsection 1.
	// See https://www.w3.org/TR/wasm-core-1/#binary-namesec
	name string
}

// textImport corresponds to the text format of a WebAssembly import.
//
// See https://www.w3.org/TR/wasm-core-1/#imports%E2%91%A0
type textImport struct {
	// module is the possibly empty module name to import. Ex. "" or "Math"
	//
	// Note: This is not necessarily the textModule.name, so it does not need to begin with '$'!
	module string
	// name is the possibly empty entity name to import. Ex. "" or "PI"
	//
	// Note: This is not necessarily the actual entity name (ex. textFunc.name), so it does not need to begin with '$'!
	name string
	desc *textFunc // TODO: oneOf textFunc,textTable,textMem,textGlobal
}

// textFormatError allows control over the format of textFormatError.Error
type textFormatError struct {
	// line is the source line number determined by unescaped '\n' characters of the error or EOF
	line int
	// Col is the UTF-8 column number of the error or EOF
	col int
	// Context is where symbolically the error occurred. Ex "imports[1].desc"
	context string
	cause   error
}

func (e *textFormatError) Error() string {
	return fmt.Sprintf("%d:%d: %v in %s", e.line, e.col, e.cause, e.context)
}

func (e *textFormatError) Unwrap() error {
	return e.cause
}

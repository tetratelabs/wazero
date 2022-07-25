package internal

import (
	"fmt"

	"github.com/tetratelabs/wazero/internal/wasm"
)

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

func unexpectedFieldName(tokenBytes []byte) error {
	return fmt.Errorf("unexpected field: %s", tokenBytes)
}

func expectedField(tok tokenType) error {
	return fmt.Errorf("expected field, but parsed %s", tok)
}

func unexpectedToken(tok tokenType, tokenBytes []byte) error {
	switch tok {
	case tokenLParen, tokenRParen:
		return fmt.Errorf("unexpected '%s'", tok)
	default:
		return fmt.Errorf("unexpected %s: %s", tok, tokenBytes)
	}
}

// importAfterModuleDefined is the failure for the condition "all imports must occur before any regular definition",
// which applies regardless of abbreviation.
//
// Ex. Both of these fail because an import can only be declared when SectionIDFunction is empty.
// `(func) (import "" "" (func))` which is the same as  `(func) (import "" "" (func))`
//
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#modules%E2%91%A0%E2%91%A2
func importAfterModuleDefined(section wasm.SectionID) error {
	return fmt.Errorf("import after module-defined %s", wasm.SectionIDName(section))
}

// moreThanOneInvalidInSection allows enforcement of section size limits.
//
// Ex. All of these fail because they result in two memories.
// * `(module (memory 1) (memory 1))`
// * `(module (memory 1) (import "" "" (memory 1)))`
//   - Note the latter expands to the same as the former: `(import "" "" (memory 1))`
//
// * `(module (import "" "" (memory 1)) (import "" "" (memory 1)))`
//
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#tables%E2%91%A0
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#memories%E2%91%A0
func moreThanOneInvalidInSection(section wasm.SectionID) error {
	return fmt.Errorf("at most one %s allowed", wasm.SectionIDName(section))
}

func unhandledSection(section wasm.SectionID) error {
	return fmt.Errorf("BUG: unhandled %s", wasm.SectionIDName(section))
}

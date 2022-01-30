package text

import (
	"fmt"

	"github.com/tetratelabs/wazero/wasm"
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

func unhandledSection(section wasm.SectionID) error {
	return fmt.Errorf("BUG: unhandled %s", wasm.SectionIDName(section))
}

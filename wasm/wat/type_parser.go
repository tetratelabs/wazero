package wat

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/tetratelabs/wazero/wasm"
)

// typeParsingState is used to give an appropriate typeParser.errorContext
type typeParsingState byte

const (
	parsingParamOrResult typeParsingState = iota
	parsingParam
	parsingResult
	parsingComplete
)

// typeParser parses an inlined type from a field such as "type" or "func". Unlike normal parsers, this is not used for
// an entire field (enclosed by parens). Rather, this only handles "param" and "result" inner fields in the correct
// order.
//
// Ex. `(module (import (func $main (param i32))))`
//   This parses after the name     ^---------^
//
// Ex. `(module (type (param i32) (result i64)))`
//   This parses here ^----------------------^
//
// Ex. `(module (import (func)))`
//   This parses nothing (because there is no type)
//
// typeParser is reusable. The caller resets when reaching the appropriate tokenRParen via beginParsingParamsOrResult.
type typeParser struct {
	// m is used as a function pointer to moduleParser.tokenParser. This updates based on state changes.
	m *moduleParser

	// onTypeEnd is invoked when the grammar "(param)* (result)?" completes.
	//
	// Note: this is called when neither a "param" nor a "result" field are found, or on any field following a "param"
	// that is not a "result".
	onTypeEnd tokenParser

	// state is initially parsingParamOrResult and updated alongside tokenParser
	state typeParsingState

	// currentParams allow us to accumulate typeFunc.params across multiple fields, as well support abbreviated
	// anonymous parameters. ex. both (param i32) (param i32) and (param i32 i32) formats.
	// See https://www.w3.org/TR/wasm-core-1/#abbreviations%E2%91%A2
	currentParams []wasm.ValueType

	// currentParamIndex is used to give an appropriate errorContext
	currentParamIndex uint32

	// foundParam allows us to check if we found a type in a "param" field. We can't use currentParamIndex because when
	// parameters are abbreviated, ex. (param i32 i32), the currentParamIndex will be less than the type count.
	foundParam bool

	// currentResult is zero until set, and only set once as WebAssembly 1.0 only supports up to one result.
	currentResult wasm.ValueType
}

// beginParsingParamsOrResult sets moduleParser.tokenParser to beginParamOrResult after resetting internal fields.
// This should only be called when reaching the first tokenLParen after the optional field name (tokenID).
//
// Ex. Given the source `(module (import (func $main (param i32))))`
//         Set this result to the next parser here --^
//             This result restores control to onTypeEnd here --^
//
// The onTypeEnd parameter is invoked once any "param" and "result" fields have been consumed.
//
// NOTE: An empty function is valid and will not reach a tokenLParen! Ex. `(module (import (func)))`
func (p *typeParser) beginParsingParamsOrResult(onTypeEnd tokenParser) {
	p.onTypeEnd = onTypeEnd
	p.state = parsingParamOrResult
	p.m.tokenParser = p.beginParamOrResult
	p.currentParams = nil
	p.currentParamIndex = 0
	p.currentResult = 0
}

// beginParamOrResult is a tokenParser called after a tokenLParen and accepts either a "param" or a "result" field
// (tokenKeyword).
func (p *typeParser) beginParamOrResult(tok tokenType, tokenBytes []byte, line, col uint32) error {
	if tok == tokenKeyword {
		switch string(tokenBytes) {
		case "param":
			p.state = parsingParam
			p.foundParam = false
			p.m.tokenParser = p.parseParam
		case "result":
			p.state = parsingResult
			p.m.tokenParser = p.parseResult
		case "type": // cannot repeat
			return errors.New("TODO: (module (import (func (type")
		}
		return nil
	}
	// If we reach here, it is a syntax error, so punt it to onTypeEnd. Ex. (func ($param i32))
	return p.onTypeEnd(tok, tokenBytes, line, col)
}

func (p *typeParser) parseMoreParamsOrResult(tok tokenType, tokenBytes []byte, line, col uint32) error {
	if tok == tokenLParen {
		p.state = parsingParamOrResult
		p.m.tokenParser = p.beginParamOrResult
		return nil
	}
	// If we reached this point, we have one or more parameters, but no result. Ex. (func (param i32)) or (func)
	return p.onTypeEnd(tok, tokenBytes, line, col)
}

func (p *typeParser) parseParam(tok tokenType, tokenBytes []byte, _, _ uint32) error {
	switch tok {
	case tokenID: // Ex. $len
		return errors.New("TODO param name ex (param $len i32), but not in abbreviation ex (param $x i32 $y i32)")
	case tokenKeyword: // Ex. i32
		vt, err := parseValueType(tokenBytes)
		if err != nil {
			return err
		}
		p.currentParams = append(p.currentParams, vt)
		p.foundParam = true
	case tokenRParen: // end of this field
		if !p.foundParam {
			return errors.New("expected a type")
		}

		// since multiple param fields are valid, ex `(func (param i32) (param i64))`, prepare for any next.
		p.currentParamIndex++
		p.state = parsingParamOrResult
		p.m.tokenParser = p.parseMoreParamsOrResult
	default:
		return unexpectedToken(tok, tokenBytes)
	}
	return nil
}

// parseResult is a tokenParser inside a "result" field (tokenKeyword). When this field completes (tokenRParen), control
// transitions to parseComplete.
func (p *typeParser) parseResult(tok tokenType, tokenBytes []byte, _, _ uint32) error {
	switch tok {
	case tokenKeyword: // Ex. i32
		if p.currentResult != 0 {
			return errors.New("redundant type")
		}
		vt, err := parseValueType(tokenBytes)
		if err != nil {
			return err
		}
		p.currentResult = vt
	case tokenRParen: // end of this field
		if p.currentResult == 0 {
			return errors.New("expected a type")
		}
		p.m.tokenParser = p.onTypeEnd
		p.state = parsingComplete
	default:
		return unexpectedToken(tok, tokenBytes)
	}
	return nil
}

func (p *typeParser) errorContext() string {
	switch p.state {
	case parsingParam:
		return fmt.Sprintf(".param[%d]", p.currentParamIndex)
	case parsingResult:
		return ".result"
	}
	return ""
}

// findOrAddType returns the index in module.types of a possibly empty type matching any current params or result.
func (p *typeParser) findOrAddType(m *module) (typeIndex uint32) {
	// search for an existing signature that matches the current type
	for i, t := range m.types {
		if bytes.Equal(p.currentParams, t.params) && p.currentResult == t.result {
			return uint32(i)
		}
	}

	// if we didn't find a match, we need to insert a new type and use it
	typeIndex = uint32(len(m.types))
	m.types = append(m.types, &typeFunc{p.currentParams, p.currentResult})
	return
}

func parseValueType(tokenBytes []byte) (vt wasm.ValueType, err error) {
	t := string(tokenBytes)
	switch t {
	case "i32":
		vt = wasm.ValueTypeI32
	case "i64":
		vt = wasm.ValueTypeI64
	case "f32":
		vt = wasm.ValueTypeF32
	case "f64":
		vt = wasm.ValueTypeF64
	default:
		err = fmt.Errorf("unknown type: %s", t)
	}
	return
}

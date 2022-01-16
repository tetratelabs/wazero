package wat

import (
	"errors"
	"fmt"
)

// indexParser parses an index field, such as module.startFunction
type indexParser struct {
	// onIndexEnd is invoked on tokenRParen an index was successfully parsed.
	onIndexEnd func(*index)

	// currentIndex is set when an index was parsed
	currentIndex *index
}

// beginParsingIndex sets the next parser to parseIndex after resetting internal fields.
// This should only be called inside a field that can only contain an index.
//
// Ex. Given the source `(module (start $main))`
//             parseIndex starts here --^    ^
//               onIndexEnd is called here --+
//
// The onIndexEnd parameter is invoked once any "param" and "result" fields have been consumed.
//
// NOTE: An empty function is valid and will not reach a tokenLParen! Ex. `(module (import (func)))`
func (p *indexParser) beginParsingIndex(onIndexEnd func(*index)) tokenParser {
	p.onIndexEnd = onIndexEnd
	p.currentIndex = nil
	return p.parseIndex
}

// parseIndex is a tokenParser called in a field that can only contain a symbolic identifier or raw numeric index.
func (p *indexParser) parseIndex(tok tokenType, tokenBytes []byte, line, col uint32) error {
	switch tok {
	case tokenUN: // Ex. 2
		if p.currentIndex != nil {
			return errors.New("redundant index")
		}
		numeric, err := decodeUint32(tokenBytes)
		if err != nil {
			return fmt.Errorf("index outside range of uint32: %s", tokenBytes)
		}
		p.currentIndex = &index{numeric: numeric, line: line, col: col}
	case tokenID: // Ex. $main
		if p.currentIndex != nil {
			return errors.New("redundant index")
		}
		p.currentIndex = &index{ID: string(stripDollar(tokenBytes)), line: line, col: col}
	case tokenRParen: // end of this field
		if p.currentIndex == nil {
			return errors.New("missing index")
		}
		p.onIndexEnd(p.currentIndex)
		return nil
	default:
		return unexpectedToken(tok, tokenBytes)
	}
	return nil
}

package wat

import (
	"errors"
	"fmt"
)

type ModuleParser struct {
	source                     []byte
	tm                         *textModule
	parenDepth, skipUntilDepth int
	// currentStringCount allows us to unquote the Import.module and Import.name fields, differentiating empty from
	// never set, without making Import.module and Import.name pointers.
	currentStringCount int
	fieldHandler       fieldHandler
	tokenParser        tokenParser
	// afterInlining sets the scope to return to after parsing a function.
	// This is needed because a function can be defined at module scope or an inlined scope such as an import.
	// TODO: https://www.w3.org/TR/wasm-core-1/#abbreviations%E2%91%A8
	afterInlining tokenParser
	currentImport *textImport
	currentFunc   *textFunc
}

// fieldHandler returns a tokenParser that resumes parsing after "($fieldName". This must handle all tokens until
// reaching a final tokenRParen. This implies nested paren handling.
type fieldHandler func(fieldName []byte) (tokenParser, error)

func (p *ModuleParser) parse(tok tokenType, tokenBytes []byte, line, col int) error {
	if p.skipUntilDepth == 0 {
		return p.tokenParser(tok, tokenBytes, line, col)
	}
	if tok == tokenLParen {
		p.parenDepth = p.parenDepth + 1
	} else if tok == tokenRParen {
		if p.parenDepth == p.skipUntilDepth {
			p.skipUntilDepth = 0
		}
		p.parenDepth = p.parenDepth - 1
	}
	return nil
}

func (p *ModuleParser) startField(tok tokenType, tokenBytes []byte, _, _ int) error {
	if tok != tokenKeyword {
		return fmt.Errorf("expected field, but found %s", tok)
	}

	np, err := p.fieldHandler(tokenBytes)
	if err != nil {
		return err
	}
	p.tokenParser = np
	return nil
}

func (p *ModuleParser) startModule(fieldName []byte) (tokenParser, error) {
	if string(fieldName) == "module" {
		return p.parseModule, nil
	} else {
		return nil, fmt.Errorf("unexpected field: %s", string(fieldName))
	}
}

func (p *ModuleParser) parseModule(tok tokenType, tokenBytes []byte, _, _ int) error {
	switch tok {
	case tokenID:
		name := string(tokenBytes)
		if p.tm.name != "" {
			return fmt.Errorf("redundant name: %s", name)
		}
		p.tm.name = name
	case tokenLParen:
		p.tokenParser = p.startField        // after this look for a field name
		p.fieldHandler = p.startModuleField // this defines the field names accepted
		return nil
	case tokenRParen: // end of module
	default:
		return p.unexpectedToken(tok, tokenBytes)
	}
	return nil
}

func (p *ModuleParser) startModuleField(fieldName []byte) (tokenParser, error) {
	switch string(fieldName) {
	case "import":
		p.currentImport = &textImport{}
		p.tm.imports = append(p.tm.imports, p.currentImport)
		return p.parseImport, nil
	case "start": // TODO: only one is allowed
		return p.parseStart, nil
	}
	return nil, fmt.Errorf("unexpected field: %s", string(fieldName))
}

func (p *ModuleParser) parseImport(tok tokenType, tokenBytes []byte, _, _ int) error {
	switch tok {
	case tokenString: // Ex. "" or "foo" including quotes!
		name := string(tokenBytes[1 : len(tokenBytes)-1]) // unquote
		if p.currentStringCount == 0 {
			p.currentImport.module = name
		} else if p.currentImport.name != "" {
			return fmt.Errorf("redundant name: %s", name)
		} else {
			p.currentImport.name = name
		}
		p.currentStringCount = p.currentStringCount + 1
	case tokenLParen: // start field
		p.tokenParser = p.startField        // after this look for a field name
		p.fieldHandler = p.startImportField // this defines the field names accepted
		return nil
	case tokenRParen: // end of this import
		switch p.currentStringCount {
		case 0:
			return errors.New("expected module and name")
		case 1:
			return errors.New("expected name")
		}
		if p.currentImport.desc == nil {
			return errors.New("expected descripton")
		}
		p.currentImport = nil
		p.currentStringCount = 0
		p.tokenParser = p.parseModule
	default:
		return p.unexpectedToken(tok, tokenBytes)
	}
	return nil
}

func (p *ModuleParser) startImportField(fieldName []byte) (tokenParser, error) {
	switch string(fieldName) {
	case "func":
		p.currentFunc = &textFunc{}
		p.currentImport.desc = p.currentFunc
		p.afterInlining = p.parseImport
		return p.parseFunc, nil
	}
	return nil, fmt.Errorf("unexpected field: %s", string(fieldName))
}

func (p *ModuleParser) parseFunc(tok tokenType, tokenBytes []byte, _, _ int) error {
	switch tok {
	case tokenID: // Ex. $main
		name := string(tokenBytes)
		if p.currentFunc.name != "" {
			return fmt.Errorf("redundant name: %s", name)
		}
		p.currentFunc.name = name
	case tokenRParen: // end of this func
		p.currentFunc = nil
		// There are two places a func ends: after inlining or after its module field.
		// Ex. (module (import "" "hello" (func $hello)) (func $goodbye))
		//                                    inlined ^   module field ^
		if p.afterInlining != nil {
			p.tokenParser = p.afterInlining
			p.afterInlining = nil
		} else {
			p.tokenParser = p.parseModule
		}
	default:
		return p.unexpectedToken(tok, tokenBytes)
	}
	return nil
}

func (p *ModuleParser) parseStart(tok tokenType, tokenBytes []byte, _, _ int) error {
	switch tok {
	case tokenUN, tokenID: // Ex. $main or 2
		funcidx := string(tokenBytes)
		if p.tm.startFunction != "" {
			return fmt.Errorf("redundant funcidx: %s", funcidx)
		}
		p.tm.startFunction = funcidx
	case tokenRParen: // end of this start
		if p.tm.startFunction == "" {
			return errors.New("missing funcidx")
		}
		p.tokenParser = p.parseModule
	default:
		return p.unexpectedToken(tok, tokenBytes)
	}
	return nil
}

func (p *ModuleParser) startFile(tok tokenType, _ []byte, _, _ int) error {
	if tok != tokenLParen {
		return fmt.Errorf("expected '(', but found %s", tok)
	}
	p.parenDepth = p.parenDepth + 1
	p.tokenParser = p.startField
	p.fieldHandler = p.startModule
	return nil
}

// ParseModule parses the configured source into a module. This function returns when the source is exhausted or an
// error occurs.
//
// Here's a description of the return values:
// * tm is the result of parsing or nil on error
// * err is a textFormatError invoking the parser, dangling block comments or unexpected characters.
func ParseModule(source []byte) (*textModule, error) {
	p := ModuleParser{source: source, tm: &textModule{}}
	p.tokenParser = p.startFile
	line, col, err := lex(p.parse, p.source)
	if err != nil {
		return nil, &textFormatError{line, col, p.errorContext(), err}
	}
	return p.tm, nil
}

func (p *ModuleParser) unexpectedToken(tok tokenType, tokenBytes []byte) error {
	if tok == tokenLParen || tok == tokenRParen {
		return fmt.Errorf("unexpected %s", tok)
	}
	return fmt.Errorf("unexpected %s: %s", tok, tokenBytes)
}

func (p *ModuleParser) errorContext() string {
	if p.currentImport != nil {
		i := len(p.tm.imports) - 1
		if p.currentImport.desc != nil {
			return fmt.Sprintf("import[%d].func", i) // TODO: func, table, memory or global
		}
		return fmt.Sprintf("import[%d]", i)
	} else if p.tm.startFunction != "" {
		return "start"
	}
	return "module"
}

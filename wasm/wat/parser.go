package wat

import (
	"errors"
	"fmt"
)

type ModuleParser struct {
	source []byte
	module *module
	// currentStringCount allows us to unquote the _import.module and _import.name fields, differentiating empty
	// from never set, without making _import.module and _import.name pointers.
	currentStringCount int
	fieldHandler       fieldHandler
	tokenParser        tokenParser
	// afterInlining sets the scope to return to after parsing a function.
	// This is needed because a function can be defined at module scope or an inlined scope such as an import.
	// TODO: https://www.w3.org/TR/wasm-core-1/#abbreviations%E2%91%A8
	afterInlining tokenParser
	currentImport *_import
}

// ParseModule parses the configured source into a module. This function returns when the source is exhausted or an
// error occurs.
//
// Here's a description of the return values:
// * module is the result of parsing or nil on error
// * err is a formatError invoking the parser, dangling block comments or unexpected characters.
func ParseModule(source []byte) (*module, error) {
	p := ModuleParser{source: source, module: &module{}}
	p.tokenParser = p.startFile
	line, col, err := lex(p.parse, p.source)
	if err != nil {
		return nil, &formatError{line, col, p.errorContext(), err}
	}
	return p.module, nil
}

// fieldHandler returns a tokenParser that resumes parsing after "($fieldName". This must handle all tokens until
// reaching a final tokenRParen. This implies nested paren handling.
type fieldHandler func(fieldName []byte) (tokenParser, error)

// parse calls the delegate ModuleParser.tokenParser
func (p *ModuleParser) parse(tok tokenType, tokenBytes []byte, line, col int) error {
	return p.tokenParser(tok, tokenBytes, line, col)
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
		if p.module.name != "" {
			return fmt.Errorf("redundant name: %s", name)
		}
		p.module.name = name
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
		p.currentImport = &_import{}
		p.module.imports = append(p.module.imports, p.currentImport)
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
		if p.currentImport.importFunc == nil {
			return errors.New("expected description")
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
		p.currentImport.importFunc = &importFunc{}
		p.afterInlining = p.parseImport
		return p.parseImportFunc, nil
	}
	return nil, fmt.Errorf("unexpected field: %s", string(fieldName))
}

func (p *ModuleParser) parseImportFunc(tok tokenType, tokenBytes []byte, _, _ int) error {
	switch tok {
	case tokenID: // Ex. $main
		name := string(tokenBytes)
		if p.currentImport.importFunc.name != "" {
			return fmt.Errorf("redundant name: %s", name)
		}
		p.currentImport.importFunc.name = name
	case tokenRParen: // end of this func
		p.tokenParser = p.afterInlining
		p.afterInlining = nil
	default:
		return p.unexpectedToken(tok, tokenBytes)
	}
	return nil
}

func (p *ModuleParser) parseStart(tok tokenType, tokenBytes []byte, _, _ int) error {
	switch tok {
	case tokenUN, tokenID: // Ex. $main or 2
		funcidx := string(tokenBytes)
		if p.module.startFunction != "" {
			return fmt.Errorf("redundant funcidx: %s", funcidx)
		}
		p.module.startFunction = funcidx
	case tokenRParen: // end of this start
		if p.module.startFunction == "" {
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
	p.tokenParser = p.startField
	p.fieldHandler = p.startModule
	return nil
}

func (p *ModuleParser) unexpectedToken(tok tokenType, tokenBytes []byte) error {
	if tok == tokenLParen || tok == tokenRParen {
		return fmt.Errorf("unexpected %s", tok)
	}
	return fmt.Errorf("unexpected %s: %s", tok, tokenBytes)
}

func (p *ModuleParser) errorContext() string {
	if p.currentImport != nil {
		i := len(p.module.imports) - 1
		if p.currentImport.importFunc != nil {
			return fmt.Sprintf("import[%d].func", i)
		}
		// TODO: table, memory or global
		return fmt.Sprintf("import[%d]", i)
	} else if p.module.startFunction != "" {
		return "start"
	}
	return "module"
}

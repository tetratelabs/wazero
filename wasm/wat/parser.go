package wat

import (
	"fmt"
)

type ModuleParser struct {
	source                     []byte
	m                          *textModule
	parenDepth, skipUntilDepth int
	// currentStringCount allows us to unquote the Import.module and Import.name fields, differentiating empty from
	// never set, without making Import.module and Import.name pointers.
	currentStringCount int
	pf                 fieldHandler // TODO: crappy name
	pt                 parseToken   // TODO: crappy name
	// afterInlining sets the scope to return to after parsing a function.
	// This is needed because a function can be defined at module scope or an inner scope such as an import.
	// TODO: https://www.w3.org/TR/wasm-core-1/#abbreviations%E2%91%A8
	afterInlining parseToken
	currentImport *textImport
	currentFunc   *textFunc
}

// fieldHandler returns a parseToken index that resumes parsing after "($fieldName". This must handle all tokens until
// reaching a final tokenRParen. This implies nested paren handling.
type fieldHandler func(fieldName []byte) (parseToken, error)

func (p *ModuleParser) parse(tok tokenType, tokenBytes []byte, line, col int) error {
	if p.skipUntilDepth == 0 {
		return p.pt(tok, tokenBytes, line, col)
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
		return fmt.Errorf("%s has a %s where a field name was expected", p.errorContext(), tok)
	}

	np, err := p.pf(tokenBytes)
	if err != nil {
		return err
	}
	p.pt = np
	return nil
}

func (p *ModuleParser) startModule(fieldName []byte) (parseToken, error) {
	if string(fieldName) == "module" {
		return p.parseModule, nil
	} else {
		return nil, fmt.Errorf("%s should not have the field: %s", p.errorContext(), string(fieldName))
	}
}

func (p *ModuleParser) parseModule(tok tokenType, tokenBytes []byte, _, _ int) error {
	switch tok {
	case tokenID:
		name := string(tokenBytes)
		if p.m.name != "" {
			return fmt.Errorf("%s has a redundant name: %s", p.errorContext(), name)
		}
		p.m.name = name
	case tokenLParen:
		p.pt = p.startField       // after this look for a field name
		p.pf = p.startModuleField // this defines the field names accepted
		return nil
	case tokenRParen: // end of module
	default:
		return p.unexpectedToken(tok, tokenBytes)
	}
	return nil
}

func (p *ModuleParser) startModuleField(fieldName []byte) (parseToken, error) {
	switch string(fieldName) {
	case "import":
		p.currentImport = &textImport{}
		p.m.imports = append(p.m.imports, p.currentImport)
		return p.parseImport, nil
	case "start": // TODO: only one is allowed
		return p.parseStart, nil
	}
	return nil, fmt.Errorf("%s should not have the field: %s", p.errorContext(), string(fieldName))
}

func (p *ModuleParser) parseImport(tok tokenType, tokenBytes []byte, _, _ int) error {
	switch tok {
	case tokenString: // Ex. "" or "foo" including quotes!
		name := string(tokenBytes[1 : len(tokenBytes)-1]) // unquote
		if p.currentStringCount == 0 {
			p.currentImport.module = name
		} else if p.currentImport.name != "" {
			return fmt.Errorf("%s has a redundant name: %s", p.errorContext(), name)
		} else {
			p.currentImport.name = name
		}
		p.currentStringCount = p.currentStringCount + 1
	case tokenLParen: // start field
		p.pt = p.startField       // after this look for a field name
		p.pf = p.startImportField // this defines the field names accepted
		return nil
	case tokenRParen: // end of this import
		switch p.currentStringCount {
		case 0:
			return fmt.Errorf("%s is missing its module and name", p.errorContext())
		case 1:
			return fmt.Errorf("%s is missing its name", p.errorContext())
		}
		if p.currentImport.desc == nil {
			return fmt.Errorf("%s is missing its descripton", p.errorContext())
		}
		p.currentImport = nil
		p.currentStringCount = 0
		p.pt = p.parseModule
	default:
		return p.unexpectedToken(tok, tokenBytes)
	}
	return nil
}

func (p *ModuleParser) startImportField(fieldName []byte) (parseToken, error) {
	switch string(fieldName) {
	case "func":
		p.currentFunc = &textFunc{}
		p.currentImport.desc = p.currentFunc
		p.afterInlining = p.parseImport
		return p.parseFunc, nil
	}
	return nil, fmt.Errorf("%s should not have the field: %s", p.errorContext(), string(fieldName))
}

func (p *ModuleParser) parseFunc(tok tokenType, tokenBytes []byte, _, _ int) error {
	switch tok {
	case tokenID: // Ex. $main
		name := string(tokenBytes)
		if p.currentFunc.name != "" {
			return fmt.Errorf("%s has a redundant name: %s", p.errorContext(), name)
		}
		p.currentFunc.name = name
	case tokenRParen: // end of this func
		p.currentFunc = nil
		// There are two places a func ends: after inlining or after its module field.
		// Ex. (module (import "" "hello" (func $hello)) (func $goodbye))
		//                                    inlined ^   module field ^
		if p.afterInlining != nil {
			p.pt = p.afterInlining
			p.afterInlining = nil
		} else {
			p.pt = p.parseModule
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
		if p.m.startFunction != "" {
			return fmt.Errorf("%s has a redundant funcidx: %s", p.errorContext(), funcidx)
		}
		p.m.startFunction = funcidx
	case tokenRParen: // end of this start
		if p.m.startFunction == "" {
			return fmt.Errorf("%s is missing its funcidx", p.errorContext())
		}
		p.pt = p.parseModule
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
	p.pt = p.startField
	p.pf = p.startModule
	return nil
}

// ParseModule parses the configured source into a module. This function returns when the source is exhausted or an
// error occurs.
//
// Here's a description of the return values:
// * m is the result of parsing or nil on error
// * line is the source line number determined by unescaped '\n' characters of the error or EOF
// * col is the UTF-8 column number of the error or EOF
// * err is an error invoking the parser, dangling block comments or unexpected characters.
func ParseModule(source []byte) (m *textModule, line int, col int, err error) {
	m = &textModule{}
	p := ModuleParser{source: source}
	p.pt = p.startFile
	p.m = m
	line, col, err = lex(p.parse, p.source)
	return m, line, col, err
}

func (p *ModuleParser) unexpectedToken(tok tokenType, tokenBytes []byte) error {
	if tok == tokenLParen || tok == tokenRParen {
		return fmt.Errorf("%s has an unexpected %s", p.errorContext(), tok)
	}
	return fmt.Errorf("%s has an unexpected %s: %s", p.errorContext(), tok, tokenBytes)
}

func (p *ModuleParser) errorContext() string {
	if p.currentImport != nil {
		i := len(p.m.imports)
		if p.currentImport.desc != nil {
			return fmt.Sprintf("import[%d].func", i) // TODO: func, table, memory or global
		}
		return fmt.Sprintf("import[%d]", i)
	} else if p.m.startFunction != "" {
		return "start"
	}
	return "module"
}

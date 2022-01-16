package wat

import (
	"errors"
	"fmt"
)

// currentField holds the positional state of parser. Values are also useful as they allow you to do a reference search
// for all related code including parsers of that position.
type currentField byte

const (
	// fieldInitial is the first position in the source being parsed.
	fieldInitial currentField = iota
	// fieldModule is at the top-level field named "module" and cannot repeat in the same source.
	fieldModule
	// fieldModuleType is at the position module.type and can repeat in the same module.
	//
	// At the start of the field, moduleParser.currentValue0 tracks typeFunc.name. If a field named "func" is
	// encountered, these names are recorded while fieldModuleTypeFunc takes over parsing.
	fieldModuleType
	// fieldModuleTypeFunc is at the position module.type.func and cannot repeat in the same type.
	fieldModuleTypeFunc
	// fieldModuleImport is at the position module.import and can repeat in the same module.
	//
	// At the start of the field, moduleParser.currentValue0 tracks importFunc.module while moduleParser.currentValue1
	// tracks importFunc.name. If a field named "func" is encountered, these names are recorded while
	// fieldModuleImportFunc takes over parsing.
	fieldModuleImport
	// fieldModuleImportFunc is at the position module.import.func and cannot repeat in the same import.
	fieldModuleImportFunc
	// fieldModuleStart is at the position module.start and cannot repeat in the same module.
	fieldModuleStart
)

type moduleParser struct {
	// tokenParser primarily supports dispatching parse to a different tokenParser depending on the position in the file
	// The initial value is ensureLParen because %.wat files must begin with a '(' token (ignoring whitespace).
	//
	// Design Note: This is an alternative to using a stack as the structure defined in "module" is fixed depth, except
	// function bodies. Any function body may be parsed in a more dynamic way.
	tokenParser tokenParser

	// source is the entire WebAssembly text format source code being parsed.
	source []byte

	// module holds the fields incrementally parsed from tokens in the source.
	module *module

	// currentField is the parser and error context.
	// This is set after reading a field name, ex "module", or after reaching the end of one, ex ')'.
	currentField currentField

	// currentValue0 is a currentField-specific place to stash a string when parsing a field.
	// Ex. for fieldModuleImport, this would be Math if (import "Math" "PI" ...)
	currentValue0 []byte

	// currentValue1 is a currentField-specific place to stash a string when parsing a field.
	// Ex. for fieldModuleImport, this would be PI if (import "Math" "PI" ...)
	currentValue1 []byte

	// currentImportIndex allows us to track the relative position of module.importFuncs regardless of position in the source.
	currentImportIndex uint32

	// currentTypeIndex allows us to track the relative position of module.types regardless of position in the source.
	currentTypeIndex uint32

	typeParser  *typeParser
	indexParser *indexParser
}

// parse has the same signature as tokenParser and called by lex on each token.
//
// The tokenParser this dispatches to should be updated when reading a new field name, and restored to the prior
// value or a different parser on endField.
func (p *moduleParser) parse(tok tokenType, tokenBytes []byte, line, col uint32) error {
	return p.tokenParser(tok, tokenBytes, line, col)
}

// parseModule parses the configured source into a module. This function returns when the source is exhausted or an
// error occurs.
//
// Here's a description of the return values:
// * module is the result of parsing or nil on error
// * err is a FormatError invoking the parser, dangling block comments or unexpected characters.
func parseModule(source []byte) (*module, error) {
	p := moduleParser{source: source, module: &module{}, indexParser: &indexParser{}}
	p.typeParser = &typeParser{m: &p}

	// A valid source must begin with the token '(', but it could be preceded by whitespace or comments. For this
	// reason, we cannot enforce source[0] == '(', and instead need to start the lexer to check the first token.
	p.tokenParser = p.ensureLParen
	line, col, err := lex(p.parse, p.source)
	if err != nil {
		return nil, &FormatError{line, col, p.errorContext(), err}
	}

	// Add any types implicitly defined from type use. Ex. (module (import (func (param i32)...
	p.module.types = append(p.module.types, p.typeParser.inlinedTypes...)

	// Ensure indices only point to numeric values
	if err = bindIndices(p.module); err != nil {
		return nil, err
	}

	return p.module, nil
}

func (p *moduleParser) ensureLParen(tok tokenType, tokenBytes []byte, _, _ uint32) error {
	if tok != tokenLParen {
		return fmt.Errorf("expected '(', but found %s: %s", tok, tokenBytes)
	}
	p.tokenParser = p.beginField
	return nil
}

// beginField assigns the correct moduleParser.currentField and moduleParser.parseModule based on the source position
// and fieldName being read.
//
// Once the next parser reaches a tokenRParen, moduleParser.endField must be called. This means that there must be
// parity between the currentField values handled here and those handled in moduleParser.endField
//
// TODO: this design will likely be revisited to introduce a type that handles both begin and end of the current field.
func (p *moduleParser) beginField(tok tokenType, fieldName []byte, _, _ uint32) error {
	if tok != tokenKeyword {
		return fmt.Errorf("expected field, but found %s", tok)
	}

	// We expect p.currentField set according to a potentially nested "($fieldName".
	// Each case must return a tokenParser that consumes the rest of the field up to the ')'.
	// Note: each branch must handle any nesting concerns. Ex. "(module (import" nests further to "(func".
	p.tokenParser = nil
	switch p.currentField {
	case fieldInitial:
		if string(fieldName) == "module" {
			p.currentField = fieldModule
			p.tokenParser = p.parseModuleName
		}
	case fieldModule:
		switch string(fieldName) {
		case "type":
			p.currentField = fieldModuleType
			p.tokenParser = p.parseTypeName
		case "import":
			p.currentField = fieldModuleImport
			p.tokenParser = p.parseImportModule
		case "start":
			if p.module.startFunction != nil {
				return errors.New("redundant start")
			}
			p.currentField = fieldModuleStart
			p.tokenParser = p.indexParser.beginParsingIndex(p.parseStartEnd)
		}
	case fieldModuleType:
		if string(fieldName) == "func" {
			p.currentField = fieldModuleTypeFunc
			p.tokenParser = p.parseTypeFunc
		}
	case fieldModuleImport:
		// Add the next import func object and ready for parsing it.
		if string(fieldName) == "func" {
			p.module.importFuncs = append(p.module.importFuncs, &importFunc{
				module:      string(p.currentValue0),
				name:        string(p.currentValue1),
				importIndex: p.currentImportIndex,
			})

			p.currentField = fieldModuleImportFunc
			p.tokenParser = p.parseImportFuncName
		} // TODO: table, memory or global
	}
	if p.tokenParser == nil {
		return fmt.Errorf("unexpected field: %s", string(fieldName))
	}
	return nil
}

// endField should be called after encountering tokenRParen. It places the current parser at the parent position based
// on fixed knowledge of the text format structure.
//
// Design Note: This is an alternative to using a stack as the structure parsed by moduleParser is fixed depth. For
// example, any function body may be parsed in a more dynamic way.
func (p *moduleParser) endField() {
	switch p.currentField {
	case fieldModuleStart, fieldModuleImport, fieldModuleType:
		p.currentField = fieldModule
		p.tokenParser = p.parseModule
	case fieldModule:
		p.currentField = fieldInitial
		p.tokenParser = p.parseUnexpectedTrailingCharacters // only one module is allowed and nothing else
	default: // currentField is an enum, we expect to have handled all cases above. panic if we didn't
		panic(fmt.Errorf("BUG: unhandled parsing state on endField: %v", p.currentField))
	}
}

// parseModuleName is the first parser inside the module field. This records the module.name if present and sets the
// next parser to parseModule. If the token isn't a tokenID, this calls parseModule.
//
// Ex. A module name is present `(module $math)`
//                        records math --^
//
// Ex. No module name `(module)`
//   calls parseModule here --^
func (p *moduleParser) parseModuleName(tok tokenType, tokenBytes []byte, line, col uint32) error {
	if tok == tokenID { // Ex. $Math
		p.module.name = string(stripDollar(tokenBytes))
		p.tokenParser = p.parseModule
		return nil
	}
	return p.parseModule(tok, tokenBytes, line, col)
}

func (p *moduleParser) parseModule(tok tokenType, tokenBytes []byte, _, _ uint32) error {
	switch tok {
	case tokenID:
		return fmt.Errorf("redundant name: %s", string(tokenBytes))
	case tokenLParen:
		p.tokenParser = p.beginField // after this look for a field name
		return nil
	case tokenRParen: // end of module
		p.endField()
	default:
		return unexpectedToken(tok, tokenBytes)
	}
	return nil
}

// parseTypeName is the first parser inside a type field. This records the typeFunc.name if present or calls parseType
// if not found.
//
// Ex. A type name is present `(type $t0 (func (result i32)))`
//                      records t0 --^   ^
//              parseType resumes here --+
//
// Ex. No type name `(type (func (result i32)))`
//       calls parseType --^
func (p *moduleParser) parseTypeName(tok tokenType, tokenBytes []byte, line, col uint32) error {
	if tok == tokenID { // Ex. $v_v
		p.currentValue0 = stripDollar(tokenBytes)
		p.tokenParser = p.parseType
		return nil
	}
	return p.parseType(tok, tokenBytes, line, col)
}

// parseType is the last parser inside the type field. This records the func field or errs if missing. When complete,
// this sets the next parser to parseModule.
//
// Ex. func is present `(module (type $rf32 (func (result f32))))`
//                            starts here --^                   ^
//                                   parseModule resumes here --+
//
// Ex. func is missing `(type $rf32 )`
//                      errs here --^
func (p *moduleParser) parseType(tok tokenType, tokenBytes []byte, _, _ uint32) error {
	switch tok {
	case tokenID:
		return errors.New("redundant name")
	case tokenLParen: // start fields, ex. (func
		// Err if there's a second func. Ex. (type (func) (func))
		if uint32(len(p.module.types)) > p.currentTypeIndex {
			return unexpectedToken(tok, tokenBytes)
		}
		p.tokenParser = p.beginField
		return nil
	case tokenRParen: // end of this type
		// Err if we never reached a description...
		if uint32(len(p.module.types)) == p.currentTypeIndex {
			return errors.New("missing func field")
		}

		// Multiple types are allowed, so advance in case there's a next.
		p.currentTypeIndex++
		p.endField()
	default:
		return unexpectedToken(tok, tokenBytes)
	}
	return nil
}

// parseTypeFunc is the second parser inside the type field. This passes control to the typeParser until
// any signature is read, then sets the next parser to parseTypeFuncEnd.
//
// Ex. `(module (type $rf32 (func (result f32))))`
//            starts here --^                 ^
//            parseTypeFuncEnd resumes here --+
//
// Ex. If there is no signature `(module (type $rf32 ))`
//                    calls parseTypeFuncEnd here ---^
func (p *moduleParser) parseTypeFunc(tok tokenType, tokenBytes []byte, line, col uint32) error {
	p.typeParser.reset()
	if tok == tokenLParen {
		p.typeParser.beginType(p.parseTypeFuncEnd)
		return nil // start fields, ex. (param or (result
	}
	return p.parseTypeFuncEnd(tok, tokenBytes, line, col) // ended with no parameters
}

// parseTypeFuncEnd is the last parser of the "func" field. As there is no alternative to ending the field, this ensures
// the token is tokenRParen and sets the next parser to parseType on tokenRParen.
func (p *moduleParser) parseTypeFuncEnd(tok tokenType, tokenBytes []byte, _, _ uint32) error {
	if tok == tokenRParen {
		sig, names := p.typeParser.getType(string(p.currentValue0))
		if names != nil {
			p.module.typeParamNames = append(p.module.typeParamNames, &typeParamNames{uint32(len(p.module.types)), names})
		}
		p.module.types = append(p.module.types, sig)
		p.currentValue0 = nil
		p.currentField = fieldModuleType
		p.tokenParser = p.parseType
		return nil
	}
	return unexpectedToken(tok, tokenBytes)
}

// parseImportModule is the first parser inside the import field. This records the importFunc.module, then sets the next
// parser to parseImportName. Since the imported module name is required, this errs on anything besides tokenString.
//
// Ex. Imported module name is present `(import "Math" "PI" (func (result f32)))`
//                                records Math --^     ^
//                      parseImportName resumes here --+
//
// Ex. Imported module name is absent `(import (func (result f32)))`
//                                 errs here --^
func (p *moduleParser) parseImportModule(tok tokenType, tokenBytes []byte, _, _ uint32) error {
	switch tok {
	case tokenString: // Ex. "" or "Math"
		p.currentValue0 = tokenBytes[1 : len(tokenBytes)-1] // unquote
		p.tokenParser = p.parseImportName
		return nil
	case tokenLParen, tokenRParen:
		return errors.New("missing module and name")
	default:
		return unexpectedToken(tok, tokenBytes)
	}
}

// parseImportName is the second parser inside the import field. This records the importFunc.name, then sets the next
// parser to parseImport. Since the imported function name is required, this errs on anything besides tokenString.
//
// Ex. Imported function name is present `(import "Math" "PI" (func (result f32)))`
//                                         starts here --^    ^
//                                           records PI --^   |
//                                 parseImport resumes here --+
//
// Ex. Imported function name is absent `(import "Math" (func (result f32)))`
//                                          errs here --+
func (p *moduleParser) parseImportName(tok tokenType, tokenBytes []byte, _, _ uint32) error {
	switch tok {
	case tokenString: // Ex. "" or "PI"
		p.currentValue1 = tokenBytes[1 : len(tokenBytes)-1] // unquote
		p.tokenParser = p.parseImport
		return nil
	case tokenLParen, tokenRParen:
		return errors.New("missing name")
	default:
		return unexpectedToken(tok, tokenBytes)
	}
}

// parseImport is the last parser inside the import field. This records the description field, ex. (func) or errs if
// missing. When complete, this sets the next parser to parseModule.
//
// Ex. Description is present `(module (import "Math" "PI" (func (result f32))))`
//                                           starts here --^                   ^
//                                                  parseModule resumes here --+
//
// Ex. Description is missing `(import "Math" "PI")`
//                                    errs here --^
func (p *moduleParser) parseImport(tok tokenType, tokenBytes []byte, _, _ uint32) error {
	switch tok {
	case tokenString: // Ex. (import "Math" "PI" "PI"
		return fmt.Errorf("redundant name: %s", tokenBytes[1:len(tokenBytes)-1]) // unquote
	case tokenLParen: // start fields, ex. (func
		// Err if there's a second description. Ex. (import "" "" (func) (func))
		if uint32(len(p.module.importFuncs)) > p.currentImportIndex {
			return unexpectedToken(tok, tokenBytes)
		}
		p.tokenParser = p.beginField
		return nil
	case tokenRParen: // end of this import
		// Err if we never reached a description...
		if uint32(len(p.module.importFuncs)) == p.currentImportIndex {
			return errors.New("missing description field") // Ex. missing (func): (import "Math" "Pi")
		}

		// Multiple imports are allowed, so advance in case there's a next.
		p.currentImportIndex++

		// Reset parsing state: this is late to help give correct error messages on multiple descriptions.
		p.currentValue0, p.currentValue1 = nil, nil
		p.endField()
	default:
		return unexpectedToken(tok, tokenBytes)
	}
	return nil
}

// parseImportFuncName is the first parser inside an imported function field. This records the typeFunc.name if present
// and sets the next parser to parseImportFunc. If the token isn't a tokenID, this calls parseImportFunc.
//
// Ex. A function name is present `(import "Math" "PI" (func $math.pi (result f32))`
//                                    records math.pi here --^
//                                     parseImportFunc resumes here --^
//
// Ex. No function name `(import "Math" "PI" (func (result f32))`
//                    calls parseImportFunc here --^
func (p *moduleParser) parseImportFuncName(tok tokenType, tokenBytes []byte, line, col uint32) error {
	if tok == tokenID { // Ex. $main
		fn := p.module.importFuncs[len(p.module.importFuncs)-1]
		fn.funcName = string(stripDollar(tokenBytes))
		p.tokenParser = p.parseImportFunc
		return nil
	}
	return p.parseImportFunc(tok, tokenBytes, line, col)
}

// parseImportFunc is the second parser inside the imported function field. This passes control to the typeParser until
// any signature is read, then sets the next parser to parseImportFuncEnd.
//
// Ex. `(import "Math" "PI" (func $math.pi (result f32)))`
//                           starts here --^           ^
//             parseImportFuncEnd resumes here --+
//
// Ex. If there is no signature `(import "" "main" (func))`
//               calls parseImportFuncEnd here ---^
func (p *moduleParser) parseImportFunc(tok tokenType, tokenBytes []byte, line, col uint32) error {
	p.typeParser.reset() // reset now in case there is never a tokenLParen
	switch tok {
	case tokenID: // Ex. (func $main $main)
		return fmt.Errorf("redundant name: %s", tokenBytes)
	case tokenLParen:
		p.typeParser.beginTypeUse(p.parseImportFuncEnd) // start fields, ex. (param or (result
		return nil
	}
	return p.parseImportFuncEnd(tok, tokenBytes, line, col)
}

// parseImportFuncEnd is the last parser inside the imported function field. This records the importFunc.typeIndex
// and/or importFunc.typeInlined and sets the next parser to parseImport.
func (p *moduleParser) parseImportFuncEnd(tok tokenType, tokenBytes []byte, _, _ uint32) error {
	if tok == tokenRParen {
		fn := p.module.importFuncs[len(p.module.importFuncs)-1]
		fn.typeIndex, fn.typeInlined, fn.paramNames = p.typeParser.getTypeUse()
		p.currentField = fieldModuleImport
		p.tokenParser = p.parseImport
		return nil
	}
	return unexpectedToken(tok, tokenBytes)
}

func (p *moduleParser) parseStartEnd(funcidx *index) {
	p.module.startFunction = funcidx
	p.endField()
}

func (p *moduleParser) parseUnexpectedTrailingCharacters(_ tokenType, tokenBytes []byte, _, _ uint32) error {
	return fmt.Errorf("unexpected trailing characters: %s", tokenBytes)
}

func unexpectedToken(tok tokenType, tokenBytes []byte) error {
	if tok == tokenLParen { // unbalanced tokenRParen is caught at the lexer layer
		return errors.New("unexpected '('")
	}
	return fmt.Errorf("unexpected %s: %s", tok, tokenBytes)
}

func (p *moduleParser) errorContext() string {
	switch p.currentField {
	case fieldInitial:
		return ""
	case fieldModule:
		return "module"
	case fieldModuleType:
		return fmt.Sprintf("module.type[%d]", p.currentTypeIndex)
	case fieldModuleTypeFunc:
		return fmt.Sprintf("module.type[%d].func%s", p.currentTypeIndex, p.typeParser.errorContext())
	case fieldModuleImport:
		return fmt.Sprintf("module.import[%d]", p.currentImportIndex)
	case fieldModuleImportFunc: // TODO: table, memory or global
		return fmt.Sprintf("module.import[%d].func%s", p.currentImportIndex, p.typeParser.errorContext())
	case fieldModuleStart:
		return "module.start"
	default: // currentField is an enum, we expect to have handled all cases above. panic if we didn't
		panic(fmt.Errorf("BUG: unhandled parsing state on errorContext: %v", p.currentField))
	}
}

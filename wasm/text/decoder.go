package text

import (
	"errors"
	"fmt"

	"github.com/tetratelabs/wazero/wasm"
)

// parserPosition holds the positional state of a parser. Values are also useful as they allow you to do a reference
// search for all related code including parsers of that position.
type parserPosition byte

const (
	positionInitial parserPosition = iota
	positionFunc
	positionType
	positionParam
	positionResult
	positionModule
	positionModuleType
	positionModuleImport
	positionModuleImportFunc
	positionModuleFunc
	positionModuleExport
	positionModuleExportFunc
	positionModuleStart
)

type moduleParser struct {
	// source is the entire WebAssembly text format source code being parsed.
	source []byte

	// module holds the fields incrementally parsed from tokens in the source.
	module *wasm.Module

	// pos is used to give an appropriate errorContext
	pos parserPosition

	// currentModuleField holds the incremental state of a module-scoped field such as an import.
	currentModuleField interface{}

	// typeNamespace represents the function index namespace, which begins with the TypeSection and ends with any
	// inlined type uses which had neither a type index, nor matched an existing type. Elements in the TypeSection can
	// can declare symbolic IDs, such as "$v_v", which are resolved here (without the '$' prefix)
	typeNamespace *indexNamespace

	// typeParser parses "param" and "result" fields in the TypeSection.
	typeParser *typeParser

	// typeParser parses "type", "param" and "result" fields for type uses such as imported or module-defined
	// functions.
	typeUseParser *typeUseParser

	// funcNamespace represents the function index namespace, where any wasm.ImportKindFunc precede the FunctionSection.
	// Non-abbreviated imported and module-defined functions can declare symbolic IDs, such as "$main", which are
	// resolved here (without the '$' prefix).
	funcNamespace *indexNamespace

	// funcParser parses the CodeSection for a given module-defined function.
	funcParser *funcParser

	// unresolvedExports holds any exports whose type index wasn't resolvable when parsed.
	unresolvedExports map[wasm.Index]*wasm.Export
}

// DecodeModule implements wasm.DecodeModule for the WebAssembly 1.0 (MVP) Text Format
// See https://www.w3.org/TR/wasm-core-1/#text-format%E2%91%A0
func DecodeModule(source []byte) (result *wasm.Module, err error) {
	// names are the wasm.Module NameSection
	//
	// * ModuleName: ex. "test" if (module $test)
	// * FunctionNames: nil od no imported or module-defined function had a name
	// * LocalNames: nil when no imported or module-defined function had named (param) fields.
	names := &wasm.NameSection{}
	module := &wasm.Module{NameSection: names}
	p := moduleParser{source: source, module: module,
		typeNamespace: newIndexNamespace(),
		funcNamespace: newIndexNamespace(),
	}
	p.typeParser = newTypeParser(p.typeNamespace, p.onTypeEnd)
	p.typeUseParser = newTypeUseParser(module, p.typeNamespace)
	p.funcParser = newFuncParser(p.endFunc)

	// A valid source must begin with the token '(', but it could be preceded by whitespace or comments. For this
	// reason, we cannot enforce source[0] == '(', and instead need to start the lexer to check the first token.
	line, col, err := lex(p.ensureLParen, p.source)
	if err != nil {
		return nil, &FormatError{line, col, p.errorContext(), err}
	}

	// All identifier contexts are now bound, so resolveTypeUses any uses of symbolic identifiers into concrete indices.
	if err = p.resolveTypeUses(module); err != nil {
		return nil, err
	}
	if err = p.resolveTypeIndices(module); err != nil {
		return nil, err
	}
	if err = p.resolveFunctionIndices(module); err != nil {
		return nil, err
	}

	// Don't set the name section unless we parsed a name!
	if names.ModuleName == "" && names.FunctionNames == nil && names.LocalNames == nil {
		module.NameSection = nil
	}

	return module, nil
}

// ensureLParen errors unless a '(' is found as the text format must start with a field.
func (p *moduleParser) ensureLParen(tok tokenType, tokenBytes []byte, _, _ uint32) (tokenParser, error) {
	if tok != tokenLParen {
		return nil, fmt.Errorf("expected '(', but parsed %s: %s", tok, tokenBytes)
	}
	return p.beginSourceField, nil
}

// beginSourceField returns parseModuleName if the field name is "module".
func (p *moduleParser) beginSourceField(tok tokenType, tokenBytes []byte, _, _ uint32) (tokenParser, error) {
	if tok != tokenKeyword {
		return nil, expectedField(tok)
	}

	if string(tokenBytes) != "module" {
		return nil, unexpectedFieldName(tokenBytes)
	}

	p.pos = positionModule
	return p.parseModuleName, nil
}

// beginModuleField returns a parser according to the module field name (tokenKeyword), or errs if invalid.
func (p *moduleParser) beginModuleField(tok tokenType, tokenBytes []byte, _, _ uint32) (tokenParser, error) {
	if tok == tokenKeyword {
		switch string(tokenBytes) {
		case "type":
			p.pos = positionModuleType
			return p.typeParser.begin, nil
		case "import":
			p.pos = positionModuleImport
			return p.parseImportModule, nil
		case "func":
			p.pos = positionModuleFunc
			return p.parseFuncID, nil
		case "export":
			p.pos = positionModuleExport
			return p.parseExportName, nil
		case "start":
			if p.module.StartSection != nil {
				return nil, errors.New("redundant start")
			}
			p.pos = positionModuleStart
			return p.parseStart, nil
		default:
			return nil, unexpectedFieldName(tokenBytes)
		}
	}
	return nil, expectedField(tok)
}

// parseModuleName records the wasm.NameSection ModuleName, if present, and resumes with parseModule.
//
// Ex. A module name is present `(module $math)`
//                        records math --^
//
// Ex. No module name `(module)`
//   calls parseModule here --^
func (p *moduleParser) parseModuleName(tok tokenType, tokenBytes []byte, line, col uint32) (tokenParser, error) {
	if tok == tokenID { // Ex. $Math
		p.module.NameSection.ModuleName = string(stripDollar(tokenBytes))
		return p.parseModule, nil
	}
	return p.parseModule(tok, tokenBytes, line, col)
}

// parseModule returns beginModuleField on the start of a field '(' or parseUnexpectedTrailingCharacters if the module
// is complete ')'.
func (p *moduleParser) parseModule(tok tokenType, tokenBytes []byte, _, _ uint32) (tokenParser, error) {
	switch tok {
	case tokenID:
		return nil, fmt.Errorf("redundant ID %s", tokenBytes)
	case tokenLParen:
		return p.beginModuleField, nil
	case tokenRParen: // end of module
		p.pos = positionInitial
		return p.parseUnexpectedTrailingCharacters, nil // only one module is allowed and nothing else
	default:
		return nil, unexpectedToken(tok, tokenBytes)
	}
}

// onType adds the current type into the TypeSection and returns parseModule to prepare for the next field.
func (p *moduleParser) onTypeEnd(ft *wasm.FunctionType) tokenParser {
	p.module.TypeSection = append(p.module.TypeSection, ft)
	p.pos = positionModule
	return p.parseModule
}

// parseImportModule returns parseImportName after recording the import module name, or errs if it couldn't be read.
//
// Ex. Imported module name is present `(import "Math" "PI" (func (result f32)))`
//                                records Math --^     ^
//                      parseImportName resumes here --+
//
// Ex. Imported module name is absent `(import (func (result f32)))`
//                                 errs here --^
func (p *moduleParser) parseImportModule(tok tokenType, tokenBytes []byte, _, _ uint32) (tokenParser, error) {
	switch tok {
	case tokenString: // Ex. "" or "Math"
		module := string(tokenBytes[1 : len(tokenBytes)-1]) // unquote
		p.currentModuleField = &wasm.Import{Module: module}
		return p.parseImportName, nil
	case tokenLParen, tokenRParen:
		return nil, errors.New("missing module and name")
	default:
		return nil, unexpectedToken(tok, tokenBytes)
	}
}

// parseImportName returns parseImport after recording the import name, or errs if it couldn't be read.
//
// Ex. Import name is present `(import "Math" "PI" (func (result f32)))`
//                                         starts here --^    ^
//                                           records PI --^   |
//                                 parseImport resumes here --+
//
// Ex. Imported function name is absent `(import "Math" (func (result f32)))`
//                                          errs here --+
func (p *moduleParser) parseImportName(tok tokenType, tokenBytes []byte, _, _ uint32) (tokenParser, error) {
	switch tok {
	case tokenString: // Ex. "" or "PI"
		name := string(tokenBytes[1 : len(tokenBytes)-1]) // unquote
		(p.currentModuleField.(*wasm.Import)).Name = name
		return p.parseImport, nil
	case tokenLParen, tokenRParen:
		return nil, errors.New("missing name")
	default:
		return nil, unexpectedToken(tok, tokenBytes)
	}
}

// parseImport returns beginImportDesc to determine the wasm.ImportKind and dispatch accordingly.
func (p *moduleParser) parseImport(tok tokenType, tokenBytes []byte, _, _ uint32) (tokenParser, error) {
	switch tok {
	case tokenString: // Ex. (import "Math" "PI" "PI"
		return nil, fmt.Errorf("redundant name: %s", tokenBytes[1:len(tokenBytes)-1]) // unquote
	case tokenLParen: // start fields, ex. (func
		return p.beginImportDesc, nil
	case tokenRParen: // end of this import
		return nil, errors.New("missing description field") // Ex. missing (func): (import "Math" "Pi")
	default:
		return nil, unexpectedToken(tok, tokenBytes)
	}
}

// beginImportDesc returns a parser according to the import field name (tokenKeyword), or errs if invalid.
func (p *moduleParser) beginImportDesc(tok tokenType, tokenBytes []byte, _, _ uint32) (tokenParser, error) {
	if tok != tokenKeyword {
		return nil, expectedField(tok)
	}

	switch string(tokenBytes) {
	case "func":
		p.pos = positionModuleImportFunc
		return p.parseImportFuncID, nil
	case "table", "memory", "global":
		return nil, fmt.Errorf("TODO: %s", tokenBytes)
	default:
		return nil, unexpectedFieldName(tokenBytes)
	}
}

// parseImportFuncID records the ID of the current imported function, if present, and resumes with parseImportFunc.
//
// Ex. A function ID is present `(import "Math" "PI" (func $math.pi (result f32))`
//                                  records math.pi here --^
//                                   parseImportFunc resumes here --^
//
// Ex. No function ID `(import "Math" "PI" (func (result f32))`
//                  calls parseImportFunc here --^
func (p *moduleParser) parseImportFuncID(tok tokenType, tokenBytes []byte, line, col uint32) (tokenParser, error) {
	if tok == tokenID { // Ex. $main
		if err := p.setFuncID(tokenBytes); err != nil {
			return nil, err
		}
		return p.parseImportFunc, nil
	}
	return p.parseImportFunc(tok, tokenBytes, line, col)
}

// parseImportFunc passes control to the typeUseParser until any signature is read, then returns onImportFunc.
//
// Ex. `(import "Math" "PI" (func $math.pi (result f32)))`
//                           starts here --^           ^
//                   onImportFunc resumes here --+
//
// Ex. If there is no signature `(import "" "main" (func))`
//                     calls onImportFunc here ---^
func (p *moduleParser) parseImportFunc(tok tokenType, tokenBytes []byte, line, col uint32) (tokenParser, error) {
	idx := wasm.Index(len(p.module.ImportSection))
	if tok == tokenID { // Ex. (func $main $main)
		return nil, fmt.Errorf("redundant ID %s", tokenBytes)
	}

	return p.typeUseParser.begin(wasm.SectionIDImport, idx, p.onImportFunc, tok, tokenBytes, line, col)
}

// onImportFunc records the type index and local names of the current imported function, and increments
// funcNamespace as it is shared across imported and module-defined functions. Finally, this returns parseImportEnd to
// the current import into the ImportSection.
func (p *moduleParser) onImportFunc(typeIdx wasm.Index, paramNames wasm.NameMap, pos onTypeUsePosition, tok tokenType, tokenBytes []byte, line, col uint32) (tokenParser, error) {
	i := p.currentModuleField.(*wasm.Import)
	i.Kind = wasm.ImportKindFunc
	i.DescFunc = typeIdx
	p.addLocalNames(paramNames)

	p.funcNamespace.count++

	switch pos {
	case onTypeUseUnhandledToken:
		return nil, unexpectedToken(tok, tokenBytes)
	case onTypeUseUnhandledField:
		return nil, unexpectedFieldName(tokenBytes)
	case onTypeUseEndField:
		p.pos = positionModuleImport
		return p.parseImportEnd, nil
	}
	return p.parseImportFuncEnd, nil
}

// parseImportEnd adds the current import into the ImportSection and returns parseModule to prepare for the next field.
func (p *moduleParser) parseImportFuncEnd(tok tokenType, tokenBytes []byte, _, _ uint32) (tokenParser, error) {
	if tok != tokenRParen {
		return nil, unexpectedToken(tok, tokenBytes)
	}

	p.pos = positionModuleImport
	return p.parseImportEnd, nil
}

// addLocalNames appends wasm.NameSection LocalNames for the current function.
func (p *moduleParser) addLocalNames(localNames wasm.NameMap) {
	if localNames != nil {
		na := &wasm.NameMapAssoc{Index: p.funcNamespace.count, NameMap: localNames}
		p.module.NameSection.LocalNames = append(p.module.NameSection.LocalNames, na)
	}
}

// parseImportEnd adds the current import into the ImportSection and returns parseModule to prepare for the next field.
func (p *moduleParser) parseImportEnd(tok tokenType, tokenBytes []byte, _, _ uint32) (tokenParser, error) {
	if tok != tokenRParen {
		return nil, unexpectedToken(tok, tokenBytes)
	}

	p.module.ImportSection = append(p.module.ImportSection, p.currentModuleField.(*wasm.Import))
	p.currentModuleField = nil
	p.pos = positionModule
	return p.parseModule, nil
}

// parseFuncID records the ID of the current function, if present, and resumes with parseFunc.
//
// Ex. A function ID is present `(module (func $math.pi (result f32))`
//                      records math.pi here --^
//                             parseFunc resumes here --^
//
// Ex. No function ID `(module (func (result f32))`
//            calls parseFunc here --^
func (p *moduleParser) parseFuncID(tok tokenType, tokenBytes []byte, line, col uint32) (tokenParser, error) {
	if tok == tokenID { // Ex. $main
		if err := p.setFuncID(tokenBytes); err != nil {
			return nil, err
		}
		return p.parseFunc, nil
	}
	return p.parseFunc(tok, tokenBytes, line, col)
}

// setFuncID adds the normalized ('$' stripped) function ID to the funcNamespace and the wasm.NameSection.
func (p *moduleParser) setFuncID(idToken []byte) error {
	id, err := p.funcNamespace.setID(idToken)
	if err != nil {
		return err
	}
	na := &wasm.NameAssoc{Index: p.funcNamespace.count, Name: id}
	p.module.NameSection.FunctionNames = append(p.module.NameSection.FunctionNames, na)
	return nil
}

// parseFunc passes control to the typeUseParser until any signature is read, then funcParser until and locals or body
// are read. Finally, this finishes via endFunc.
//
// Ex. `(module (func $math.pi (result f32))`
//               starts here --^           ^
//                  endFunc resumes here --+
//
// Ex.    `(module (func $math.pi (result f32) (local i32) )`
//                  starts here --^            ^           ^
//             funcParser.begin resumes here --+           |
//                                  endFunc resumes here --+
//
// Ex. If there is no signature `(func)`
//              calls endFunc here ---^
func (p *moduleParser) parseFunc(tok tokenType, tokenBytes []byte, line, col uint32) (tokenParser, error) {
	idx := wasm.Index(len(p.module.FunctionSection))
	if tok == tokenID { // Ex. (func $main $main)
		return nil, fmt.Errorf("redundant ID %s", tokenBytes)
	}

	return p.typeUseParser.begin(wasm.SectionIDFunction, idx, p.funcParser.begin, tok, tokenBytes, line, col)
}

// endFunc adds the type index, code and local names for the current function, and increments funcNamespace as it is
// shared across imported and module-defined functions. Finally, this returns parseModule to prepare for the next field.
func (p *moduleParser) endFunc(typeIdx wasm.Index, code *wasm.Code, localNames wasm.NameMap) (tokenParser, error) {
	p.module.FunctionSection = append(p.module.FunctionSection, typeIdx)
	p.module.CodeSection = append(p.module.CodeSection, code)
	p.addLocalNames(localNames)

	// Multiple funcs are allowed, so advance in case there's a next.
	p.funcNamespace.count++
	p.pos = positionModule
	return p.parseModule, nil
}

// parseExportName returns parseExport after recording the export name, or errs if it couldn't be read.
//
// Ex. Export name is present `(export "PI" (func 0))`
//                       starts here --^    ^
//                         records PI --^   |
//               parseExport resumes here --+
//
// Ex. Export name is absent `(export (func 0))`
//                        errs here --^
func (p *moduleParser) parseExportName(tok tokenType, tokenBytes []byte, _, _ uint32) (tokenParser, error) {
	switch tok {
	case tokenString: // Ex. "" or "PI"
		name := string(tokenBytes[1 : len(tokenBytes)-1]) // strip quotes
		if _, ok := p.module.ExportSection[name]; ok {
			return nil, fmt.Errorf("duplicate name %q", name)
		}
		p.currentModuleField = &wasm.Export{Name: name}
		return p.parseExport, nil
	case tokenLParen, tokenRParen:
		return nil, errors.New("missing name")
	default:
		return nil, unexpectedToken(tok, tokenBytes)
	}
}

// parseExport returns beginExportDesc to determine the wasm.ExportKind and dispatch accordingly.
func (p *moduleParser) parseExport(tok tokenType, tokenBytes []byte, _, _ uint32) (tokenParser, error) {
	switch tok {
	case tokenString: // Ex. (export "PI" "PI"
		return nil, fmt.Errorf("redundant name: %s", tokenBytes[1:len(tokenBytes)-1]) // unquote
	case tokenLParen: // start fields, ex. (func
		return p.beginExportDesc, nil
	case tokenRParen: // end of this export
		return nil, errors.New("missing description field") // Ex. missing (func): (export "Math" "Pi")
	default:
		return nil, unexpectedToken(tok, tokenBytes)
	}
}

// beginExportDesc returns a parser according to the export field name (tokenKeyword), or errs if invalid.
func (p *moduleParser) beginExportDesc(tok tokenType, tokenBytes []byte, _, _ uint32) (tokenParser, error) {
	if tok != tokenKeyword {
		return nil, expectedField(tok)
	}

	switch string(tokenBytes) {
	case "func":
		p.pos = positionModuleExportFunc
		return p.parseExportFunc, nil
	case "table", "memory", "global":
		return nil, fmt.Errorf("TODO: %s", tokenBytes)
	default:
		return nil, unexpectedFieldName(tokenBytes)
	}
}

// parseExportFunc records the symbolic or numeric function index the function export
func (p *moduleParser) parseExportFunc(tok tokenType, tokenBytes []byte, line, col uint32) (tokenParser, error) {
	eIdx := wasm.Index(len(p.module.ExportSection))
	typeIdx, resolved, err := p.funcNamespace.parseIndex(wasm.SectionIDExport, eIdx, tok, tokenBytes, line, col)
	if err != nil {
		return nil, err
	}
	e := p.currentModuleField.(*wasm.Export)
	e.Kind = wasm.ExportKindFunc
	e.Index = typeIdx

	// All sections in wasm.Module are numeric indices except exports. Hence, we have to special-case here
	if !resolved {
		if p.unresolvedExports == nil {
			p.unresolvedExports = map[wasm.Index]*wasm.Export{eIdx: e}
		} else {
			p.unresolvedExports[eIdx] = e
		}
	}
	return p.parseExportFuncEnd, nil
}

// parseExportFuncEnd returns parseExportEnd to add the current export.
func (p *moduleParser) parseExportFuncEnd(tok tokenType, tokenBytes []byte, _, _ uint32) (tokenParser, error) {
	switch tok {
	case tokenUN, tokenID:
		return nil, errors.New("redundant index")
	case tokenRParen:
		p.pos = positionModuleExport
		return p.parseExportEnd, nil
	default:
		return nil, unexpectedToken(tok, tokenBytes)
	}
}

// parseImportEnd adds the current export into the ExportSection and returns parseModule to prepare for the next field.
func (p *moduleParser) parseExportEnd(tok tokenType, tokenBytes []byte, _, _ uint32) (tokenParser, error) {
	if tok != tokenRParen {
		return nil, unexpectedToken(tok, tokenBytes)
	}

	e := p.currentModuleField.(*wasm.Export)
	p.currentModuleField = nil
	if p.module.ExportSection == nil {
		p.module.ExportSection = map[string]*wasm.Export{e.Name: e}
	} else {
		p.module.ExportSection[e.Name] = e
	}
	p.pos = positionModule
	return p.parseModule, nil
}

// parseStart returns parseStartEnd after recording the start function index, or errs if it couldn't be read.
func (p *moduleParser) parseStart(tok tokenType, tokenBytes []byte, line, col uint32) (tokenParser, error) {
	idx, _, err := p.funcNamespace.parseIndex(wasm.SectionIDStart, 0, tok, tokenBytes, line, col)
	if err != nil {
		return nil, err
	}
	p.module.StartSection = &idx
	return p.parseStartEnd, nil
}

// parseStartEnd returns parseModule to prepare for the next field.
func (p *moduleParser) parseStartEnd(tok tokenType, tokenBytes []byte, _, _ uint32) (tokenParser, error) {
	switch tok {
	case tokenUN, tokenID:
		return nil, errors.New("redundant index")
	case tokenRParen:
		p.pos = positionModule
		return p.parseModule, nil
	default:
		return nil, unexpectedToken(tok, tokenBytes)
	}
}

func (p *moduleParser) parseUnexpectedTrailingCharacters(_ tokenType, tokenBytes []byte, _, _ uint32) (tokenParser, error) {
	return nil, fmt.Errorf("unexpected trailing characters: %s", tokenBytes)
}

// resolveTypeIndices ensures any indices point are numeric or returns a FormatError if they cannot be bound.
func (p *moduleParser) resolveTypeIndices(module *wasm.Module) error {
	for _, unresolved := range p.typeNamespace.unresolvedIndices {
		target, err := p.typeNamespace.resolve(unresolved)
		if err != nil {
			return err
		}
		switch unresolved.section {
		case wasm.SectionIDImport:
			module.ImportSection[unresolved.idx].DescFunc = target
		case wasm.SectionIDFunction:
			module.FunctionSection[unresolved.idx] = target
		default:
			panic(unhandledSection(unresolved.section))
		}
	}
	return nil
}

// resolveFunctionIndices ensures any indices point are numeric or returns a FormatError if they cannot be bound.
func (p *moduleParser) resolveFunctionIndices(module *wasm.Module) error {
	for _, unresolved := range p.funcNamespace.unresolvedIndices {
		target, err := p.funcNamespace.resolve(unresolved)
		if err != nil {
			return err
		}
		switch unresolved.section {
		case wasm.SectionIDStart:
			module.StartSection = &target
		case wasm.SectionIDExport:
			p.unresolvedExports[unresolved.idx].Index = target
		default:
			panic(unhandledSection(unresolved.section))
		}
	}
	return nil
}

// resolveTypeUses adds any missing inlined types, resolving any type indexes in the FunctionSection or ImportSection.
// This errs if any type index is unresolved, out of range or mismatches an inlined type use signature.
func (p *moduleParser) resolveTypeUses(module *wasm.Module) error {
	inlinedToRealIdx := p.addInlinedTypes()
	return p.resolveInlined(module, inlinedToRealIdx)
}

func (p *moduleParser) resolveInlined(module *wasm.Module, inlinedToRealIdx map[wasm.Index]wasm.Index) error {
	// Now look for all the uses of the inlined types and apply the mapping above
	for _, i := range p.typeUseParser.inlinedTypeIndices {
		switch i.section {
		case wasm.SectionIDImport:
			if i.typePos == nil {
				module.ImportSection[i.idx].DescFunc = inlinedToRealIdx[i.inlinedIdx]
				continue
			}

			typeIdx := module.ImportSection[i.idx].DescFunc
			if err := p.requireInlinedMatchesReferencedType(module.TypeSection, typeIdx, i); err != nil {
				return err
			}
		case wasm.SectionIDFunction:
			if i.typePos == nil {
				module.FunctionSection[i.idx] = inlinedToRealIdx[i.inlinedIdx]
				continue
			}

			typeIdx := module.FunctionSection[i.idx]
			if err := p.requireInlinedMatchesReferencedType(module.TypeSection, typeIdx, i); err != nil {
				return err
			}
		default:
			panic(unhandledSection(i.section))
		}
	}
	return nil
}

func (p *moduleParser) requireInlinedMatchesReferencedType(typeSection []*wasm.FunctionType, typeIdx wasm.Index, i *inlinedTypeIndex) error {
	inlined := p.typeUseParser.inlinedTypes[i.inlinedIdx]
	if err := requireInlinedMatchesReferencedType(typeSection, typeIdx, inlined.Params, inlined.Results); err != nil {
		var context string
		switch i.section {
		case wasm.SectionIDImport:
			context = fmt.Sprintf("module.import[%d].func", i.idx)
		case wasm.SectionIDFunction:
			context = fmt.Sprintf("module.func[%d]", i.idx)
		default:
			panic(unhandledSection(i.section))
		}
		return &FormatError{Line: i.typePos.line, Col: i.typePos.col, Context: context, cause: err}
	}
	return nil
}

// addInlinedTypes adds any inlined types missing from the module TypeSection and returns an index mapping the inlined
// index to real index in the TypeSection. This avoids adding or looking up a type twice when it has multiple type uses.
func (p *moduleParser) addInlinedTypes() map[wasm.Index]wasm.Index {
	inlinedTypeCount := len(p.typeUseParser.inlinedTypes)
	if inlinedTypeCount == 0 {
		return nil
	}

	inlinedToRealIdx := make(map[wasm.Index]wasm.Index, inlinedTypeCount)
INLINED:
	for idx, inlined := range p.typeUseParser.inlinedTypes {
		inlinedIdx := wasm.Index(idx)

		// A type can be defined after its type use. Ex. (module (func (param i32)) (type (func (param i32)))
		// This uses an inner loop to avoid creating a large map for an edge case.
		for realIdx, t := range p.module.TypeSection {
			if funcTypeEquals(t, inlined.Params, inlined.Results) {
				inlinedToRealIdx[inlinedIdx] = wasm.Index(realIdx)
				continue INLINED
			}
		}

		// When we get here, this means the inlined type is not in the TypeSection, yet, so add it.
		inlinedToRealIdx[inlinedIdx] = p.typeNamespace.count
		p.module.TypeSection = append(p.module.TypeSection, inlined)
		p.typeNamespace.count++
	}
	return inlinedToRealIdx
}

func (p *moduleParser) errorContext() string {
	switch p.pos {
	case positionInitial:
		return ""
	case positionModule:
		return "module"
	case positionModuleType:
		idx := wasm.Index(len(p.module.TypeSection))
		return fmt.Sprintf("module.type[%d]%s", idx, p.typeParser.errorContext())
	case positionModuleImport, positionModuleImportFunc: // TODO: table, memory or global
		idx := wasm.Index(len(p.module.ImportSection))
		if p.pos == positionModuleImport {
			return fmt.Sprintf("module.import[%d]", idx)
		}
		return fmt.Sprintf("module.import[%d].func%s", idx, p.typeUseParser.errorContext())
	case positionModuleFunc:
		idx := wasm.Index(len(p.module.FunctionSection))
		return fmt.Sprintf("module.func[%d]%s", idx, p.typeUseParser.errorContext())
	case positionModuleExport, positionModuleExportFunc: // TODO: table, memory or global
		idx := wasm.Index(len(p.module.ExportSection))
		if p.pos == positionModuleExport {
			return fmt.Sprintf("module.export[%d]", idx)
		}
		return fmt.Sprintf("module.export[%d].func", idx)
	case positionModuleStart:
		return "module.start"
	default: // parserPosition is an enum, we expect to have handled all cases above. panic if we didn't
		panic(fmt.Errorf("BUG: unhandled parsing state on errorContext: %v", p.pos))
	}
}

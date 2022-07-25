package internal

import (
	"errors"
	"fmt"

	"github.com/tetratelabs/wazero/internal/wasm"
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
	positionImport
	positionImportFunc
	positionMemory
	positionExport
	positionExportFunc
	positionExportMemory
	positionStart
)

type callbackPosition byte

const (
	// callbackPositionUnhandledToken is set on a token besides a paren.
	callbackPositionUnhandledToken callbackPosition = iota
	// callbackPositionUnhandledField is at the field name (tokenKeyword) which isn't "type", "param" or "result"
	callbackPositionUnhandledField
	// callbackPositionEndField is at the end (tokenRParen) of the field enclosing the type use.
	callbackPositionEndField
)

// moduleParser parses a single api.Module from WebAssembly 1.0 (20191205) Text format.
//
// Note: The indexNamespace of wasm.SectionIDMemory and wasm.SectionIDTable allow up-to-one item. For example, you
// cannot define both one import and one module-defined memory, rather one or the other (or none). Even if these rules
// are also enforced in module instantiation, they are also enforced here, to allow relevant source line/col in errors.
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#modules%E2%91%A3
type moduleParser struct {
	// enabledFeatures ensure parsing errs at the correct line and column number when a feature is disabled.
	enabledFeatures wasm.Features

	// source is the entire WebAssembly text format source code being parsed.
	source []byte

	// module holds the fields incrementally parsed from tokens in the source.
	module *wasm.Module

	// pos is used to give an appropriate errorContext
	pos parserPosition

	// currentModuleField holds the incremental state of a module-scoped field such as an import.
	currentModuleField interface{}

	// typeNamespace represents the function type index namespace, which begins with the TypeSection and ends with any
	// inlined type uses which had neither a type index, nor matched an existing type. Elements in the TypeSection can
	// can declare symbolic IDs, such as "$v_v", which are resolved here (without the '$' prefix)
	typeNamespace *indexNamespace

	// typeParser parses "param" and "result" fields in the TypeSection.
	typeParser *typeParser

	// typeParser parses "type", "param" and "result" fields for type uses such as imported or module-defined
	// functions.
	typeUseParser *typeUseParser

	// funcNamespace represents the function index namespace, which begins with any wasm.ExternTypeFunc in the
	// wasm.SectionIDImport followed by the wasm.SectionIDFunction.
	//
	// Non-abbreviated imported and module-defined functions can declare symbolic IDs, such as "$main", which are
	// resolved here (without the '$' prefix).
	funcNamespace *indexNamespace

	// funcParser parses the CodeSection for a given module-defined function.
	funcParser *funcParser

	// memoryNamespace represents the memory index namespace, which begins with any wasm.ExternTypeMemory in
	// the wasm.SectionIDImport followed by the wasm.SectionIDMemory.
	//
	// Non-abbreviated imported and module-defined memories can declare symbolic IDs, such as "$mem", which are resolved
	// here (without the '$' prefix).
	memoryNamespace *indexNamespace

	// memoryParser parses the MemorySection for a given module-defined memory.
	memoryParser *memoryParser

	// unresolvedExports holds any exports whose type index wasn't resolvable when parsed.
	unresolvedExports map[wasm.Index]*wasm.Export

	// field counts can be different from the count in a section when abbreviated imports exist. To give an accurate
	// errorContext, we count explicitly.
	fieldCountFunc uint32

	exportedName map[string]struct{}
}

// DecodeModule implements wasm.DecodeModule for the WebAssembly 1.0 (20191205) Text Format
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#text-format%E2%91%A0
func DecodeModule(
	source []byte,
	enabledFeatures wasm.Features,
	memorySizer func(minPages uint32, maxPages *uint32) (min, capacity, max uint32),
) (module *wasm.Module, err error) {
	// TODO: when globals are supported, err on global vars if disabled

	// names are the wasm.Module NameSection
	//
	// * ModuleName: ex. "test" if (module $test)
	// * FunctionNames: nil when neither imported nor module-defined functions had a name
	// * LocalNames: nil when neither imported nor module-defined functions had named (param) fields.
	names := &wasm.NameSection{}
	module = &wasm.Module{NameSection: names}
	p := newModuleParser(module, enabledFeatures, memorySizer)
	p.source = source

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
	return
}

func newModuleParser(
	module *wasm.Module,
	enabledFeatures wasm.Features,
	memorySizer func(minPages uint32, maxPages *uint32) (min, capacity, max uint32),
) *moduleParser {
	p := moduleParser{module: module, enabledFeatures: enabledFeatures,
		typeNamespace:   newIndexNamespace(module.SectionElementCount),
		funcNamespace:   newIndexNamespace(module.SectionElementCount),
		memoryNamespace: newIndexNamespace(module.SectionElementCount),
	}
	p.typeParser = newTypeParser(enabledFeatures, p.typeNamespace, p.onTypeEnd)
	p.typeUseParser = newTypeUseParser(enabledFeatures, module, p.typeNamespace)
	p.funcParser = newFuncParser(enabledFeatures, p.typeUseParser, p.funcNamespace, p.endFunc)
	p.memoryParser = newMemoryParser(memorySizer, p.memoryNamespace, p.endMemory)
	return &p
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
			p.pos = positionType
			return p.typeParser.begin, nil
		case "import":
			p.pos = positionImport
			return p.parseImportModule, nil
		case wasm.ExternTypeFuncName:
			p.pos = positionFunc
			return p.funcParser.begin, nil
		case wasm.ExternTypeTableName:
			return nil, fmt.Errorf("TODO: %s", tokenBytes)
		case wasm.ExternTypeMemoryName:
			if p.module.SectionElementCount(wasm.SectionIDMemory) > 0 {
				return nil, moreThanOneInvalidInSection(wasm.SectionIDMemory)
			}
			p.pos = positionMemory
			return p.memoryParser.begin, nil
		case wasm.ExternTypeGlobalName:
			return nil, fmt.Errorf("TODO: %s", tokenBytes)
		case "export":
			p.pos = positionExport
			return p.parseExportName, nil
		case "start":
			if p.module.SectionElementCount(wasm.SectionIDStart) > 0 {
				return nil, moreThanOneInvalidInSection(wasm.SectionIDStart)
			}
			p.pos = positionStart
			return p.parseStart, nil
		case "elem":
			return nil, errors.New("TODO: elem")
		case "data":
			return nil, errors.New("TODO: data")
		default:
			return nil, unexpectedFieldName(tokenBytes)
		}
	}
	return nil, expectedField(tok)
}

// parseModuleName records the wasm.NameSection ModuleName, if present, and resumes with parseModule.
//
// Ex. A module name is present `(module $math)`
//
//	records math --^
//
// Ex. No module name `(module)`
//
//	calls parseModule here --^
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
//
//	          records Math --^     ^
//	parseImportName resumes here --+
//
// Ex. Imported module name is absent `(import (func (result f32)))`
//
//	errs here --^
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
//
//	        starts here --^^   ^
//	          records PI --+   |
//	parseImport resumes here --+
//
// Ex. Imported function name is absent `(import "Math" (func (result f32)))`
//
//	errs here --+
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

// parseImport returns beginImportDesc to determine the wasm.ExternType and dispatch accordingly.
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
	case wasm.ExternTypeFuncName:
		if p.module.SectionElementCount(wasm.SectionIDFunction) > 0 {
			return nil, importAfterModuleDefined(wasm.SectionIDFunction)
		}
		p.pos = positionImportFunc
		return p.parseImportFuncID, nil
	case wasm.ExternTypeTableName, wasm.ExternTypeMemoryName, wasm.ExternTypeGlobalName:
		return nil, fmt.Errorf("TODO: %s", tokenBytes)
	default:
		return nil, unexpectedFieldName(tokenBytes)
	}
}

// parseImportFuncID records the ID of the current imported function, if present, and resumes with parseImportFunc.
//
// Ex. A function ID is present `(import "Math" "PI" (func $math.pi (result f32))`
//
//	records math.pi here --^
//	 parseImportFunc resumes here --^
//
// Ex. No function ID `(import "Math" "PI" (func (result f32))`
//
//	calls parseImportFunc here --^
func (p *moduleParser) parseImportFuncID(tok tokenType, tokenBytes []byte, line, col uint32) (tokenParser, error) {
	if tok == tokenID { // Ex. $main
		if name, err := p.funcNamespace.setID(tokenBytes); err != nil {
			return nil, err
		} else {
			p.addFunctionName(name)
		}
		return p.parseImportFunc, nil
	}
	return p.parseImportFunc(tok, tokenBytes, line, col)
}

// addFunctionName appends the current imported or module-defined function name to the wasm.NameSection iff it is not
// empty.
func (p *moduleParser) addFunctionName(name string) {
	if name == "" {
		return // there's no value in an empty name
	}
	na := &wasm.NameAssoc{Index: p.funcNamespace.count, Name: name}
	p.module.NameSection.FunctionNames = append(p.module.NameSection.FunctionNames, na)
}

// parseImportFunc passes control to the typeUseParser until any signature is read, then returns onImportFunc.
//
// Ex. `(import "Math" "PI" (func $math.pi (result f32)))`
//
//	        starts here --^           ^
//	onImportFunc resumes here --+
//
// Ex. If there is no signature `(import "" "main" (func))`
//
//	calls onImportFunc here ---^
func (p *moduleParser) parseImportFunc(tok tokenType, tokenBytes []byte, line, col uint32) (tokenParser, error) {
	if tok == tokenID { // Ex. (func $main $main)
		return nil, fmt.Errorf("redundant ID %s", tokenBytes)
	}
	return p.typeUseParser.begin(wasm.SectionIDImport, p.onImportFunc, tok, tokenBytes, line, col)
}

// onImportFunc records the type index and local names of the current imported function, and increments
// funcNamespace as it is shared across imported and module-defined functions. Finally, this returns parseImportEnd to
// the current import into the ImportSection.
func (p *moduleParser) onImportFunc(typeIdx wasm.Index, paramNames wasm.NameMap, pos callbackPosition, tok tokenType, tokenBytes []byte, _, _ uint32) (tokenParser, error) {
	i := p.currentModuleField.(*wasm.Import)
	i.Type = wasm.ExternTypeFunc
	i.DescFunc = typeIdx
	p.addLocalNames(paramNames)

	p.funcNamespace.count++

	switch pos {
	case callbackPositionUnhandledToken:
		return nil, unexpectedToken(tok, tokenBytes)
	case callbackPositionUnhandledField:
		return nil, unexpectedFieldName(tokenBytes)
	case callbackPositionEndField:
		p.pos = positionImport
		return p.parseImportEnd, nil
	}
	return p.parseImportFuncEnd, nil
}

// parseImportEnd adds the current import into the ImportSection and returns parseModule to prepare for the next field.
func (p *moduleParser) parseImportFuncEnd(tok tokenType, tokenBytes []byte, _, _ uint32) (tokenParser, error) {
	if tok != tokenRParen {
		return nil, unexpectedToken(tok, tokenBytes)
	}

	p.pos = positionImport
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

// endFunc adds the type index, code and local names for the current function, and increments funcNamespace as it is
// shared across imported and module-defined functions. Finally, this returns parseModule to prepare for the next field.
func (p *moduleParser) endFunc(typeIdx wasm.Index, code *wasm.Code, name string, localNames wasm.NameMap) (tokenParser, error) {
	p.addFunctionName(name)
	p.module.FunctionSection = append(p.module.FunctionSection, typeIdx)
	p.module.CodeSection = append(p.module.CodeSection, code)
	p.addLocalNames(localNames)

	// Multiple funcs are allowed, so advance in case there's a next.
	p.funcNamespace.count++
	p.fieldCountFunc++
	p.pos = positionModule
	return p.parseModule, nil
}

// endMemory adds the limits for the current memory, and increments memoryNamespace as it is shared across imported and
// module-defined memories. Finally, this returns parseModule to prepare for the next field.
func (p *moduleParser) endMemory(mem *wasm.Memory) tokenParser {
	p.module.MemorySection = mem
	p.pos = positionModule
	return p.parseModule
}

// parseExportName returns parseExport after recording the export name, or errs if it couldn't be read.
//
// Ex. Export name is present `(export "PI" (func 0))`
//
//	        starts here --^    ^
//	          records PI --^   |
//	parseExport resumes here --+
//
// Ex. Export name is absent `(export (func 0))`
//
//	errs here --^
func (p *moduleParser) parseExportName(tok tokenType, tokenBytes []byte, _, _ uint32) (tokenParser, error) {
	switch tok {
	case tokenString: // Ex. "" or "PI"
		name := string(tokenBytes[1 : len(tokenBytes)-1]) // strip quotes
		if p.exportedName == nil {
			p.exportedName = map[string]struct{}{}
		}
		if _, ok := p.exportedName[name]; ok {
			return nil, fmt.Errorf("%q already exported", name)
		} else {
			p.exportedName[name] = struct{}{}
		}
		p.currentModuleField = &wasm.Export{Name: name}
		return p.parseExport, nil
	case tokenLParen, tokenRParen:
		return nil, errors.New("missing name")
	default:
		return nil, unexpectedToken(tok, tokenBytes)
	}
}

// parseExport returns beginExportDesc to determine the wasm.ExternType and dispatch accordingly.
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
	case wasm.ExternTypeFuncName:
		p.pos = positionExportFunc
		return p.parseExportDesc, nil
	case wasm.ExternTypeMemoryName:
		p.pos = positionExportMemory
		return p.parseExportDesc, nil
	case wasm.ExternTypeTableName, wasm.ExternTypeGlobalName:
		return nil, fmt.Errorf("TODO: %s", tokenBytes)
	default:
		return nil, unexpectedFieldName(tokenBytes)
	}
}

// parseExportDesc records the symbolic or numeric function index of the export target
func (p *moduleParser) parseExportDesc(tok tokenType, tokenBytes []byte, line, col uint32) (tokenParser, error) {
	var namespace *indexNamespace
	e := p.currentModuleField.(*wasm.Export)
	switch p.pos {
	case positionExportFunc:
		e.Type = wasm.ExternTypeFunc
		namespace = p.funcNamespace
	case positionExportMemory:
		e.Type = wasm.ExternTypeMemory
		namespace = p.memoryNamespace
	default:
		panic(fmt.Errorf("BUG: unhandled parsing state on parseExportDesc: %v", p.pos))
	}
	typeIdx, resolved, err := namespace.parseIndex(wasm.SectionIDExport, 0, tok, tokenBytes, line, col)
	if err != nil {
		return nil, err
	}
	e.Index = typeIdx

	// All sections in wasm.Module are numeric indices except exports. Hence, we have to special-case here
	if !resolved {
		eIdx := p.module.SectionElementCount(wasm.SectionIDExport)
		if p.unresolvedExports == nil {
			p.unresolvedExports = map[wasm.Index]*wasm.Export{eIdx: e}
		} else {
			p.unresolvedExports[eIdx] = e
		}
	}
	return p.parseExportDescEnd, nil
}

// parseExportFuncEnd returns parseExportEnd to add the current export.
func (p *moduleParser) parseExportDescEnd(tok tokenType, tokenBytes []byte, _, _ uint32) (tokenParser, error) {
	switch tok {
	case tokenUN, tokenID:
		return nil, errors.New("redundant index")
	case tokenRParen:
		p.pos = positionExport
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
	p.module.ExportSection = append(p.module.ExportSection, e)
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
		case wasm.SectionIDCode:
			if target > 255 {
				return errors.New("TODO: unresolved function indexes that don't fit in a byte")
			}
			module.CodeSection[unresolved.idx].Body[unresolved.bodyOffset] = byte(target)
		case wasm.SectionIDExport:
			p.unresolvedExports[unresolved.idx].Index = target
		case wasm.SectionIDStart:
			module.StartSection = &target
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
	return p.resolveInlinedTypes(module, inlinedToRealIdx)
}

func (p *moduleParser) resolveInlinedTypes(module *wasm.Module, inlinedToRealIdx map[wasm.Index]wasm.Index) error {
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
			if t.EqualsSignature(inlined.Params, inlined.Results) {
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
	case positionType:
		idx := p.module.SectionElementCount(wasm.SectionIDType)
		return fmt.Sprintf("module.type[%d]%s", idx, p.typeParser.errorContext())
	case positionImport, positionImportFunc: // TODO: table, memory or global
		idx := p.module.SectionElementCount(wasm.SectionIDImport)
		if p.pos == positionImport {
			return fmt.Sprintf("module.import[%d]", idx)
		}
		return fmt.Sprintf("module.import[%d].%s%s", idx, wasm.ExternTypeFuncName, p.typeUseParser.errorContext())
	case positionFunc:
		idx := p.fieldCountFunc
		return fmt.Sprintf("module.%s[%d]%s", wasm.ExternTypeFuncName, idx, p.typeUseParser.errorContext())
	case positionMemory:
		return fmt.Sprintf("module.%s[0]", wasm.ExternTypeMemoryName)
	case positionExport, positionExportFunc: // TODO: table, memory or global
		idx := p.module.SectionElementCount(wasm.SectionIDExport)
		if p.pos == positionExport {
			return fmt.Sprintf("module.export[%d]", idx)
		}
		return fmt.Sprintf("module.export[%d].%s", idx, wasm.ExternTypeFuncName)
	case positionStart:
		return "module.start"
	default: // parserPosition is an enum, we expect to have handled all cases above. panic if we didn't
		panic(fmt.Errorf("BUG: unhandled parsing state on errorContext: %v", p.pos))
	}
}

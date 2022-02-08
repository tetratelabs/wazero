package text

import (
	"errors"
	"fmt"

	"github.com/tetratelabs/wazero/wasm"
)

func newAbbreviationParser(module *wasm.Module, indexNamespace *indexNamespace, onAbbreviations onAbbreviations) *abbreviationParser {
	p := &abbreviationParser{module: module, indexNamespace: indexNamespace, onAbbreviations: onAbbreviations}
	sectionID := indexNamespace.sectionID
	switch sectionID {
	case wasm.SectionIDFunction:
		p.importKind = wasm.ImportKindFunc
		p.exportKind = wasm.ExportKindFunc
	case wasm.SectionIDTable:
		p.importKind = wasm.ImportKindTable
		p.exportKind = wasm.ExportKindTable
	case wasm.SectionIDMemory:
		p.importKind = wasm.ImportKindMemory
		p.exportKind = wasm.ExportKindMemory
	case wasm.SectionIDGlobal:
		p.importKind = wasm.ImportKindGlobal
		p.exportKind = wasm.ExportKindGlobal
	default:
		panic(fmt.Errorf("BUG: %s is not a section that can have abbreviations", wasm.SectionIDName(sectionID)))
	}
	return p
}

// onAbbreviations is invoked when the grammar "(export)* (import)?" completes.
//
// * name is the tokenID field stripped of '$' prefix
// * i is nil unless there was only one "import" field. When set, this includes possibly empty Module and Name fields.
// * pos is the context used to determine which tokenParser to return
//
// Note: this is called when neither an "import" nor "export" field are parsed, or on any subsequent field that is
// neither "import" nor "export": pos clarifies this.
//
// Note: Any Exports are already added to the wasm.Module ExportSection.
type onAbbreviations func(name string, i *wasm.Import, pos callbackPosition, tok tokenType, tokenBytes []byte, line, col uint32) (tokenParser, error)

// abbreviationParser parses any abbreviated imports or exports from a field such "func" and calls onAbbreviations.
//
// Ex.     `(func $math.pi (import "Math" "PI"))`
//   begin here --^                            ^
//              onAbbreviations resumes here --+
//
// Note: Unlike normal parsers, this is not used for an entire field (enclosed by parens). Rather, this only handles
// "export" and "import" inner fields in the correct order.
// Note: abbreviationParser is reusable. The caller resets via begin.
type abbreviationParser struct {
	// module is used for the following purposes and allows abbreviation errors to attach the correct source line/col:
	//  * This parser eagerly adds exports into the ExportSection to ensure uniqueness
	//  * When the section corresponding with indexNamespace.sectionID is non-empty, imports are out of order.
	module *wasm.Module

	// indexNamespace stores IDs parsed, and also allows uniqueness validation. Moreover, the count is the export index.
	indexNamespace *indexNamespace

	// importKind is set according to indexNamespace.sectionID and allows pre-population of import fields.
	importKind wasm.ImportKind

	// exportKind is set according to indexNamespace.sectionID and allows pre-population of export fields.
	exportKind wasm.ExportKind

	// onAbbreviations is invoked on end
	onAbbreviations onAbbreviations

	// pos is used to give an appropriate errorContext
	pos parserPosition

	// currentName is a tokenID field stripped of the leading '$'.
	//
	// Note: this for the wasm.NameSection, which has no reason to differentiate empty string from no name.
	currentName string

	// currentImport is the inlined import field
	//
	// Note: multiple inlined imports are not supported as their expansion would be ambiguous
	// See https://github.com/WebAssembly/spec/issues/1418
	currentImport *wasm.Import
}

// begin must be called inside a field that may contain an abbreviated export or import. This begins by reading any ID.
// Following that, it parses any number of "export" fields and up to one "import". Finally, this ends onAbbreviations or
// error.
//
// Ex. Given the source `(module (func $main (export "a")))`
//          sets the ID to main here --^      ^          ^
//          beginExportOrImport starts here --+          |
//                        onAbbreviations resumes here --+
//
// Ex. Given the source `(module (func (import "" "") (param i32)`
//   beginExportOrImport starts here --^              ^
//                     onAbbreviations resumes here --+
//
func (p *abbreviationParser) begin(tok tokenType, tokenBytes []byte, line, col uint32) (tokenParser, error) {
	pos := callbackPositionUnhandledToken
	p.pos = positionInitial // to ensure errorContext reports properly
	switch tok {
	case tokenID: // Ex. $main
		if err := p.setID(tokenBytes); err != nil {
			return nil, err
		}
		return p.parseMoreExportsOrImport, nil
	case tokenLParen:
		return p.beginExportOrImport, nil
	case tokenRParen:
		pos = callbackPositionEndField
	}
	return p.onAbbreviations("", nil, pos, tok, tokenBytes, line, col)
}

// setID adds the current ID into the indexNamespace. This errs when the ID was already in use.
//
// Note: Due to abbreviated syntax, `(func $main...` could later be found to be an import. Ex. `(func $main (import...`
// In other words, it isn't known if what's being parsed is module-defined vs import, and the latter could have an
// ordering error. The ordering constraint imposed on "module composition" is that abbreviations are well-formed if and
// only if their expansions are. This means `(module (func $one) (func $two (import "" "")))` is invalid as it expands
// to `(module (func $one) (import "" "" (func $two)))` which violates the ordering constraint of imports first.
//
// We may set an ID here and find that the function declaration is invalid later due to above. Should we save off the
// source position? No: this function only ensures there's no ID conflict: an error here is about reuse of an ID. It
// cannot and shouldn't check for other errors like ordering due to expansion. If later, there's a failure due to the
// "import" abbreviation field, the parser would be at a relevant source position to err.
//
// See https://github.com/WebAssembly/spec/issues/1417
// See https://www.w3.org/TR/wasm-core-1/#abbreviations%E2%91%A8
func (p *abbreviationParser) setID(tokenBytes []byte) error {
	name, err := p.indexNamespace.setID(tokenBytes)
	if err != nil {
		return err
	}
	p.currentName = name
	return nil
}

// beginExportOrImport decides which tokenParser to use based on its field name: "export" or "import".
func (p *abbreviationParser) beginExportOrImport(tok tokenType, tokenBytes []byte, line, col uint32) (tokenParser, error) {
	if tok != tokenKeyword {
		return nil, unexpectedToken(tok, tokenBytes)
	}

	switch string(tokenBytes) {
	case "export":
		// See https://github.com/WebAssembly/spec/issues/1420
		if p.currentImport != nil {
			return nil, errors.New("export abbreviation after import")
		}
		p.pos = positionExport
		return p.parseExport, nil
	case "import":
		if len(p.module.FunctionSection) > 0 {
			return nil, errors.New("import after module-defined function")
		}
		if p.currentImport != nil {
			return nil, errors.New("redundant import")
		}
		p.pos = positionImport
		return p.parseImportModule, nil
	default:
		return p.end(callbackPositionUnhandledField, tok, tokenBytes, line, col)
	}
}

// parseMoreExportsOrImport looks for a '(', and if present returns beginExportOrImport to continue the type. Otherwise,
// it calls parseEnd.
func (p *abbreviationParser) parseMoreExportsOrImport(tok tokenType, tokenBytes []byte, line, col uint32) (tokenParser, error) {
	switch tok {
	case tokenID:
		return nil, fmt.Errorf("redundant ID: %s", tokenBytes)
	case tokenLParen:
		return p.beginExportOrImport, nil
	}
	return p.parseEnd(tok, tokenBytes, line, col)
}

// parseExport returns parseAbbreviationEnd after recording the export name, or errs if it couldn't be read.
//
// Ex. Export name is present `(func (export "PI"))`
//                             begins here --^   ^
//                               records PI --^  |
//           parseAbbreviationEnd resumes here --+
//
// Ex. Export name is absent `(export (func 0))`
//                        errs here --^
func (p *abbreviationParser) parseExport(tok tokenType, tokenBytes []byte, _, _ uint32) (tokenParser, error) {
	switch tok {
	case tokenString: // Ex. "" or "PI"
		name := string(tokenBytes[1 : len(tokenBytes)-1]) // strip quotes
		if _, ok := p.module.ExportSection[name]; ok {
			return nil, fmt.Errorf("duplicate name %q", name)
		}
		// Exports are added as they are parsed to allow index collisions to have correct line/col numbers
		export := &wasm.Export{Kind: p.exportKind, Name: name, Index: p.indexNamespace.count}
		if p.module.ExportSection == nil {
			p.module.ExportSection = map[string]*wasm.Export{name: export}
		} else {
			p.module.ExportSection[name] = export
		}
		return p.parseAbbreviationEnd, nil
	case tokenRParen:
		return nil, errors.New("missing name")
	default:
		return nil, unexpectedToken(tok, tokenBytes)
	}
}

// parseImportModule returns parseImportName after recording the import module name, or errs if it couldn't be read.
//
// Ex. Imported module name is present `(func (import "Math" "PI") (result f32))`
//                                      records Math --^     ^
//                            parseImportName resumes here --+
//
// Ex. Imported module name is absent `(func (import) (result f32))`
//                                      errs here --^
func (p *abbreviationParser) parseImportModule(tok tokenType, tokenBytes []byte, _, _ uint32) (tokenParser, error) {
	switch tok {
	case tokenString: // Ex. "" or "Math"
		module := string(tokenBytes[1 : len(tokenBytes)-1]) // unquote
		p.currentImport = &wasm.Import{Kind: p.importKind, Module: module}
		return p.parseImportName, nil
	case tokenLParen, tokenRParen:
		return nil, errors.New("missing module and name")
	default:
		return nil, unexpectedToken(tok, tokenBytes)
	}
}

// parseImportName returns parseAbbreviationEnd after recording the import name, or errs if it couldn't be read.
//
// Ex. Import name is present `(func (import "Math" "PI") (result f32))`
//                                    starts here --^^  ^
//                                      records PI --+  |
//                              parseEnd resumes here --+
//
// Ex. Imported function name is absent `(func (import "Math") (result f32))`
//                                               errs here --^
func (p *abbreviationParser) parseImportName(tok tokenType, tokenBytes []byte, _, _ uint32) (tokenParser, error) {
	switch tok {
	case tokenString: // Ex. "" or "PI"
		name := string(tokenBytes[1 : len(tokenBytes)-1]) // unquote
		p.currentImport.Name = name
		return p.parseAbbreviationEnd, nil
	case tokenLParen, tokenRParen:
		return nil, errors.New("missing name")
	default:
		return nil, unexpectedToken(tok, tokenBytes)
	}
}

// parseAbbreviationEnd continues even on "import" which cannot be followed by an "export". This is to allow a better
// error message on out-of order.
// See https://github.com/WebAssembly/spec/issues/1418
func (p *abbreviationParser) parseAbbreviationEnd(tok tokenType, tokenBytes []byte, _, _ uint32) (tokenParser, error) {
	switch tok {
	case tokenString: // Ex. "" or "PI"
		return nil, fmt.Errorf("redundant name %s", tokenBytes)
	case tokenRParen:
		p.pos = positionInitial
		return p.parseMoreExportsOrImport, nil
	default:
		return nil, unexpectedToken(tok, tokenBytes)
	}
}

func (p *abbreviationParser) parseEnd(tok tokenType, tokenBytes []byte, line, col uint32) (tokenParser, error) {
	pos := callbackPositionUnhandledToken
	if tok == tokenRParen {
		pos = callbackPositionEndField
	}
	return p.end(pos, tok, tokenBytes, line, col)
}

func (p *abbreviationParser) errorContext() string {
	switch p.pos {
	case positionImport:
		return ".import"
	case positionExport:
		return ".export"
	}
	return ""
}

// end invokes onAbbreviations to continue parsing
func (p *abbreviationParser) end(pos callbackPosition, tok tokenType, tokenBytes []byte, line, col uint32) (parser tokenParser, err error) {
	// Invoke the onAbbreviations hook with the current token
	parser, err = p.onAbbreviations(p.currentName, p.currentImport, pos, tok, tokenBytes, line, col)
	// reset
	p.currentName = ""
	p.currentImport = nil
	return
}

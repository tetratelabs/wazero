package internal

import (
	"errors"
	"fmt"

	"github.com/tetratelabs/wazero/internal/wasm"
)

// newIndexNamespace sectionElementCount parameter should be wasm.Module SectionElementCount unless testing.
func newIndexNamespace(sectionElementCount func(wasm.SectionID) uint32) *indexNamespace {
	return &indexNamespace{sectionElementCount: sectionElementCount, idToIdx: map[string]wasm.Index{}}
}

// indexNamespace contains the count in an index namespace and any association of symbolic IDs to numeric indices.
//
// The Web Assembly Text Format allows use of symbolic identifiers, ex "$main", instead of numeric indices for most
// sections, notably types, functions and parameters. For example, if a function is defined with the tokenID "$main",
// the start section can use that symbolic ID instead of its numeric offset in the code section. These IDs require two
// features, uniqueness and lookup, as implemented with a map. The key is stripped of the leading '$' to match other
// tools, as described in stripDollar
//
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#text-context
type indexNamespace struct {
	sectionElementCount func(wasm.SectionID) uint32

	unresolvedIndices []*unresolvedIndex

	// count is the count of items in this namespace
	count uint32

	// idToIdx resolves symbolic identifiers, such as "v_v" to a numeric index in the appropriate section, such
	// as '2'. Duplicate identifiers are not allowed by specification.
	//
	// Note: This is not encoded in the wasm.NameSection as there is no type name section in WebAssembly 1.0 (20191205)
	//
	// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#text-context
	idToIdx map[string]wasm.Index
}

// setID ensures the given tokenID is unique within this context and raises an error if not. The resulting mapping is
// stripped of the leading '$' to match other tools, as described in stripDollar.
func (i *indexNamespace) setID(idToken []byte) (string, error) {
	name, err := i.requireNoID(idToken)
	if err != nil {
		return name, err
	}
	i.idToIdx[name] = i.count
	return name, nil
}

// hasID checks to see if this tokenID is unique within this context and returns an error. The result string is
// stripped of the leading '$' to match other tools, as described in stripDollar.
func (i *indexNamespace) requireNoID(idToken []byte) (string, error) {
	name := string(stripDollar(idToken))
	if _, ok := i.idToIdx[name]; ok {
		return name, fmt.Errorf("duplicate ID %s", idToken)
	}
	return name, nil
}

// parseIndex is a tokenParser called in a field that can only contain a symbolic identifier or raw numeric index.
func (i *indexNamespace) parseIndex(section wasm.SectionID, bodyOffset uint32, tok tokenType, tokenBytes []byte, line, col uint32) (targetIdx wasm.Index, resolved bool, err error) {
	switch tok {
	case tokenUN: // Ex. 2
		if i, overflow := decodeUint32(tokenBytes); overflow {
			return 0, false, fmt.Errorf("index outside range of uint32: %s", tokenBytes)
		} else {
			targetIdx = i
		}

		if targetIdx < i.count {
			resolved = true
		} else {
			i.recordOutOfRange(section, bodyOffset, targetIdx, line, col)
		}
	case tokenID: // Ex. $main
		targetID := string(stripDollar(tokenBytes))
		if targetIdx, resolved = i.idToIdx[targetID]; !resolved {
			i.recordUnresolved(section, bodyOffset, targetID, line, col)
		}
		return
	case tokenRParen:
		err = errors.New("missing index")
	default:
		err = unexpectedToken(tok, tokenBytes)
	}
	return
}

// recordUnresolved records an ID, such as "main", is not yet resolvable.
//
// See unresolvedIndex for parameter descriptions
func (i *indexNamespace) recordUnresolved(section wasm.SectionID, bodyOffset uint32, targetID string, line, col uint32) {
	idx := i.sectionElementCount(section)
	i.unresolvedIndices = append(i.unresolvedIndices, &unresolvedIndex{section: section, idx: idx, bodyOffset: bodyOffset, targetID: targetID, line: line, col: col})
}

// recordUnresolved records numeric index is currently out of bounds.
//
// See unresolvedIndex for parameter descriptions
func (i *indexNamespace) recordOutOfRange(section wasm.SectionID, bodyOffset uint32, targetIdx wasm.Index, line, col uint32) {
	idx := i.sectionElementCount(section)
	i.unresolvedIndices = append(i.unresolvedIndices, &unresolvedIndex{section: section, idx: idx, bodyOffset: bodyOffset, targetIdx: targetIdx, line: line, col: col})
}

// unresolvedIndex is symbolic ID, such as "main", or its equivalent numeric value, such as "2".
//
// Note: section, idx, line and col used for lazy validation of index. These are attached to an error if later parsed to
// be invalid (ex an unknown function or out-of-bound index).
//
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#indices%E2%91%A4
type unresolvedIndex struct {
	// section is the primary index of what's targeting this index (in the api.Module)
	section wasm.SectionID

	// idx is slice position in the section
	idx wasm.Index

	// bodyOffset is only used when section is wasm.SectionIDCode and identifies the offset in wasm.Code Body.
	bodyOffset uint32

	// id is set when its corresponding token is tokenID to a symbolic identifier index. Ex. main
	//
	// Ex. This is t0 from (import "Math" "PI" (func (type $t0))), when (type $t0 (func ...)) does not yet exist.
	//
	// Note: After parsing, convert this to a numeric index via requireIndex
	targetID string

	// numeric is set when its corresponding token is tokenUN is a wasm.Index. Ex. 3
	//
	// Ex. If this is 32 from (import "Math" "PI" (func (type 32))), but there are only 10 types defined in the module.
	//
	// Note: To avoid conflating unset with the valid index zero, only read this value when ID is unset.
	// Note: After parsing, check this is in range via requireIndexInRange
	targetIdx wasm.Index

	// line is the line in the source where the index was defined.
	line uint32

	// col is the column on the line where the index was defined.
	col uint32
}

// resolve ensures the idx points to a valid numeric function index or returns a FormatError if it cannot be bound.
//
// Failure cases are when a symbolic identifier points nowhere or a numeric index is out of range.
// Ex. (start $t0) exists, but there's no import or module-defined function with that name.
//
//	or (start 32) exists, but there are only 10 functions.
func (i *indexNamespace) resolve(unresolved *unresolvedIndex) (wasm.Index, error) {
	if unresolved.targetID == "" { // already bound to a numeric index, but we have to verify it is in range
		if err := requireIndexInRange(unresolved.targetIdx, i.count); err != nil {
			return 0, unresolved.formatErr(err)
		}
		return unresolved.targetIdx, nil
	}
	numeric, err := i.requireIndex(unresolved.targetID)
	if err != nil {
		return 0, unresolved.formatErr(err)
	}
	return numeric, nil
}

func (i *indexNamespace) requireIndex(id string) (wasm.Index, error) {
	if numeric, ok := i.idToIdx[id]; ok {
		return numeric, nil
	}
	return 0, fmt.Errorf("unknown ID $%s", id) // re-attach '$' as that was in the text format!
}

func requireIndexInRange(index wasm.Index, count uint32) error {
	if index >= count {
		if count == 0 {
			return fmt.Errorf("index %d is not in range due to empty namespace", index)
		}
		return fmt.Errorf("index %d is out of range [0..%d]", index, count-1)
	}
	return nil
}

func (d *unresolvedIndex) formatErr(err error) error {
	// This check allows us to defer Sprintf until there's an error, and reuse the same logic for non-indexed types.
	var context string
	switch d.section {
	case wasm.SectionIDCode:
		context = fmt.Sprintf("module.code[%d].body[%d]", d.idx, d.bodyOffset)
	case wasm.SectionIDExport:
		context = fmt.Sprintf("module.exports[%d].func", d.idx)
	case wasm.SectionIDStart:
		context = "module.start"
	}
	return &FormatError{d.line, d.col, context, err}
}

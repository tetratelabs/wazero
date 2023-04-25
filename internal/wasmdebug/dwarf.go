package wasmdebug

import (
	"debug/dwarf"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
)

// DWARFLines is used to retrieve source code line information from the DWARF data.
type DWARFLines struct {
	// d is created by DWARF custom sections.
	d *dwarf.Data
	// linesPerEntry maps dwarf.Offset for dwarf.Entry to the list of lines contained by the entry.
	// The value is sorted in the increasing order by the address.
	linesPerEntry map[dwarf.Offset][]line
	// Cache for Language(). nil when Language() has never been called.
	lang *DwarfLang
	mux  sync.Mutex
}

type line struct {
	addr uint64
	pos  dwarf.LineReaderPos
}

// NewDWARFLines returns DWARFLines for the given *dwarf.Data.
func NewDWARFLines(d *dwarf.Data) *DWARFLines {
	if d == nil {
		return nil
	}
	return &DWARFLines{d: d, linesPerEntry: map[dwarf.Offset][]line{}}
}

type DwarfLang int64

// DWARF languages.
// https://github.com/llir/llvm/blob/466de3faf5e44c65af50a62f5564748a25649cef/ir/enum/enum.go#L300
const (
	// DWARF v2.
	DwarfLangUnknown   DwarfLang = 0x0000
	DwarfLangC89       DwarfLang = 0x0001 // DW_LANG_C89
	DwarfLangC         DwarfLang = 0x0002 // DW_LANG_C
	DwarfLangAda83     DwarfLang = 0x0003 // DW_LANG_Ada83
	DwarfLangCPlusPlus DwarfLang = 0x0004 // DW_LANG_C_plus_plus
	DwarfLangCobol74   DwarfLang = 0x0005 // DW_LANG_Cobol74
	DwarfLangCobol85   DwarfLang = 0x0006 // DW_LANG_Cobol85
	DwarfLangFortran77 DwarfLang = 0x0007 // DW_LANG_Fortran77
	DwarfLangFortran90 DwarfLang = 0x0008 // DW_LANG_Fortran90
	DwarfLangPascal83  DwarfLang = 0x0009 // DW_LANG_Pascal83
	DwarfLangModula2   DwarfLang = 0x000A // DW_LANG_Modula2
	// DWARF v3.
	DwarfLangJava         DwarfLang = 0x000B // DW_LANG_Java
	DwarfLangC99          DwarfLang = 0x000C // DW_LANG_C99
	DwarfLangAda95        DwarfLang = 0x000D // DW_LANG_Ada95
	DwarfLangFortran95    DwarfLang = 0x000E // DW_LANG_Fortran95
	DwarfLangPLI          DwarfLang = 0x000F // DW_LANG_PLI
	DwarfLangObjC         DwarfLang = 0x0010 // DW_LANG_ObjC
	DwarfLangObjCPlusPlus DwarfLang = 0x0011 // DW_LANG_ObjC_plus_plus
	DwarfLangUPC          DwarfLang = 0x0012 // DW_LANG_UPC
	DwarfLangD            DwarfLang = 0x0013 // DW_LANG_D
	// DWARF v4.
	DwarfLangPython DwarfLang = 0x0014 // DW_LANG_Python
	// DWARF v5.
	DwarfLangOpenCL       DwarfLang = 0x0015 // DW_LANG_OpenCL
	DwarfLangGo           DwarfLang = 0x0016 // DW_LANG_Go
	DwarfLangModula3      DwarfLang = 0x0017 // DW_LANG_Modula3
	DwarfLangHaskell      DwarfLang = 0x0018 // DW_LANG_Haskell
	DwarfLangCPlusPlus03  DwarfLang = 0x0019 // DW_LANG_C_plus_plus_03
	DwarfLangCPlusPlus11  DwarfLang = 0x001A // DW_LANG_C_plus_plus_11
	DwarfLangOCaml        DwarfLang = 0x001B // DW_LANG_OCaml
	DwarfLangRust         DwarfLang = 0x001C // DW_LANG_Rust
	DwarfLangC11          DwarfLang = 0x001D // DW_LANG_C11
	DwarfLangSwift        DwarfLang = 0x001E // DW_LANG_Swift
	DwarfLangJulia        DwarfLang = 0x001F // DW_LANG_Julia
	DwarfLangDylan        DwarfLang = 0x0020 // DW_LANG_Dylan
	DwarfLangCPlusPlus14  DwarfLang = 0x0021 // DW_LANG_C_plus_plus_14
	DwarfLangFortran03    DwarfLang = 0x0022 // DW_LANG_Fortran03
	DwarfLangFortran08    DwarfLang = 0x0023 // DW_LANG_Fortran08
	DwarfLangRenderScript DwarfLang = 0x0024 // DW_LANG_RenderScript
	DwarfLangBLISS        DwarfLang = 0x0025 // DW_LANG_BLISS
	// Vendor extensions.
	DwarfLangMipsAssembler      DwarfLang = 0x8001 // DW_LANG_Mips_Assembler
	DwarfLangGoogleRenderScript DwarfLang = 0x8E57 // DW_LANG_GOOGLE_RenderScript
	DwarfLangBorlandDelphi      DwarfLang = 0xB000 // DW_LANG_BORLAND_Delphi
)

func (d *DWARFLines) Language() DwarfLang {
	if d == nil {
		return DwarfLangUnknown
	}
	if d.lang != nil {
		return *d.lang
	}

	d.mux.Lock()
	defer d.mux.Unlock()

	d.lang = new(DwarfLang)
	*d.lang = DwarfLangUnknown

	r := d.d.Reader()
	for {
		ent, err := r.Next()
		if err != nil || ent == nil {
			break
		}

		if ent.Tag != dwarf.TagCompileUnit {
			continue
		}

		langAttr := ent.Val(dwarf.AttrLanguage)
		if langAttr == nil {
			continue
		}

		// Assumes same language for all compilation units.
		*d.lang = DwarfLang(langAttr.(int64))
		break
	}
	return *d.lang
}

// LinkageNamesDo parses the DWARF data and execute f for all subprograms that
// have a linkage and a name attributes.
func (d *DWARFLines) LinkageNamesDo(f func(linkageName, name string)) {
	if d == nil {
		return
	}

	d.mux.Lock()
	defer d.mux.Unlock()

	r := d.d.Reader()

	for {
		ent, err := r.Next()
		if err != nil || ent == nil {
			break
		}

		if ent.Tag != dwarf.TagSubprogram {
			continue
		}

		linkageNameAttr := ent.Val(dwarf.AttrLinkageName)
		if linkageNameAttr == nil {
			continue
		}
		nameAttr := ent.Val(dwarf.AttrName)
		if nameAttr == nil {
			continue
		}
		f(linkageNameAttr.(string), nameAttr.(string))
	}
}

// Line returns the line information for the given instructionOffset which is an offset in
// the code section of the original Wasm binary. Returns empty string if the info is not found.
func (d *DWARFLines) Line(instructionOffset uint64) (ret []string) {
	if d == nil {
		return
	}

	// DWARFLines is created per Wasm binary, so there's a possibility that multiple instances
	// created from a same binary face runtime error at the same time, and that results in
	// concurrent access to this function.
	d.mux.Lock()
	defer d.mux.Unlock()

	r := d.d.Reader()

	var inlinedRoutines []*dwarf.Entry
	var cu *dwarf.Entry
	var inlinedDone bool
entry:
	for {
		ent, err := r.Next()
		if err != nil || ent == nil {
			break
		}

		// If we already found the compilation unit and relevant inlined routines, we can stop searching entries.
		if cu != nil && inlinedDone {
			break
		}

		switch ent.Tag {
		case dwarf.TagCompileUnit, dwarf.TagInlinedSubroutine:
		default:
			// Only CompileUnit and InlinedSubroutines are relevant.
			continue
		}

		// Check if the entry spans the range which contains the target instruction.
		ranges, err := d.d.Ranges(ent)
		if err != nil {
			continue
		}
		for _, pcs := range ranges {
			if pcs[0] <= instructionOffset && instructionOffset < pcs[1] {
				switch ent.Tag {
				case dwarf.TagCompileUnit:
					cu = ent
				case dwarf.TagInlinedSubroutine:
					inlinedRoutines = append(inlinedRoutines, ent)
					// Search inlined subroutines until all the children.
					inlinedDone = !ent.Children
					// Not that "children" in the DWARF spec is defined as the next entry to this entry.
					// See "2.3 Relationship of Debugging Information Entries" in https://dwarfstd.org/doc/DWARF4.pdf
				}
				continue entry
			}
		}
	}

	// If the relevant compilation unit is not found, nothing we can do with this DWARF info.
	if cu == nil {
		return
	}

	lineReader, err := d.d.LineReader(cu)
	if err != nil || lineReader == nil {
		return
	}
	var lines []line
	var ok bool
	var le dwarf.LineEntry
	// Get the lines inside the entry.
	if lines, ok = d.linesPerEntry[cu.Offset]; !ok {
		// If not found, we create the list of lines by reading all the LineEntries in the Entry.
		//
		// Note that the dwarf.LineEntry.SeekPC API shouldn't be used because the Go's dwarf package assumes that
		// all the line entries in an Entry are sorted in increasing order which *might not* be true
		// for some languages. Such order requirement is not a part of DWARF specification,
		// and in fact Zig language tends to emit interleaved line information.
		//
		// Thus, here we read all line entries here, and sort them in the increasing order wrt addresses.
		for {
			pos := lineReader.Tell()
			err = lineReader.Next(&le)
			if errors.Is(err, io.EOF) {
				break
			} else if err != nil {
				return
			}
			lines = append(lines, line{addr: le.Address, pos: pos})
		}
		sort.Slice(lines, func(i, j int) bool { return lines[i].addr < lines[j].addr })
		d.linesPerEntry[cu.Offset] = lines // Caches for the future inquiries for the same Entry.
	}

	// Now we have the lines for this entry. We can find the corresponding source line for instructionOffset
	// via binary search on the list.
	n := len(lines)
	index := sort.Search(n, func(i int) bool { return lines[i].addr >= instructionOffset })

	if index == n { // This case the address is not found. See the doc sort.Search.
		return
	}

	ln := lines[index]
	if ln.addr != instructionOffset {
		// If the address doesn't match exactly, the previous entry is the one that contains the instruction.
		// That can happen anytime as the DWARF spec allows it, and other tools can handle it in this way conventionally
		// https://github.com/gimli-rs/addr2line/blob/3a2dbaf84551a06a429f26e9c96071bb409b371f/src/lib.rs#L236-L242
		// https://github.com/kateinoigakukun/wasminspect/blob/f29f052f1b03104da9f702508ac0c1bbc3530ae4/crates/debugger/src/dwarf/mod.rs#L453-L459
		if index-1 < 0 {
			return
		}
		ln = lines[index-1]
	}

	// Advance the line reader for the found position.
	lineReader.Seek(ln.pos)
	err = lineReader.Next(&le)

	if err != nil {
		// If we reach this block, that means there's a bug in the []line creation logic above.
		panic("BUG: stored dwarf.LineReaderPos is invalid")
	}

	// In the inlined case, the line info is the innermost inlined function call.
	inlined := len(inlinedRoutines) != 0
	prefix := fmt.Sprintf("%#x: ", instructionOffset)
	ret = append(ret, formatLine(prefix, le.File.Name, int64(le.Line), int64(le.Column), inlined))

	if inlined {
		prefix = strings.Repeat(" ", len(prefix))
		files := lineReader.Files()
		// inlinedRoutines contain the inlined call information in the reverse order (children is higher than parent),
		// so we traverse the reverse order and emit the inlined calls.
		for i := len(inlinedRoutines) - 1; i >= 0; i-- {
			inlined := inlinedRoutines[i]
			fileIndex, ok := inlined.Val(dwarf.AttrCallFile).(int64)
			if !ok {
				return
			} else if fileIndex >= int64(len(files)) {
				// This in theory shouldn't happen according to the spec, but guard against ill-formed DWARF info.
				return
			}
			fileName := files[fileIndex]
			line, _ := inlined.Val(dwarf.AttrCallLine).(int64)
			col, _ := inlined.Val(dwarf.AttrCallColumn).(int64)
			ret = append(ret, formatLine(prefix, fileName.Name, line, col,
				// Last one is the origin of the inlined function calls.
				i != 0))
		}
	}
	return
}

func formatLine(prefix, fileName string, line, col int64, inlined bool) string {
	builder := strings.Builder{}
	builder.WriteString(prefix)
	builder.WriteString(fileName)

	if line != 0 {
		builder.WriteString(fmt.Sprintf(":%d", line))
		if col != 0 {
			builder.WriteString(fmt.Sprintf(":%d", col))
		}
	}

	if inlined {
		builder.WriteString(" (inlined)")
	}
	return builder.String()
}

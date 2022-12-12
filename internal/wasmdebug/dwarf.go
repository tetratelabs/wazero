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
	mux           sync.Mutex
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

	// Advance the line reader for the found position.
	lineReader.Seek(lines[index].pos)
	err = lineReader.Next(&le)

	if err != nil {
		// If we reach this block, that means there's a bug in the []line creation logic above.
		panic("BUG: stored dwarf.LineReaderPos is invalid")
	}

	if len(inlinedRoutines) == 0 {
		// Do early return for non-inlined case.
		ret = []string{fmt.Sprintf("%#x: %s:%d:%d", le.Address, le.File.Name, le.Line, le.Column)}
		return
	}

	// In the inlined case, the line info is the innermost inlined function call.
	addr := fmt.Sprintf("%#x: ", le.Address)
	ret = append(ret, fmt.Sprintf("%s%s:%d:%d (inlined)", addr, le.File.Name, le.Line, le.Column))

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
		fileName, line, col := files[fileIndex], inlined.Val(dwarf.AttrCallLine), inlined.Val(dwarf.AttrCallColumn)
		if i == 0 {
			// Last one is the origin of the inlined function calls.
			ret = append(ret, fmt.Sprintf("%s%s:%d:%d", strings.Repeat(" ", len(addr)), fileName.Name, line, col))
		} else {
			ret = append(ret, fmt.Sprintf("%s%s:%d:%d (inlined)", strings.Repeat(" ", len(addr)), fileName.Name, line, col))
		}
	}
	return
}

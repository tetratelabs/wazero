package wasmdebug

import (
	"debug/dwarf"
	"errors"
	"fmt"
	"io"
	"sort"
)

// DWARFLines is used to retrieve source code line information from the DWARF data.
type DWARFLines struct {
	// d is created by DWARF custom sections.
	d *dwarf.Data
	// linesPerEntry maps dwarf.Offset for dwarf.Entry to the list of lines contained by the entry.
	// The value is sorted in the increasing order by the address.
	linesPerEntry map[dwarf.Offset][]line
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
func (d *DWARFLines) Line(instructionOffset uint64) string {
	if d == nil {
		return ""
	}

	r := d.d.Reader()

	// Get the dwarf.Entry containing the instruction.
	entry, err := r.SeekPC(instructionOffset)
	if err != nil {
		return ""
	}

	lineReader, err := d.d.LineReader(entry)
	if err != nil {
		return ""
	}

	var lines []line
	var ok bool
	var le dwarf.LineEntry
	// Get the lines inside the entry.
	if lines, ok = d.linesPerEntry[entry.Offset]; !ok {
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
				return ""
			}
			lines = append(lines, line{addr: le.Address, pos: pos})
		}
		sort.Slice(lines, func(i, j int) bool { return lines[i].addr < lines[j].addr })
		d.linesPerEntry[entry.Offset] = lines // Caches for the future inquiries for the same Entry.
	}

	// Now we have the lines for this entry. We can find the corresponding source line for instructionOffset
	// via binary search on the list.
	n := len(lines)
	index := sort.Search(n, func(i int) bool { return lines[i].addr >= instructionOffset })

	if index == n { // This case the address is not found. See the doc sort.Search.
		return ""
	}

	// Advance the line reader for the found position.
	lineReader.Seek(lines[index].pos)
	err = lineReader.Next(&le)
	if err != nil {
		// If we reach this block, that means there's a bug in the []line creation logic above.
		panic("BUG: stored dwarf.LineReaderPos is invalid")
	}
	return fmt.Sprintf("%#x: %s:%d:%d", le.Address, le.File.Name, le.Line, le.Column)
}

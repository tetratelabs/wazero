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
func (d *DWARFLines) Line(instructionOffset uint64) string {
	if d == nil {
		return ""
	}

	// DWARFLines is created per Wasm binary, so there's a possibility that multiple instances
	// created from a same binary face runtime error at the same time, and that results in
	// concurrent access to this function.
	d.mux.Lock()
	defer d.mux.Unlock()

	r := d.d.Reader()

	var ents []*dwarf.Entry
	for {
		ent, err := r.Next()
		if err != nil || ent == nil {
			break
		}

		switch ent.Tag {
		case dwarf.TagInlinedSubroutine, dwarf.TagCompileUnit:
		default:
			continue
		}

		ranges, err := d.d.Ranges(ent)
		if err != nil {
			continue
		}
		for _, pcs := range ranges {
			if pcs[0] <= instructionOffset && instructionOffset < pcs[1] {
				ents = append(ents, ent)
			}
		}
	}

	var strs []string
	var files []*dwarf.LineFile
	for _, entry := range ents {
		lineReader, err := d.d.LineReader(entry)
		if err != nil {
			return ""
		} else if lineReader != nil {
			files = lineReader.Files()

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

			strs = append(strs, fmt.Sprintf("%#x: %s:%d:%d", le.Address, le.File.Name, le.Line, le.Column))
		} else {
			strs = append(strs,
				fmt.Sprintf("\t%s:%d:%d",
					files[entry.Val(dwarf.AttrCallFile).(int64)].Name,
					entry.Val(dwarf.AttrCallLine),
					entry.Val(dwarf.AttrCallColumn),
				),
			)
		}
	}

	return strings.Join(strs, "\n")
}

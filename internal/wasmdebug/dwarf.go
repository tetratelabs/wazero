package wasmdebug

import (
	"debug/dwarf"
	"fmt"
)

// GetSourceInfo returns the source information for the given instructionOffset which is an offset in
// the code section of the original Wasm binary. Returns empty string if the info is not found.
func GetSourceInfo(d *dwarf.Data, instructionOffset uint64) string {
	if d == nil {
		return ""
	}

	r := d.Reader()
	entry, err := r.SeekPC(instructionOffset)
	if err != nil {
		return ""
	}

	lineReader, err := d.LineReader(entry)
	if err != nil {
		return ""
	}

	var le dwarf.LineEntry
	err = lineReader.SeekPC(instructionOffset, &le)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%#x: %s:%d:%d", le.Address, le.File.Name, le.Line, le.Column)
}

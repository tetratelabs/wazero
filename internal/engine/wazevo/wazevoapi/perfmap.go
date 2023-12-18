package wazevoapi

import (
	"fmt"
	"os"
	"strconv"
)

var PerfMap *Perfmap

func init() {
	if PerfMapEnabled {
		pid := os.Getpid()
		filename := "/tmp/perf-" + strconv.Itoa(pid) + ".map"

		fh, err := os.OpenFile(filename, os.O_APPEND|os.O_RDWR|os.O_CREATE, 0644)
		if err != nil {
			panic(err)
		}

		PerfMap = &Perfmap{fh: fh}
	}
}

// Perfmap holds perfmap entries to be flushed into a perfmap file.
type Perfmap struct {
	entries []entry
	fh      *os.File
}

type entry struct {
	addr int64
	size uint64
	name string
}

// AddEntry adds a new entry to the perfmap. Each entry contains the address,
// the size and the name of the function.
func (f *Perfmap) AddEntry(addr int64, size uint64, name string) {
	e := entry{addr, size, name}
	if f.entries == nil {
		f.entries = []entry{e}
		return
	}
	f.entries = append(f.entries, e)
}

func (f *Perfmap) Clear() {
	f.entries = f.entries[:0]
}

func (f *Perfmap) Flush(offset uintptr) {
	defer func() {
		_ = f.fh.Sync()
	}()

	for _, e := range f.entries {
		f.fh.WriteString(fmt.Sprintf("%x %s %s\n",
			uintptr(e.addr)+offset,
			strconv.FormatUint(e.size, 16),
			e.name,
		))
	}
}

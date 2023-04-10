package wasmdebug

import (
	"fmt"
	"os"
	"strconv"
)

// Perfmap holds perfmap entries to be flushed into a perfmap file.
type Perfmap struct {
	entries []entry
	fh      *os.File
}

type entry struct {
	addr *uint64
	size uint64
	name string
}

func NewPerfmap() *Perfmap {
	return &Perfmap{}
}

// AddEntry adds a new entry to the perfmap. Each entry contains the address,
// the size and the name of the function.
func (f *Perfmap) AddEntry(addr *uint64, size uint64, name string) {
	e := entry{addr, size, name}
	if f.entries == nil {
		f.entries = []entry{e}
		return
	}
	f.entries = append(f.entries, e)
}

func (f *Perfmap) Flush() error {
	defer func() {
		_ = f.fh.Sync()
		_ = f.fh.Close()
	}()

	var err error
	f.fh, err = f.file()
	if err != nil {
		return err
	}

	for _, e := range f.entries {
		f.fh.WriteString(fmt.Sprintf("%x %s %s\n",
			e.addr,
			strconv.FormatUint(e.size, 16),
			e.name,
		))
	}
	return nil
}

func (f *Perfmap) file() (*os.File, error) {
	pid := os.Getpid()
	filename := "/tmp/perf-" + strconv.Itoa(pid) + ".map"

	fh, err := os.OpenFile(filename, os.O_APPEND|os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return nil, err
	}
	return fh, nil
}

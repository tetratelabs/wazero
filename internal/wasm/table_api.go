package wasm

import (
	"fmt"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/internalapi"
)

// exportedTable wraps TableInstance to implement api.Table.
type exportedTable struct {
	internalapi.WazeroOnlyType
	t *TableInstance
}

// Type implements api.Table.
func (t exportedTable) Type() api.ValueType {
	return t.t.Type.Kind()
}

// Size implements api.Table.
func (t exportedTable) Size() uint32 {
	return uint32(len(t.t.References))
}

// Grow implements api.Table.
func (t exportedTable) Grow(delta uint32, init uint64) (previousSize uint32, ok bool) {
	previousSize = t.t.Grow(delta, Reference(init))
	// TableInstance.Grow's own sentinel for "rejected" is the same -1-as-uint32 value the table.grow
	// instruction itself returns, per its doc comment. delta==0 always returns the (possibly identical
	// looking) current length rather than failing, so only treat the sentinel as failure when a real
	// grow was requested.
	if delta != 0 && previousSize == 0xffffffff {
		return 0, false
	}
	return previousSize, true
}

// Get implements api.Table.
func (t exportedTable) Get(i uint32) (uint64, error) {
	ref, err := t.t.Get(i)
	return uint64(ref), err
}

// Set implements api.Table.
func (t exportedTable) Set(i uint32, v uint64) error {
	return t.t.Set(i, Reference(v))
}

// String implements fmt.Stringer.
func (t exportedTable) String() string {
	return fmt.Sprintf("Table(%s)", RefTypeName(t.t.Type))
}

var _ api.Table = exportedTable{}

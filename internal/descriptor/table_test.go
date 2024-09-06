package descriptor

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func Test_sizeOfTable(t *testing.T) {
	tests := []struct {
		name         string
		operation    func(*Table[int32, string])
		expectedSize int
	}{
		{
			name:         "empty table",
			operation:    func(table *Table[int32, string]) {},
			expectedSize: 0,
		},
		{
			name: "1 insert",
			operation: func(table *Table[int32, string]) {
				table.Insert("a")
			},
			expectedSize: 1,
		},
		{
			name: "32 inserts",
			operation: func(table *Table[int32, string]) {
				for i := 0; i < 32; i++ {
					table.Insert("a")
				}
			},
			expectedSize: 1,
		},
		{
			name: "257 inserts",
			operation: func(table *Table[int32, string]) {
				for i := 0; i < 257; i++ {
					table.Insert("a")
				}
			},
			expectedSize: 5,
		},
		{
			name: "1 insert at 63",
			operation: func(table *Table[int32, string]) {
				table.InsertAt("a", 63)
			},
			expectedSize: 1,
		},
		{
			name: "1 insert at 64",
			operation: func(table *Table[int32, string]) {
				table.InsertAt("a", 64)
			},
			expectedSize: 2,
		},
		{
			name: "1 insert at 257",
			operation: func(table *Table[int32, string]) {
				table.InsertAt("a", 257)
			},
			expectedSize: 5,
		},
		{
			name: "insert at until 320",
			operation: func(table *Table[int32, string]) {
				for i := int32(0); i < 320; i++ {
					table.InsertAt("a", i)
				}
			},
			expectedSize: 5,
		},
	}
	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			table := new(Table[int32, string])
			tc.operation(table)
			require.Equal(t, tc.expectedSize, len(table.masks))
			require.Equal(t, tc.expectedSize*64, len(table.items))
		})
	}
}

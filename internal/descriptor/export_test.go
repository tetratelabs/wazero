package descriptor

// Masks returns the masks of the table for testing purposes.
func (t *Table[int32, string]) Masks() []uint64 {
	return t.masks
}

// Items returns the items of the table for testing purposes.
func (t *Table[int32, string]) Items() []string {
	return t.items
}

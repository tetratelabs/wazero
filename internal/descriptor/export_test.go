package descriptor

// Masks returns the masks of the table for testing purposes.
func Masks[Key ~int32, Item any](t *Table[Key, Item]) []uint64 {
	return t.masks
}

// Items returns the items of the table for testing purposes.
func Items[Key ~int32, Item any](t *Table[Key, Item]) []Item {
	return t.items
}

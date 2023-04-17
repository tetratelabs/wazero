package platform

func fill(b ...int64) (bits [16]int64) {
	copy(bits[:], b)
	return
}

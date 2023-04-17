package platform

func fill(b ...int32) (bits [32]int32) {
	copy(bits[:], b)
	return
}

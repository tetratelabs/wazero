package platform

func fill(b ...int32) (bits [32]int32) {
	for i, v := range b {
		bits[i] = v
	}
	return
}

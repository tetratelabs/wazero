package platform

func fill(value int64) (bits [16]int64) {
	for i := range bits {
		bits[i] = value
	}
	return
}

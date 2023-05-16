//go:build !linux

package platform

func remapCodeSegmentAMD64(code []byte, size int) ([]byte, error) {
	b, err := mmapCodeSegmentAMD64(size)
	if err != nil {
		return nil, err
	}
	copy(b, code)
	mustMunmapCodeSegment(code)
	return b, nil
}

// mustMunmapCodeSegment panics instead of returning an error to the
// application.
//
// # Why panic?
//
// It is less disruptive to the application to leak the previous block if it
// could be unmapped than to leak the new block and return an error.
// Realistically, either scenarios are pretty hard to debug, so we panic. There
// is less as the dominant runtime.GOOS (linux) doesn't use this anyway.
func mustMunmapCodeSegment(code []byte) {
	if err := munmapCodeSegment(code); err != nil {
		panic(err)
	}
}

func remapCodeSegmentARM64(code []byte, size int) ([]byte, error) {
	b, err := mmapCodeSegmentARM64(size)
	if err != nil {
		return nil, err
	}
	copy(b, code)
	mustMunmapCodeSegment(code)
	return b, nil
}

//go:build !linux

package platform

func remapCodeSegmentAMD64(code []byte, size int) ([]byte, error) {
	b, err := mmapCodeSegmentAMD64(size)
	if err != nil {
		return nil, err
	}
	copy(b, code)
	err = munmapCodeSegment(code)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func remapCodeSegmentARM64(code []byte, size int) ([]byte, error) {
	b, err := mmapCodeSegmentARM64(size)
	if err != nil {
		return nil, err
	}
	copy(b, code)
	err = munmapCodeSegment(code)
	if err != nil {
		return nil, err
	}
	return b, nil
}

//go:build tinygo

package platform

func remapCodeSegmentAMD64(code []byte, size int) ([]byte, error) {
	return nil, errNotYetSupported
}

func remapCodeSegmentARM64(code []byte, size int) ([]byte, error) {
	return nil, errNotYetSupported
}

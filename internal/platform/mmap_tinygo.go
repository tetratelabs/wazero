//go:build tinygo

package platform

import "errors"

var errNotYetSupported = errors.New("not yet supported")

func munmapCodeSegment(code []byte) error {
	return errNotYetSupported
}

func mmapCodeSegmentAMD64(size int) ([]byte, error) {
	return nil, errNotYetSupported
}

func mmapCodeSegmentARM64(size int) ([]byte, error) {
	return nil, errNotYetSupported
}

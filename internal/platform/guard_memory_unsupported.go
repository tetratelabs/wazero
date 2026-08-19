//go:build !((darwin || linux || freebsd) && (amd64 || arm64))

package platform

import "errors"

// GuardPageMemorySupported is false on this platform; guard-page-backed
// linear memory silently falls back to bounds-checked code generation.
const GuardPageMemorySupported = false

var errGuardUnsupported = errors.New("guard-page memory is not supported on this platform")

func ReserveGuardRegion(uintptr) ([]byte, error) { return nil, errGuardUnsupported }
func CommitGuardRegion([]byte, uintptr) error    { return errGuardUnsupported }
func ReleaseGuardRegion([]byte) error            { return errGuardUnsupported }

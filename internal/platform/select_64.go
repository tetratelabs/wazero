//go:build !darwin && !netbsd && !openbsd && (amd64 || arm64 || ppc64le || mips64 || mips64le || s390x || riscv64 || loong64)

package platform

// darwin, netbsd and openbsd uses uint32 on both amd64 and 386

const (
	// NFDBITS is the amount of bits per mask
	NFDBITS = 8 * 8
)

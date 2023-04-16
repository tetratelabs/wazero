package platform

// Set adds fd to the set fds.
func (fds *FdSet) Set(fd int) {
	fds.Bits[fd/nfdbits] |= (1 << (uintptr(fd) % nfdbits))
}

// Clear removes fd from the set fds.
func (fds *FdSet) Clear(fd int) {
	fds.Bits[fd/nfdbits] &^= (1 << (uintptr(fd) % nfdbits))
}

// IsSet returns whether fd is in the set fds.
func (fds *FdSet) IsSet(fd int) bool {
	return fds.Bits[fd/nfdbits]&(1<<(uintptr(fd)%nfdbits)) != 0
}

// Zero clears the set fds.
func (fds *FdSet) Zero() {
	for i := range fds.Bits {
		fds.Bits[i] = 0
	}
}

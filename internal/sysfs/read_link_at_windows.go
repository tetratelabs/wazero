package sysfs

import (
	"os"
	"syscall"
	"unsafe"
)

// ReadLinkAt gets the target of a symlink.  This function does not support `.`
// and `..` in `path`.
func ReadLinkAt(dir *os.File, path string) (string, error) {
	objectName, err := NewNTUnicodeString(path)
	if err != nil {
		return "", err
	}

	objectAttributes := OBJECT_ATTRIBUTES{
		Length:             uint32(unsafe.Sizeof(OBJECT_ATTRIBUTES{})),
		RootDirectory:      syscall.Handle(dir.Fd()),
		ObjectName:         objectName,
		Attributes:         OBJ_CASE_INSENSITIVE,
		SecurityDescriptor: 0,
		SecurityQoS:        0,
	}

	var (
		fd             syscall.Handle
		allocationSize int64
		ioStatusBlock  IO_STATUS_BLOCK
	)

	e := NtCreateFile(
		&fd,
		FILE_GENERIC_READ,
		&objectAttributes,
		&ioStatusBlock,
		&allocationSize,
		syscall.FILE_ATTRIBUTE_NORMAL,
		uint32(syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE),
		FILE_OPEN,
		FILE_OPEN_FOR_BACKUP_INTENT|FILE_OPEN_REPARSE_POINT,
		0,
		0,
	)
	if e != 0 {
		return "", e
	}

	reparseBuffer := make([]byte, MAXIMUM_REPARSE_DATA_BUFFER_SIZE)
	var bytesReturned uint32

	err = syscall.DeviceIoControl(
		fd,
		FSCTL_GET_REPARSE_POINT,
		nil,
		0,
		&reparseBuffer[0],
		uint32(len(reparseBuffer)),
		&bytesReturned,
		nil,
	)
	if err != nil {
		return "", err
	}

	rdb := (*REPARSE_DATA_BUFFER)(unsafe.Pointer(&reparseBuffer[0]))
	rb := (*SymbolicLinkReparseBuffer)(unsafe.Pointer(&rdb.DUMMYUNIONNAME))
	s := rb.Path()

	return s, nil
}

package sysfs

import (
	"syscall"
	"unsafe"
)

// NTUnicodeString is a UTF-16 string for NT native APIs, corresponding to UNICODE_STRING.
type NTUnicodeString struct {
	Length        uint16
	MaximumLength uint16
	Buffer        *uint16
}

type OBJECT_ATTRIBUTES struct {
	Length        uint32
	RootDirectory syscall.Handle
	ObjectName    *NTUnicodeString
	Attributes    uint32

	// Always nil.
	SecurityDescriptor uintptr

	// Always nil.
	SecurityQoS uintptr
}

type IO_STATUS_BLOCK struct {
	Status      NTStatus
	Information uintptr
}

// https://learn.microsoft.com/en-us/windows/win32/secauthz/access-mask
type ACCESS_MASK uint32

// Constants for type ACCESS_MASK
const (
	READ_CONTROL            = 0x00020000
	SYNCHRONIZE             = 0x00100000
	STANDARD_RIGHTS_READ    = READ_CONTROL
	STANDARD_RIGHTS_WRITE   = READ_CONTROL
	STANDARD_RIGHTS_EXECUTE = READ_CONTROL
)

// File access rights constants.
// https://learn.microsoft.com/en-us/windows/win32/fileio/file-access-rights-constants
const (
	FILE_READ_DATA        = 0x00000001
	FILE_READ_ATTRIBUTES  = 0x00000080
	FILE_READ_EA          = 0x00000008
	FILE_WRITE_DATA       = 0x00000002
	FILE_WRITE_ATTRIBUTES = 0x00000100
	FILE_WRITE_EA         = 0x00000010
	FILE_APPEND_DATA      = 0x00000004
	FILE_EXECUTE          = 0x00000020
)

const (
	FILE_GENERIC_READ    = STANDARD_RIGHTS_READ | FILE_READ_DATA | FILE_READ_ATTRIBUTES | FILE_READ_EA | SYNCHRONIZE
	FILE_GENERIC_WRITE   = STANDARD_RIGHTS_WRITE | FILE_WRITE_DATA | FILE_WRITE_ATTRIBUTES | FILE_WRITE_EA | FILE_APPEND_DATA | SYNCHRONIZE
	FILE_GENERIC_EXECUTE = STANDARD_RIGHTS_EXECUTE | FILE_READ_ATTRIBUTES | FILE_EXECUTE | SYNCHRONIZE
)

// NtCreateFile CreateDisposition
const (
	FILE_SUPERSEDE = 0x00000000
	FILE_OPEN      = 0x00000001
	FILE_CREATE    = 0x00000002
	FILE_OPEN_IF   = 0x00000003
	FILE_OVERWRITE = 0x00000004
)

const (
	// NtCreateFile CreateOptions
	FILE_OPEN_FOR_BACKUP_INTENT = 0x00004000

	// https://learn.microsoft.com/en-us/windows/win32/api/ntdef/nf-ntdef-initializeobjectattributes
	OBJ_CASE_INSENSITIVE = 0x00000040
)

func NtCreateFile(
	handle *syscall.Handle,
	access uint32,
	oa *OBJECT_ATTRIBUTES,
	iosb *IO_STATUS_BLOCK,
	allocationSize *int64,
	attributes uint32,
	share uint32,
	disposition uint32,
	options uint32,
	eabuffer uintptr,
	ealength uint32,
) syscall.Errno {
	r0, _, _ := syscall.SyscallN(
		procNtCreateFile.Addr(),
		uintptr(unsafe.Pointer(handle)),
		uintptr(access),
		uintptr(unsafe.Pointer(oa)),
		uintptr(unsafe.Pointer(iosb)),
		uintptr(unsafe.Pointer(allocationSize)),
		uintptr(attributes),
		uintptr(share),
		uintptr(disposition),
		uintptr(options),
		uintptr(eabuffer),
		uintptr(ealength),
	)

	return NTStatus(r0).Errno()
}

// NewNTUnicodeString returns a new NTUnicodeString structure for use with native
// NT APIs that work over the NTUnicodeString type. Note that most Windows APIs
// do not use NTUnicodeString, and instead UTF16PtrFromString should be used for
// the more common *uint16 string type.
func NewNTUnicodeString(s string) (*NTUnicodeString, error) {
	var u NTUnicodeString

	s16, err := syscall.UTF16PtrFromString(s)
	if err != nil {
		return nil, err
	}

	RtlInitUnicodeString(&u, s16)

	return &u, nil
}

func RtlInitUnicodeString(destinationString *NTUnicodeString, sourceString *uint16) {
	syscall.SyscallN(
		procRtlInitUnicodeString.Addr(),
		uintptr(unsafe.Pointer(destinationString)),
		uintptr(unsafe.Pointer(sourceString)),
	)
}

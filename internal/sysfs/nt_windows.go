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
	// https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-smb2/e8fb45c1-a03d-44ca-b7ae-47385cfd7997
	FILE_OPEN_FOR_BACKUP_INTENT = 0x00004000
	FILE_OPEN_REPARSE_POINT     = 0x00200000

	// https://learn.microsoft.com/en-us/windows/win32/api/ntdef/nf-ntdef-initializeobjectattributes
	OBJ_CASE_INSENSITIVE = 0x00000040

	FSCTL_GET_REPARSE_POINT = 0x000900A8

	// 16 KB
	MAXIMUM_REPARSE_DATA_BUFFER_SIZE = 16 * 1024
)

// These structures are described
// in https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-fscc/ca069dad-ed16-42aa-b057-b6b207f447cc
// and https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-fscc/b41f1cbf-10df-4a47-98d4-1c52a833d913.
type REPARSE_DATA_BUFFER struct {
	ReparseTag        uint32
	ReparseDataLength uint16
	Reserved          uint16
	DUMMYUNIONNAME    byte
}

type SymbolicLinkReparseBuffer struct {
	// The integer that contains the offset, in bytes,
	// of the substitute name string in the PathBuffer array,
	// computed as an offset from byte 0 of PathBuffer. Note that
	// this offset must be divided by 2 to get the array index.
	SubstituteNameOffset uint16

	// The integer that contains the length, in bytes, of the
	// substitute name string. If this string is null-terminated,
	// SubstituteNameLength does not include the Unicode null character.
	SubstituteNameLength uint16

	// PrintNameOffset is similar to SubstituteNameOffset.
	PrintNameOffset uint16

	// PrintNameLength is similar to SubstituteNameLength.
	PrintNameLength uint16

	// Flags specifies whether the substitute name is a full path name or
	// a path name relative to the directory containing the symbolic link.
	Flags      uint32
	PathBuffer [1]uint16
}

// Path returns path stored in rb.
func (rb *SymbolicLinkReparseBuffer) Path() string {
	n1 := rb.SubstituteNameOffset / 2
	n2 := (rb.SubstituteNameOffset + rb.SubstituteNameLength) / 2

	return syscall.UTF16ToString((*[0xffff]uint16)(unsafe.Pointer(&rb.PathBuffer[0]))[n1:n2:n2])
}

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

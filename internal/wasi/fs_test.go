package internalwasi

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"testing"
	"testing/fstest"
	"testing/iotest"

	"github.com/stretchr/testify/require"
)

//
// fs_test.go tests structs and methods defined fs.go.
// Types in fs.go often implements fs.FS or fs.File. fs.FS provides a test function to test the behavior
// of read-only fs.FS, and iotest provides a test function to test io.Reader and io.Seeker interfaces.
// So, tests in this file utilizes those generic test functions for read-only operations if possible.
//

// TestMemFile_Read_Seek tests the behavior of Read and Seek by iotest.TestReader.
// See iotest.TestReader
func TestMemFile_Read_Seek(t *testing.T) {
	expectedFileContent := []byte("wazero") // arbitrary contents
	memFile := &memFile{
		fsEntry: &memFSEntry{Contents: expectedFileContent},
	}
	// TestReader tests that io.Reader correctly reads the expected contents.
	// It also tests io.Seeker when it's implemented, which memFile does.
	err := iotest.TestReader(memFile, expectedFileContent)
	require.NoError(t, err)
}

// TestMemFile_Seek_Errors tests the error result of Seek, which iotest.TestReader does not test.
func TestMemFile_Seek_Errors(t *testing.T) {
	memFile := &memFile{
		fsEntry: &memFSEntry{Contents: []byte("wazero")},
	}

	tests := []struct {
		name   string
		offset int64
		whence int
		err    string
	}{
		{
			name:   "seek to negative",
			offset: -1,
			whence: io.SeekStart,
			err:    "invalid new offset: -1",
		},
		{
			name:   "seek to more than file length",
			offset: 1,
			whence: io.SeekEnd,
			err:    fmt.Sprintf("invalid new offset: %d", len(memFile.fsEntry.Contents)+1),
		},
		{
			name:   "invalid whence",
			offset: 1,
			whence: 42, // arbitrary invalid whence
			err:    fmt.Sprintf("invalid whence: %d", 42),
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			_, err := memFile.Seek(tc.offset, tc.whence)
			require.EqualError(t, err, tc.err)
		})
	}
}

// TestMemFS_FSInterface tests the behavior of fs.FS interface of MemFS by fstest.TestFS.
// It includes testing the behavior of fs.FS.Open, and fs.File.(Read, Seek, Stat, ReadDir)
// See io/fs.FS
// See io/fs.File
// See fstest.TestFS
func TestMemFS_FSInterface(t *testing.T) {
	memFS := &MemFS{
		"file1": {Contents: []byte("wazero")},
		"directory1": {
			Entries: map[string]*memFSEntry{
				"file1": {Contents: []byte("wazero")},
				"file2": {Contents: []byte("wazero")},
				"nested_directory1": {
					Entries: map[string]*memFSEntry{
						"file": {Contents: []byte("wazero")},
					},
				},
				"nested_directory2": {Entries: map[string]*memFSEntry{}}, // nested directory with no children
			},
		},
		"directory2": {Entries: map[string]*memFSEntry{}}, // directory with no children
	}

	// TestFS tests that fs.FS correctly does the expected operations.
	err := fstest.TestFS(memFS,
		"file1",
		"directory1",
		"directory1/file1",
		"directory1/file2",
		"directory1/nested_directory1",
		"directory1/nested_directory1/file",
		"directory1/nested_directory2",
		"directory2")
	require.NoError(t, err)
}

// TestMemFS_OpenFile_Flags tests OpenFile with flags such as os.O_CREATE.
// Note that read-only operations are tested thoroughly via TestMemFS_FSInterface instead of this test.
func TestMemFS_OpenFile_Flags(t *testing.T) {
	tests := []struct {
		testName       string
		fileName       string
		flag           int
		expectedResult *memFile
		expectedFS     func(openFileResult *memFile) *MemFS
	}{
		{
			testName: "new file",
			fileName: "new file", // arbitrary new file name
			flag:     os.O_CREATE,
			expectedResult: &memFile{
				name:    "new file",
				fsEntry: &memFSEntry{},
			},
			expectedFS: func(expectedResult *memFile) *MemFS {
				return &MemFS{
					"new file": expectedResult.fsEntry,
					"file":     {Contents: []byte("simple cont")},
					"dir":      {Entries: map[string]*memFSEntry{}},
				}
			},
		},
		{
			testName: "new file inside a directory",
			fileName: "dir/new file", // arbitrary new file name
			flag:     os.O_CREATE,
			expectedResult: &memFile{
				name:    "new file",
				fsEntry: &memFSEntry{},
			},
			expectedFS: func(expectedResult *memFile) *MemFS {
				return &MemFS{
					"file": {Contents: []byte("simple cont")},
					"dir": {
						Entries: map[string]*memFSEntry{
							"new file": expectedResult.fsEntry,
						},
					},
				}
			},
		},
		{
			testName: "O_CREATE should have no effect on existing file",
			fileName: "file", // existing file
			flag:     os.O_CREATE,
			expectedResult: &memFile{
				name: "file",
				fsEntry: &memFSEntry{
					Contents: []byte("simple cont"),
				},
			},
			expectedFS: func(expectedResult *memFile) *MemFS {
				return &MemFS{
					"file": expectedResult.fsEntry,
					"dir":  {Entries: map[string]*memFSEntry{}},
				}
			},
		},
		{
			testName: "O_TRUNC should trunc the contents",
			fileName: "file", // existing file
			flag:     os.O_TRUNC,
			expectedResult: &memFile{
				name: "file",
				fsEntry: &memFSEntry{
					Contents: []byte{}, // must be truncated
				},
			},
			expectedFS: func(expectedResult *memFile) *MemFS {
				return &MemFS{
					"file": expectedResult.fsEntry,
					"dir":  {Entries: map[string]*memFSEntry{}},
				}
			},
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.testName, func(t *testing.T) {
			memFS := &MemFS{
				"file": {Contents: []byte("simple cont")},
				"dir":  {Entries: map[string]*memFSEntry{}},
			}

			file, err := memFS.OpenFile(tc.fileName, tc.flag, 0)
			require.NoError(t, err)
			require.Equal(t, tc.expectedResult, file.(*memFile))
			require.Equal(t, tc.expectedFS(tc.expectedResult), memFS)
		})
	}
}

// TestMemFS_OpenFile_Flags_Errors tests that OpenFile with flags returns expected errors.
func TestMemFS_OpenFile_Flags_Errors(t *testing.T) {
	memFS := &MemFS{
		"file": {
			Contents: []byte("simple cont"),
		},
		"dir": {
			Entries: map[string]*memFSEntry{},
		},
	}

	tests := []struct {
		testName      string
		fileName      string
		flag          int
		expectedError error
	}{
		{
			testName:      "no file exists",
			fileName:      "new file", // arbitrary new file name
			flag:          0,          // no O_CREATE
			expectedError: fs.ErrNotExist,
		},
		{
			testName: "file must not exist when O_EXCL is specified",
			fileName: "new file", // arbitrary new file name
			flag:     os.O_CREATE | os.O_EXCL,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.testName, func(t *testing.T) {

			_, err := memFS.OpenFile(tc.fileName, tc.flag, 0)
			require.ErrorIs(t, err, tc.expectedError)
		})
	}
}

// TestReaderWriterFile_Read tests the behavior of Read by iotest.TestReader.
// See iotest.TestReader
func TestReaderWriterFile_Read(t *testing.T) {
	tests := []struct {
		name            string
		expectedContent []byte
		reader          io.Reader
	}{
		{
			name:            "simple read",
			expectedContent: []byte("wazero"),
			reader:          bytes.NewBuffer([]byte("wazero")),
		},
		{
			name:            "nil reader reads empty content",
			expectedContent: []byte{},
			reader:          nil,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			file := &readerWriterFile{
				reader: tc.reader,
			}
			// TestReader tests that io.Reader correctly reads the expected contents.
			err := iotest.TestReader(file, tc.expectedContent)
			require.NoError(t, err)
		})
	}
}

func TestReaderWriterFile_Write(t *testing.T) {
	t.Run("simple write", func(t *testing.T) {
		writer := bytes.NewBuffer([]byte{})
		file := &readerWriterFile{
			writer: writer,
		}
		content := []byte("wazero")
		nwritten, err := file.Write(content)
		require.NoError(t, err)
		require.Equal(t, len(content), nwritten)
		require.Equal(t, content, writer.Bytes())
	})

	t.Run("nil writer returns no error", func(t *testing.T) {
		file := &readerWriterFile{
			writer: nil,
		}
		content := []byte("wazero")
		nwritten, err := file.Write(content)
		require.NoError(t, err)
		require.Equal(t, len(content), nwritten)
	})
}

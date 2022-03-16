package internalwasi

import (
	"bytes"
	"io"
	"io/fs"
	"os"
	"testing"
	"testing/fstest"
	"testing/iotest"

	"github.com/stretchr/testify/require"
)

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

// TestMemFS_FSInterface tests the behavior of fs.FS interface of MemFS by fstest.TestFS.
// It includes testing the behavior of fs.FS.Open, and fs.File.(Read, Seek, Stat, ReadDir)
// See io/fs.FS
// See io/fs.File
// See fstest.TestFS
func TestMemFS_FSInterface(t *testing.T) {
	memFS := &MemFS{
		"simple file": {Contents: []byte("simple cont")},
		"directory": {
			Entries: map[string]*memFSEntry{
				"file1 inside directory": {Contents: []byte("simple cont file1 inside directory")},
				"file2 inside directory": {Contents: []byte("simple cont file2 inside directory")},
				"nested directory": {
					Entries: map[string]*memFSEntry{
						"file1 inside directory": {Contents: []byte("simple cont file1 inside directory")},
					},
				},
				"nested directory with no children": {Entries: map[string]*memFSEntry{}},
			},
		},
		"directory with no children": {Entries: map[string]*memFSEntry{}},
	}

	// TestFS tests that fs.FS correctly does the expected operations.
	err := fstest.TestFS(memFS,
		"simple file",
		"directory",
		"directory/file1 inside directory",
		"directory/file2 inside directory",
		"directory/nested directory",
		"directory/nested directory/file1 inside directory",
		"directory/nested directory with no children",
		"directory with no children")
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
				Reader: tc.reader,
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
			Writer: writer,
		}
		content := []byte("wazero")
		// TestReader tests that io.Reader correctly reads the expected contents.
		nwritten, err := file.Write(content)
		require.NoError(t, err)
		require.Equal(t, len(content), nwritten)
		require.Equal(t, content, writer.Bytes())
	})

	t.Run("nil writer returns no error", func(t *testing.T) {
		file := &readerWriterFile{
			Writer: nil,
		}
		content := []byte("wazero")
		// TestReader tests that io.Reader correctly reads the expected contents.
		nwritten, err := file.Write(content)
		require.NoError(t, err)
		require.Equal(t, len(content), nwritten)
	})
}

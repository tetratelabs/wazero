package fs

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"syscall"
)

func Main() {
	testAdHoc()
}

func testAdHoc() {
	if wd, err := syscall.Getwd(); err != nil {
		log.Panicln(err)
	} else if wd != "/" {
		log.Panicln("not root")
	}
	fmt.Println("wd ok")

	if err := syscall.Chdir("/animals.txt"); err == nil {
		log.Panicln("shouldn't be able to chdir to file")
	} else {
		fmt.Println(err) // should be the textual message of the errno.
	}

	// Ensure stat works, particularly mode.
	for _, path := range []string{"sub", "/animals.txt", "animals.txt"} {
		if stat, err := os.Stat(path); err != nil {
			log.Panicln(err)
		} else {
			fmt.Println(path, "mode", stat.Mode())
		}
	}

	// Read the full contents of the file using io.Reader
	b, err := os.ReadFile("/animals.txt")
	if err != nil {
		log.Panicln(err)
	}
	fmt.Println("contents:", string(b))

	// Re-open the same file to test io.ReaderAt
	f, err := os.Open("/animals.txt")
	if err != nil {
		log.Panicln(err)
	}
	defer f.Close()

	// Seek to an arbitrary position.
	if _, err = f.Seek(4, io.SeekStart); err != nil {
		log.Panicln(err)
	}

	b1 := make([]byte, len(b))
	// We expect to revert to the original position on ReadAt zero.
	if _, err = f.ReadAt(b1, 0); err != nil {
		log.Panicln(err)
	}

	if !bytes.Equal(b, b1) {
		log.Panicln("unexpected ReadAt contents: ", string(b1))
	}

	b, err = os.ReadFile("/empty.txt")
	if err != nil {
		log.Panicln(err)
	}
	fmt.Println("empty:" + string(b))
}

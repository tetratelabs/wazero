package fs

import (
	"fmt"
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

	b, err := os.ReadFile("/animals.txt")
	if err != nil {
		log.Panicln(err)
	}
	fmt.Println("contents:", string(b))

	b, err = os.ReadFile("/empty.txt")
	if err != nil {
		log.Panicln(err)
	}
	fmt.Println("empty:" + string(b))
}

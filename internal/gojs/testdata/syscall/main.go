package syscall

import (
	"fmt"
	"log"
	"os"
	"syscall"
)

func Main() {
	fmt.Printf("syscall.Getpid()=%d\n", syscall.Getpid())
	fmt.Printf("syscall.Getppid()=%d\n", syscall.Getppid())
	fmt.Printf("syscall.Getuid()=%d\n", syscall.Getuid())
	fmt.Printf("syscall.Getgid()=%d\n", syscall.Getgid())
	fmt.Printf("syscall.Geteuid()=%d\n", syscall.Geteuid())
	fmt.Printf("syscall.Umask(0077)=%O\n", syscall.Umask(0077))
	if g, err := syscall.Getgroups(); err != nil {
		log.Panicln(err)
	} else {
		fmt.Printf("syscall.Getgroups()=%v\n", g)
	}

	if p, err := os.FindProcess(syscall.Getpid()); err != nil {
		log.Panicln(err)
	} else {
		fmt.Printf("os.FindProcess(pid)=%v\n", p)
	}
}

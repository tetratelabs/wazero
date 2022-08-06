package main

import (
	"fmt"
	"time"
)

func main() {
	fmt.Println(time.Local.String()) // trigger initLocal
	t := time.Now()                  // uses walltime
	fmt.Println(time.Since(t))       // uses nanotime1
}

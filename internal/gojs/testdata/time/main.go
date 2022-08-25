package time

import (
	"fmt"
	"time"
)

func Main() {
	fmt.Println(time.Local.String()) // trigger initLocal
	t := time.Now()                  // uses walltime
	fmt.Println(time.Since(t))       // uses nanotime1
}

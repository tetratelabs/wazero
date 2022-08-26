package goroutine

import "fmt"

func Main() {
	msg := make(chan int)
	finished := make(chan int)
	go func() {
		<-msg
		fmt.Println("consumer")
		finished <- 1
	}()
	go func() {
		fmt.Println("producer")
		msg <- 1
	}()
	<-finished
}

package main

func main() {}

//export call_host_func
func callHostFunc(cnt int32) {
	for i := int32(0); i < cnt; i++ {
		hostFunc()
	}
}

//export host_func
func hostFunc()

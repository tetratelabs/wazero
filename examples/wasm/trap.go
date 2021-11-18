package main

func main() {}

//export cause_panic
func causePanic() {
	one()
}

func one() {
	two()
}

func two() {
	three()
}

func three() {
	panic("causing panic!!!!!!!!!!")
}

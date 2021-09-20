package main

import (
	"fmt"
)

const maxBufferSize = 512 // bytes
const apiAddr = "127.0.0.1:420"
const udpAddr = "127.0.0.1:69"

func main() {
	fmt.Println(" * starting...")

	whoDied := make(chan int)

	// start HTTP api handler
	api := make(chan error)
	startAPI(whoDied, api)

	// start UDP handler
	udp := make(chan error)
	startUDP(whoDied, udp)

	// listen for errors
	select {
	case <-api:
		whoDied <- 1
		break
	case <-udp:
		whoDied <- 1
		break
	}

	fmt.Println(" ! shutting down.")
}

package main

import (
	"crypto/cipher"
	"flag"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

const maxBufferSize = 512 // bytes

var apiAddr string // = "127.0.0.1:420"
var udpAddr string // = "127.0.0.1:"
var udpPorts chan int

var kill chan int

var apiDied chan error

// var udpDied chan error

type Data struct {
	Time int64   `json:"time"`
	Lat  float32 `json:"latitude"`
	Lon  float32 `json:"longitude"`
}
type Session struct {
	addr      string
	port      int
	key       []byte
	token     uuid.UUID
	decr      cipher.Block // the block size == 16 bytes
	die       chan int
	StartTime time.Time      `json:"start"`
	EndTime   time.Time      `json:"end"`
	data      map[int64]Data // TODO -- make this a redis cache
	Data      []Data         `json:"data"` // sorted exported version
}

var sessions map[int]Session = make(map[int]Session) // port->data

func main() {
	// setup command-line args
	apiAddrPtr := flag.String("api-addr", "127.0.0.1", "ip-address for API handler")
	apiPortPtr := flag.Int("api-port", 420, "port for API handler")
	udpAddrPtr := flag.String("udp-addr", "127.0.0.1", "ip-address for UDP server")
	udpPortsPtr := flag.String("udp-ports", "69,71-73", "list of ports available for UDP server -- comma-separated list, use '-' to specify port range")
	flag.Parse()

	// verify args
	r, err := regexp.Compile(`^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}$`)
	if err != nil {
		fmt.Printf("err %v\n", err)
		return
	}
	if !r.MatchString(*apiAddrPtr) {
		fmt.Printf("invalid api-addr %v\n", *apiAddrPtr)
		return
	}
	if !r.MatchString(*udpAddrPtr) {
		fmt.Printf("invalid udp-addr %v\n", *udpAddrPtr)
		return
	}
	apiAddr = *apiAddrPtr + ":" + strconv.Itoa(*apiPortPtr)
	udpAddr = *udpAddrPtr
	var udpPortsList []int
	for _, port := range strings.Split(*udpPortsPtr, ",") {
		if strings.Contains(port, "-") {
			pp := strings.Split(port, "-")
			if len(pp) != 2 {
				fmt.Printf("invalid port range %v\n", port)
				return
			}
			p0, err := strconv.Atoi(pp[0])
			if err != nil {
				fmt.Printf("invalid start port in range %v\n", port)
				return
			}
			p1, err := strconv.Atoi(pp[1])
			if err != nil {
				fmt.Printf("invalid end port in range %v\n", port)
				return
			}
			for i := p0; i <= p1; i++ {
				udpPortsList = append(udpPortsList, i)
			}
		} else {
			p0, err := strconv.Atoi(port)
			if err != nil {
				fmt.Printf("invalid port %v\n", port)
				return
			}
			udpPortsList = append(udpPortsList, p0)
		}
	}
	udpPorts = make(chan int, len(udpPortsList))
	for _, v := range udpPortsList {
		udpPorts <- v
	}
	fmt.Printf(" > UDP port pool %v\n", udpPortsList)

	// start up handlers
	fmt.Println(" * starting server...")

	kill = make(chan int, 1)

	// start HTTP api handler
	apiDied = make(chan error, 1)
	startAPI(apiDied)

	// listen for errors
	// select {
	// case <-apiDied:
	// 	kill <- 1
	// 	break
	// case <-udpDied: // not needed...yet
	// 	kill <- 1
	// 	break
	// }
	fmt.Printf(" ! API error %v\n", <-apiDied)
	kill <- 1

	fmt.Println(" ! shutting down.")
}

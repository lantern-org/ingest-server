package main

import (
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"flag"
	"io"
	"log"
	"math/rand"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const MAX_BUFFER_SIZE = 256 // bytes
const PACKET_LENGTH = 48    // bytes

var apiAddr string // = "127.0.0.1:420"
var udpAddr string // = "127.0.0.1:"
var udpType string // "udp" or "unixgram"
var udpPorts chan int

var users map[string][]byte = make(map[string][]byte) // username->hashedPassword

var kill chan int

var apiDied chan error

type Data struct {
	Version  uint32  `json:"version"`
	Index    uint32  `json:"index"`
	Time     int64   `json:"time"`      // unix epoch
	Lat      float32 `json:"latitude"`  // degrees
	Lon      float32 `json:"longitude"` // degrees
	Acc      float32 `json:"accuracy"`  // radius meters
	Internet byte    `json:"internet"`  // 0-4 (relative internet signal strength)
}
type Session struct {
	addr       string
	Port       int `json:"port"`
	key        []byte
	token      uuid.UUID
	code       string       // 4-char code
	decr       cipher.Block // the block size == 16 bytes
	die        chan int
	udpLoopEnd chan int
	StartTime  time.Time `json:"start"`
	EndTime    time.Time `json:"end"`
	Data       []Data    `json:"data"` // will be sorted
	PacErrs    int       `json:"num_error_packets"`
}

var sessions map[int]*Session = make(map[int]*Session) // port->data
var sessionsLock = sync.RWMutex{}

var codes map[string]int = make(map[string]int) // code->port
var codesLock = sync.RWMutex{}

func main() {
	rand.Seed(time.Now().UnixNano())
	{
		path, err := os.Getwd()
		log.Printf(" > context directory: %q (error: %v)\n", path, err)
		path, err = os.Executable()
		log.Printf(" > executable location: %q (error: %v)\n", path, err)
	}
	// setup command-line args
	apiAddrPtr := flag.String("api-addr", "", "ip-address for API handler")
	apiPortPtr := flag.Int("api-port", 1025, "port for API handler (>1025, unless running as root)")
	udpAddrPtr := flag.String("udp-addr", "/tmp", "root folder for UDP server unix sockets (OR ip address)")
	// https://en.wikipedia.org/wiki/List_of_TCP_and_UDP_port_numbers
	udpPortsPtr := flag.String("udp-ports", "42069,49152-65535", "list of ports available for UDP server -- comma-separated list, use '-' to specify port range")
	userDatabaseFilenamePtr := flag.String("user-file", "database.json", "json user database file")
	// TODO -- verbose printing
	flag.Parse()

	// verify args
	r, err := regexp.Compile(`^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}$`)
	if err != nil {
		log.Printf(" ! err %v\n", err)
		return
	}
	// ensure api address is a valid address
	if !r.MatchString(*apiAddrPtr) && *apiAddrPtr != "" && *apiAddrPtr != "localhost" {
		log.Printf(" ! invalid api-addr %v\n", *apiAddrPtr)
		return
	}
	// ensure udp is valid address or folder
	if r.MatchString(*udpAddrPtr) || *udpAddrPtr == "" || *udpAddrPtr == "localhost" {
		log.Printf(" > using internet address %v for UDP packets\n", *udpAddrPtr)
		udpAddr = *udpAddrPtr
		udpType = "udp"
	} else if _, err := os.ReadDir(*udpAddrPtr); err == nil {
		// todo -- test permissions?
		log.Printf(" > using unix sockets in %v\n for UDP packets", *udpAddrPtr)
		udpAddr = *udpAddrPtr
		udpType = "unixgram"
	} else {
		log.Printf(" ! invalid udp address %v\n", *udpAddrPtr)
		return
	}

	apiAddr = *apiAddrPtr + ":" + strconv.Itoa(*apiPortPtr)
	udpAddr = *udpAddrPtr
	// ensure udp ports are valid
	var udpPortsList []int
	for _, port := range strings.Split(*udpPortsPtr, ",") {
		// todo -- check port numbers?
		if strings.Contains(port, "-") {
			pp := strings.Split(port, "-")
			if len(pp) != 2 {
				log.Printf(" ! invalid port range %v\n", port)
				return
			}
			p0, err := strconv.Atoi(pp[0])
			if err != nil {
				log.Printf(" ! invalid start port in range %v\n", port)
				return
			}
			p1, err := strconv.Atoi(pp[1])
			if err != nil {
				log.Printf(" ! invalid end port in range %v\n", port)
				return
			}
			for i := p0; i <= p1; i++ {
				udpPortsList = append(udpPortsList, i)
			}
		} else {
			p0, err := strconv.Atoi(port)
			if err != nil {
				log.Printf(" ! invalid port %v\n", port)
				return
			}
			udpPortsList = append(udpPortsList, p0)
		}
	}
	udpPorts = make(chan int, len(udpPortsList))
	for _, v := range udpPortsList { // add udp ports to revolving channel
		udpPorts <- v
	}
	if len(udpPortsList) > 10 {
		log.Printf(" > UDP port pool %v ... [%v]\n", udpPortsList[0:10], udpPortsList[len(udpPortsList)-1])
	} else {
		log.Printf(" > UDP port pool %v\n", udpPortsList)
	}
	// pull in, parse, verify user database
	file, err := os.Open(*userDatabaseFilenamePtr)
	if err != nil {
		log.Printf(" ! issue opening %v (error: %v)\n", *userDatabaseFilenamePtr, err)
		return
	}
	filebytes, err := io.ReadAll(file)
	if err != nil {
		log.Printf(" ! issue reading %v (error: %v)\n", *userDatabaseFilenamePtr, err)
		return
	}
	type User struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	var readUsers []User
	err = json.Unmarshal(filebytes, &readUsers)
	if err != nil {
		log.Printf(" ! issue unmarshaling %v (error: %v)\n", *userDatabaseFilenamePtr, err)
		return
	}
	for _, u := range readUsers {
		b, err := base64.RawStdEncoding.DecodeString(u.Password)
		if err != nil {
			log.Printf(" ! there was some error decoding %v's password (error: %v)\n", u.Username, err)
		} else {
			users[u.Username] = b
		}
	}
	log.Printf(" > using user database with %d users\n", len(readUsers))

	// start up handlers
	log.Println(" * starting server...")

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
	log.Printf(" ! API error %v\n", <-apiDied)
	kill <- 1

	log.Println(" ! shutting down.")
}

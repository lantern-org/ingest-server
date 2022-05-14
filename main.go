package main

import (
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
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

var pauseDuration time.Duration
var stopDuration time.Duration

var users map[string][]byte = make(map[string][]byte) // username->hashedPassword

var kill chan int

var apiDied chan error

type Data struct {
	Version   uint16  `json:"version"`
	Index     uint32  `json:"index"`
	Time      int64   `json:"time"`      // unix epoch
	Lat       float32 `json:"latitude"`  // degrees
	Lon       float32 `json:"longitude"` // degrees
	Acc       float32 `json:"accuracy"`  // radius meters
	Internet  byte    `json:"internet"`  // 0-4 (relative internet signal strength)
	Processed int64   `json:"processed"` // time at which this packet was processed
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
	pause      *time.Ticker
	paused     chan bool
	StartTime  time.Time `json:"start"`
	EndTime    time.Time `json:"end"`
	Data       []Data    `json:"data"` // will be sorted
	dataLock   sync.RWMutex
	PacErrs    int `json:"num_error_packets"`
}

var sessions map[int]*Session = make(map[int]*Session) // port->data
var sessionsLock = sync.RWMutex{}

var codes map[string]int = make(map[string]int) // code->port
var codesLock = sync.RWMutex{}

type ports map[int]bool

func (p *ports) String() string {
	return ""
}
func (p *ports) Set(value string) error {
	// check initial value of *ports?
	for _, port := range strings.Split(value, ",") {
		// https://en.wikipedia.org/wiki/List_of_TCP_and_UDP_port_numbers
		if strings.Contains(port, "-") {
			pp := strings.Split(port, "-")
			if len(pp) != 2 {
				return fmt.Errorf("invalid port range %v", port)
			}
			p0, err := strconv.Atoi(pp[0])
			if err != nil || p0 <= 0 {
				return fmt.Errorf("invalid start port in provided range %v", port)
			}
			p1, err := strconv.Atoi(pp[1])
			if err != nil || p1 >= 65536 {
				return fmt.Errorf("invalid end port in range %v", port)
			}
			for i := p0; i <= p1; i++ {
				(*p)[i] = true
			}
		} else {
			p0, err := strconv.Atoi(port)
			if err != nil || p0 <= 0 || p0 >= 65536 {
				return fmt.Errorf("invalid port %v", port)
			}
			(*p)[p0] = true
		}
	}
	return nil
}

func main() {
	rand.Seed(time.Now().UnixNano())
	{
		path, err := os.Getwd()
		log.Printf(" > context directory: %q (error: %v)\n", path, err)
		path, err = os.Executable()
		log.Printf(" > executable location: %q (error: %v)\n", path, err)
	}
	// setup command-line args
	flag.StringVar(&apiAddr, "api-addr", "", "ip-address for API handler")
	apiPortPtr := flag.Int("api-port", 1025, "port for API handler (>1025, unless running as root)")
	flag.StringVar(&udpAddr, "udp-addr", "/tmp", "UDP address; if unix sockets, provide root folder; if network, provide ip address")
	var udpPortsIn ports = map[int]bool{} // was "42069,49152-65535"
	flag.Var(&udpPortsIn, "udp-ports", "list of ports available for UDP server -- comma-separated list, use '-' to specify port range")
	var userDatabaseFilename string
	flag.StringVar(&userDatabaseFilename, "user-file", "database.json", "json user database file")
	flag.DurationVar(&pauseDuration, "pause-duration", 20*time.Second, "a data stream is considered \"paused\" if it has been this flag's length of time after which the most recent packet has been received")
	flag.DurationVar(&stopDuration, "stop-duration", 24*time.Hour, "a \"paused\" data stream is considered \"stopped\" (and will NOT restart) if it has been paused for this flag's length of time")

	// TODO -- verbose printing
	flag.Parse()

	// verify args
	r, err := regexp.Compile(`^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}$`)
	if err != nil {
		log.Printf(" ! err %v\n", err)
		return
	}
	// ensure api address is a valid address
	if !r.MatchString(apiAddr) && apiAddr != "" && apiAddr != "localhost" {
		log.Printf(" ! invalid api-addr %v\n", apiAddr)
		return
	}
	// ensure udp is valid address or folder
	if r.MatchString(udpAddr) || udpAddr == "" || udpAddr == "localhost" {
		log.Printf(" > using internet address %v for UDP packets\n", udpAddr)
		udpType = "udp"
	} else if _, err := os.ReadDir(udpAddr); err == nil {
		// todo -- test permissions?
		log.Printf(" > using unix sockets in %v\n for UDP packets", udpAddr)
		udpType = "unixgram"
	} else {
		log.Printf(" ! invalid udp address %v\n", udpAddr)
		return
	}

	apiAddr += ":" + strconv.Itoa(*apiPortPtr)
	// ensure udp ports are valid
	udpPorts = make(chan int, len(udpPortsIn))
	numPorts := 0
	for p, b := range udpPortsIn {
		if b { // should always be true?
			udpPorts <- p // add udp ports to revolving channel
			numPorts += 1
		}
	}
	log.Printf(" > UDP port pool has %v ports\n", numPorts)
	// pull in, parse, verify user database
	file, err := os.Open(userDatabaseFilename)
	if err != nil {
		log.Printf(" ! issue opening %v (error: %v)\n", userDatabaseFilename, err)
		return
	}
	filebytes, err := io.ReadAll(file)
	if err != nil {
		log.Printf(" ! issue reading %v (error: %v)\n", userDatabaseFilename, err)
		return
	}
	type User struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	var readUsers []User
	err = json.Unmarshal(filebytes, &readUsers)
	if err != nil {
		log.Printf(" ! issue unmarshaling %v (error: %v)\n", userDatabaseFilename, err)
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

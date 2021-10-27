package main

import (
	"context"
	"crypto/aes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

func newPort() int {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case port, ok := <-udpPorts:
			if !ok {
				log.Println(" ! udpPorts closed??")
				port = -1
			}
			return port
		case <-ticker.C:
			return -1
		}
	}
}

// https://stackoverflow.com/questions/22892120/how-to-generate-a-random-string-of-a-fixed-length-in-go
const letterBytes = "ABCDEFGHIJKLMNOPQRSTUVWXYZABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789" // no more than 63 chars, for 6 bits in letterIdxBits
const (
	letterIdxBits = 6                    // 6 bits to represent a letter index (0b111110)
	letterIdxMask = 1<<letterIdxBits - 1 // All 1-bits, as many as letterIdxBits
	letterIdxMax  = 63 / letterIdxBits   // # of letter indices fitting in 63 bits
)

var src = rand.NewSource(time.Now().UnixNano())

func randString(n int) string {
	// we only need n=4, but if we ever decide to make n > 10
	// then the function will still work
	sb := strings.Builder{}
	sb.Grow(n)
	// A src.Int63() generates 63 random bits, enough for letterIdxMax characters!
	for i, cache, remain := n-1, src.Int63(), letterIdxMax; i >= 0; {
		if remain == 0 {
			cache, remain = src.Int63(), letterIdxMax
		}
		if idx := int(cache & letterIdxMask); idx < len(letterBytes) {
			sb.WriteByte(letterBytes[idx])
			i--
		}
		cache >>= letterIdxBits
		remain--
	}
	return sb.String()
}

func newCode() string {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			return ""
		default:
			code := randString(4)
			if _, ok := codes[code]; !ok {
				// found un-used code (highly likely)
				return code
			}
		}
	}
}

/*
TODO -- add timer for each UDP session that kills the connection
if it hasn't received an update in a while (30mins?)
*/

/*
TODO -- proper logging and error messages
*/

func startSession(w http.ResponseWriter, r *http.Request) {
	/*
		POST
		we receive session key securely over HTTPS
		{
			"username":string,
			"password":string,
			"key":string // string should be hex values encoded to characters
		}

		we use this session key to decrypt incoming UDP packets
		send back url for sending packets
		send back token to end session
		{"port":int,"token":string}
		or error
		{"error":string}
	*/
	// ensure POST
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusForbidden)
		w.Header().Add("Content-Type", "application/json")
		w.Write([]byte("{\"error\":\"invalid HTTP method\"}")) // handle error?
		return
	}
	// get POST body
	// TODO -- oauth2-type system
	var v struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Key      string `json:"key"`
	}
	err := json.NewDecoder(r.Body).Decode(&v) // does json.NewDecoder ever return nil ?
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Header().Add("Content-Type", "application/json")
		w.Write([]byte("{\"error\":\"" + err.Error() + "\"}")) // handle error?
		return
	}
	// validate user/pass
	if pw, ok := users[v.Username]; !ok {
		w.WriteHeader(http.StatusForbidden)
		w.Header().Add("Content-Type", "application/json")
		w.Write([]byte("{\"error\":\"get out of my swamp!\"}"))
		return
	} else {
		err := bcrypt.CompareHashAndPassword(pw, []byte(v.Password))
		if err != nil {
			w.WriteHeader(http.StatusForbidden)
			w.Header().Add("Content-Type", "application/json")
			w.Write([]byte("{\"error\":\"get out of my swamp!\"}"))
			return
		}
	}
	// decode given key
	key, err := hex.DecodeString(v.Key)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Header().Add("Content-Type", "application/json")
		w.Write([]byte("{\"error\":\"" + err.Error() + "\"}")) // handle error?
		return
	}
	// validate key
	if len(key) != 32 {
		w.WriteHeader(http.StatusBadRequest)
		w.Header().Add("Content-Type", "application/json")
		w.Write([]byte(fmt.Sprintf("{\"error\":\"invalid key length, received %d\"}", len(key)))) // handle error?
		return
	}
	// start new cipher based on the key
	decr, err := aes.NewCipher(key)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Add("Content-Type", "application/json")
		w.Write([]byte("{\"error\":\"" + err.Error() + "\"}")) // handle error?
		return
	}
	// start new session
	var port = newPort()
	// ok==true imples that port's session is still active (shouldn't happen unless there's an inconsistency)
	if _, ok := sessions[port]; port == -1 || ok { // TODO -- send to inside newPort ?
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Add("Content-Type", "application/json")
		w.Write([]byte("{\"error\":\"could not allocate session\"}")) // handle error?
		return
	}
	// todo -- test if unix socket exists? (if not using internet)
	//
	var code = newCode()
	if code == "" {
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Add("Content-Type", "application/json")
		w.Write([]byte("{\"error\":\"all session codes are being used...\"}")) // need to bump up n=4->5
		return
	}
	token := uuid.New()
	addr := ""
	if udpType == "udp" {
		addr = udpAddr + ":" + strconv.Itoa(port)
	} else { // assume uinxgram socket
		addr = fmt.Sprintf("%s/%d.sock", udpAddr, port)
	}
	s := Session{
		addr:       addr,
		port:       port,
		key:        key,
		token:      token,
		code:       code,
		decr:       decr,
		die:        make(chan int, 1),
		StartTime:  time.Now(),
		recentTime: 0,
		data:       make(map[int64]Data),
	}
	if !s.startUDP() {
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Add("Content-Type", "application/json")
		w.Write([]byte(fmt.Sprintf("{\"error\":\"could not start UDP listener on port %v\"}", port))) // handle error?
		return
	}
	sessions[port] = &s
	codes[code] = port

	w.WriteHeader(http.StatusOK)
	w.Header().Add("Content-Type", "application/json")
	w.Write([]byte(fmt.Sprintf("{\"port\":%d,\"token\":\"%s\",\"code\":\"%s\"}", s.port, token.String(), code)))
}

// TODO -- if server dies before user can end session, what do?
func stopSession(w http.ResponseWriter, r *http.Request) {
	/*
		POST
		we must receive the session token given during startSession
		{"port":int,"token":string}

		export in-memory data to file

		send back success
		{"success":true}
		or error
		{"error":string}
	*/
	// ensure POST
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusForbidden)
		w.Header().Add("Content-Type", "application/json")
		w.Write([]byte("{\"error\":\"invalid HTTP method\"}")) // handle error?
		return
	}
	// get POST body
	var v struct {
		Port  int    `json:"port"`
		Token string `json:"token"`
	}
	err := json.NewDecoder(r.Body).Decode(&v) // does json.NewDecoder ever return nil ?
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Header().Add("Content-Type", "application/json")
		w.Write([]byte("{\"error\":\"" + err.Error() + "\"}")) // handle error?
		return
	}
	// verify the request is valid
	s, ok := sessions[v.Port]
	if !ok || s.port != v.Port || s.token.String() != v.Token {
		w.WriteHeader(http.StatusBadRequest)
		w.Header().Add("Content-Type", "application/json")
		w.Write([]byte("{\"error\":\"failed\"}")) // handle error?
		return
	}
	if udpType == "unixgram" {
		err = os.Remove(s.addr)
		if err != nil {
			// log it, but it's ok i guess
			log.Printf("couldn't remove socket file %s\n", s.addr)
		} else {
			udpPorts <- s.port // free back the port (shouldn't block)
		}
	} else { // with ip addresses we don't have to remove a file
		udpPorts <- s.port // free back the port (shouldn't block)
	}
	s.die <- 1
	go func() {
		// export to file
		s.EndTime = time.Now()
		// s.StartTime.Format("2006-01-02_15-04-05")
		f, err := os.Create("data/" + s.token.String() + "_" + strconv.Itoa(s.port) + ".dat")
		if err != nil {
			log.Printf(" ! err: %v\n", err)
			return
		}
		i := 0
		s.Data = make([]Data, len(s.data))
		for _, d := range s.data {
			s.Data[i] = d
			i++
		}
		sort.Slice(s.Data, func(i, j int) bool { return s.Data[i].Time < s.Data[j].Time }) // slow
		j, err := json.MarshalIndent(s, "", "  ")
		if err != nil {
			// w.WriteHeader(http.StatusInternalServerError)
			// w.Header().Add("Content-Type", "application/json")
			// w.Write([]byte("{\"error\":\"" + err.Error() + "\"}")) // handle error?
			return
		}
		f.Write(j)
		f.Close() // don't care about error
		// finally, delete the session from our active map
		delete(sessions, v.Port)
		delete(codes, s.code)
	}()
	// assume go-routine succeeded
	w.WriteHeader(http.StatusOK)
	w.Header().Add("Content-Type", "application/json")
	w.Write([]byte("{\"success\":true}"))
}

func sessionInfo(w http.ResponseWriter, r *http.Request) {
	// w.Header().Set("Access-Control-Allow-Origin", "*") // ONLY FOR DEBUGGING ON LOCAL MACHINE
	/*
		GET
		parse session token from URL
		check for token in map(token->port)
		send back relevant info
		loc==lat,lon pair
		{"location":[float32, float32], "time":int32, "status":string}
		it's up to the caller to save locations to show a built-up route
		we _only_ give the most recently known location
		and status updates (TODO)
		(maybe you use a cookie in case the client refreshes the page?)

		on error, send back message that makes sense
		{"error":string}
	*/
	code := strings.TrimPrefix(r.URL.Path, "/location/") // TODO -- hard-coded
	port, ok := codes[code]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		w.Header().Add("Content-Type", "application/json")
		fmt.Fprintf(w, "{\"error\":\"could not find code '%v'\"}", code)
		return
	}
	sesh, ok := sessions[port]
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Add("Content-Type", "application/json")
		fmt.Fprintf(w, "{\"error\":\"illegal state error (code:'%v')\"}", code)
		return
	}
	data, ok := sesh.data[sesh.recentTime]
	if !ok {
		w.WriteHeader(http.StatusTooEarly)
		w.Header().Add("Content-Type", "application/json")
		fmt.Fprintf(w, "{\"error\":\"no data yet! (code:'%v')\"}", code)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Header().Add("Content-Type", "application/json")
	fmt.Fprintf(w, "{\"location\":[%f,%f],\"time\":%d,\"status\":\"%s\"}", data.Lat, data.Lon, data.Time, code)
}

// api handler
func startAPI(iDied chan<- error) {
	log.Println(" * starting API handler.")
	srv := &http.Server{Addr: apiAddr}
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%q", r.URL.Path)
	})
	http.HandleFunc("/session/start", startSession)
	http.HandleFunc("/session/stop", stopSession)
	http.HandleFunc("/location/", sessionInfo) // /location/CODE
	// /location/CODE/all
	// /active/CODE
	die := make(chan error)
	go func() {
		log.Println(" * API listening on " + apiAddr)
		err := srv.ListenAndServe()
		// err != http.ErrServerClosed
		die <- err
	}()
	go func() {
		select {
		case <-kill:
			kill <- 1
			srv.Shutdown(context.TODO())
			return
		case iDied <- <-die:
			return
		}
	}()
	log.Println(" * API handler started.")
}

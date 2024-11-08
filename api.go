package main

import (
	"context"
	"crypto/aes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
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

// https://pkg.go.dev/math/rand
// "The default Source is safe for concurrent use by multiple goroutines, but Sources created by NewSource are not."
// var src = rand.NewSource(time.Now().UnixNano())

func randString(n int) string {
	// we only need n=4, but if we ever decide to make n > 10
	// then the function will still work
	sb := strings.Builder{}
	sb.Grow(n)
	// A src.Int63() generates 63 random bits, enough for letterIdxMax characters!
	for i, cache, remain := n-1, rand.Int63(), letterIdxMax; i >= 0; {
		if remain == 0 {
			cache, remain = rand.Int63(), letterIdxMax
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
	codesLock.RLock()
	defer codesLock.RUnlock()
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
		{"port":int,"token":string,"code":string}
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
	} else if err := bcrypt.CompareHashAndPassword(pw, []byte(v.Password)); err != nil {
		w.WriteHeader(http.StatusForbidden)
		w.Header().Add("Content-Type", "application/json")
		w.Write([]byte("{\"error\":\"get out of my swamp!\"}"))
		return
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
	sessionsLock.RLock()
	if _, ok := sessions[port]; port == -1 || ok {
		sessionsLock.RUnlock()
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Add("Content-Type", "application/json")
		w.Write([]byte("{\"error\":\"could not allocate session\"}"))
		return
	}
	sessionsLock.RUnlock()
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
		Port:       port,
		key:        key,
		token:      token,
		code:       code,
		decr:       decr,
		die:        make(chan int, 2),
		udpLoopEnd: make(chan int, 1),
		paused:     make(chan bool, 1),
		StartTime:  time.Now(),
		Data:       make([]Data, 0, 36000), // pre-alloc -- avg 10 pac/sec for 1 hour = 60*60*10 = 36000
	}
	if !s.startUDP() {
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Add("Content-Type", "application/json")
		w.Write([]byte(fmt.Sprintf("{\"error\":\"could not start UDP listener on port %v\"}", port))) // handle error?
		return
	}
	sessionsLock.Lock()
	sessions[port] = &s
	sessionsLock.Unlock()
	codesLock.Lock()
	codes[code] = port
	codesLock.Unlock()

	w.WriteHeader(http.StatusOK)
	w.Header().Add("Content-Type", "application/json")
	w.Write([]byte(fmt.Sprintf("{\"port\":%d,\"token\":\"%s\",\"code\":\"%s\"}", s.Port, token.String(), code)))
}

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
	err := json.NewDecoder(r.Body).Decode(&v)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Header().Add("Content-Type", "application/json")
		w.Write([]byte("{\"error\":\"" + err.Error() + "\"}")) // handle error?
		return
	}
	// verify the request is valid
	sessionsLock.RLock()
	s, ok := sessions[v.Port]
	sessionsLock.RUnlock()
	if !ok || s.Port != v.Port || s.token.String() != v.Token {
		w.WriteHeader(http.StatusBadRequest)
		w.Header().Add("Content-Type", "application/json")
		w.Write([]byte("{\"error\":\"failed\"}")) // handle error?
		return
	}
	s.die <- 1 // signal to UDP handler that we're done
	// assume go-routine succeeded
	w.WriteHeader(http.StatusOK)
	w.Header().Add("Content-Type", "application/json")
	w.Write([]byte("{\"success\":true}"))
}

func sessionInfo(w http.ResponseWriter, r *http.Request) {
	// w.Header().Set("Access-Control-Allow-Origin", "*") // ONLY FOR DEBUGGING ON LOCAL MACHINE
	/*
		GET
		parse session code from URL
		check for code in map(code->port)
		send back relevant info
		(see Data struct)
		{
			"version":uint16, "index":uint32, "time":int64,
			"latitude":float32, "longitude":float32, "accuracy":float32,
			"internet":byte "processed":int64 "status":string
		}
		it's up to the caller to save locations to show a built-up route
		we _only_ give the most recently known location
		and status updates (TODO)
		(maybe you use a cookie in case the client refreshes the page?)

		on error, send back message that makes sense
		{"error":string}
	*/
	code := strings.TrimPrefix(r.URL.Path, "/location/") // TODO -- hard-coded
	codesLock.RLock()
	port, ok := codes[code]
	codesLock.RUnlock()
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		w.Header().Add("Content-Type", "application/json")
		fmt.Fprintf(w, "{\"error\":\"could not find code '%v'\"}", code)
		return
	}
	sessionsLock.RLock()
	sesh, ok := sessions[port]
	sessionsLock.RUnlock()
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Add("Content-Type", "application/json")
		fmt.Fprintf(w, "{\"error\":\"illegal state error (code:'%v')\"}", code)
		return
	}
	sesh.dataLock.RLock()
	if len(sesh.Data) == 0 {
		sesh.dataLock.RUnlock()
		w.WriteHeader(http.StatusTooEarly)
		w.Header().Add("Content-Type", "application/json")
		fmt.Fprintf(w, "{\"error\":\"no data yet! (code:'%v')\"}", code)
		return
	}
	data := sesh.Data[len(sesh.Data)-1] // !!! not guaranteed to be the latest update
	sesh.dataLock.RUnlock()
	status := "TODO" // TODO -- last-known status update
	select {
	case <-sesh.die:
		status = "Exited"
		break
	case p := <-sesh.paused:
		sesh.paused <- p
		if p {
			status = "Paused"
		}
	default:
	}
	w.WriteHeader(http.StatusOK)
	w.Header().Add("Content-Type", "application/json")
	json.NewEncoder(w).Encode(
		struct {
			Data
			Status string `json:"status"`
		}{
			data,
			status,
		},
	)
}

func tokenLog(w http.ResponseWriter, r *http.Request) {
	/*
		GET
		parse session token from URL
		attempt to read data/token.dat from file
		return file contents (should be json)
	*/
	token, err := uuid.Parse(strings.TrimPrefix(r.URL.Path, "/log/")) // TODO -- hard-coded; todo -- should be safe?
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		w.Header().Add("Content-Type", "application/json")
		fmt.Fprint(w, "{\"error\":\"could not find token\"}")
		return
	}
	file, err := os.Open(fmt.Sprintf("data/%s.dat", token)) // unsafe?
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		w.Header().Add("Content-Type", "application/json")
		fmt.Fprint(w, "{\"error\":\"could not find token\"}")
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Header().Add("Content-Type", "application/json")
	io.Copy(w, file) // todo -- test performance
}

func roomList(w http.ResponseWriter, r *http.Request) {
	/*
		GET
		send a list of all room codes
		[]string
	*/
	codesLock.RLock()
	codeList := make([]string, 0, len(codes))
	for k := range codes {
		codeList = append(codeList, k)
	}
	codesLock.RUnlock()
	// TODO sort rooms by most active? idk LOL
	w.WriteHeader(http.StatusOK)
	w.Header().Add("Content-Type", "application/json")
	json.NewEncoder(w).Encode(codeList)
}

// api handler
func startAPI(iDied chan<- error) {
	log.Println(" * starting API handler.")
	srv := &http.Server{Addr: apiAddr}
	// TODO use a real router library
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%q", r.URL.Path)
	})
	http.HandleFunc("/session/start", startSession)
	http.HandleFunc("/session/stop", stopSession)
	http.HandleFunc("/location/", sessionInfo) // /location/CODE
	// /active/CODE
	http.HandleFunc("/log/", tokenLog) // /log/TOKEN
	http.HandleFunc("/rooms", roomList)
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

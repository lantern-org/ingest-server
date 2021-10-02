package main

import (
	"context"
	"crypto/aes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/google/uuid"
)

func newPort() int {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case port, ok := <-udpPorts:
			if !ok {
				fmt.Println("udpPorts closed??")
				port = -1
			}
			return port
		case <-ticker.C:
			return -1
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
		{"key":string} -- string should be hex values encoded to characters

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
	var v struct {
		Key string `json:"key"`
	}
	err := json.NewDecoder(r.Body).Decode(&v) // does json.NewDecoder ever return nil ?
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Header().Add("Content-Type", "application/json")
		w.Write([]byte("{\"error\":\"" + err.Error() + "\"}")) // handle error?
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
	if _, ok := sessions[port]; port == -1 || ok {
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Add("Content-Type", "application/json")
		w.Write([]byte("{\"error\":\"could not allocate session\"}")) // handle error?
		return
	}
	token := uuid.New()
	s := Session{
		addr:      udpAddr + ":" + strconv.Itoa(port),
		port:      port,
		key:       key,
		token:     token,
		decr:      decr,
		die:       make(chan int, 1),
		StartTime: time.Now(),
		data:      make(map[int64]Data),
	}
	if !s.startUDP() {
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Add("Content-Type", "application/json")
		w.Write([]byte(fmt.Sprintf("{\"error\":\"could not start UDP listener on port %v\"}", port))) // handle error?
		return
	}
	sessions[port] = s

	w.WriteHeader(http.StatusOK)
	w.Header().Add("Content-Type", "application/json")
	w.Write([]byte(fmt.Sprintf("{\"port\":%d,\"token\":\"%s\"}", s.port, token.String()))) // handle error?
}

func endSession(w http.ResponseWriter, r *http.Request) {
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
	udpPorts <- s.port // free back the port (shouldn't block)
	s.die <- 1
	go func() {
		// export to file
		s.EndTime = time.Now()
		// s.StartTime.Format("2006-01-02_15-04-05")
		f, err := os.Create("data/" + s.token.String() + "_" + strconv.Itoa(s.port) + ".dat")
		if err != nil {
			fmt.Printf("err: %v\n", err)
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
	}()
	// assume go-routine succeeded
	w.WriteHeader(http.StatusOK)
	w.Header().Add("Content-Type", "application/json")
	w.Write([]byte("{\"success\":true}")) // handle error?
}

// api handler
func startAPI(iDied chan<- error) {
	fmt.Println(" * starting API handler.")

	srv := &http.Server{Addr: apiAddr}
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		//
		io.WriteString(w, "hello world\n")
		//
	})
	http.HandleFunc("/session/start", startSession)
	http.HandleFunc("/session/end", endSession)

	die := make(chan error)
	go func() {
		fmt.Println(" * API listening on " + apiAddr)
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

	fmt.Println(" * API handler started.")
}

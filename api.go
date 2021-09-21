package main

import (
	"context"
	"crypto/aes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

func kex(w http.ResponseWriter, r *http.Request) {
	/*
		POST
		we receive session key securely over HTTPS
		{"key":string} -- string should be hex values encoded to characters

		we use this session key to decrypt incoming UDP packets
		send back url for sending packets
		{"address":string}
		or error
		{"error":string}
	*/
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusForbidden)
		w.Header().Add("Content-Type", "application/json")
		w.Write([]byte("{\"error\":\"invalid HTTP method\"}")) // handle error?
		return
	}
	var v struct {
		Key string `json:"key"`
	}
	// x, _ := ioutil.ReadAll(r.Body)
	// fmt.Printf("%v\n", x)
	err := json.NewDecoder(r.Body).Decode(&v) // does json.NewDecoder ever return nil ?
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Header().Add("Content-Type", "application/json")
		w.Write([]byte("{\"error\":\"" + err.Error() + "\"}")) // handle error?
		return
	}
	key, err = hex.DecodeString(v.Key)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Header().Add("Content-Type", "application/json")
		w.Write([]byte("{\"error\":\"" + err.Error() + "\"}")) // handle error?
		return
	}
	if len(key) != 32 {
		w.WriteHeader(http.StatusBadRequest)
		w.Header().Add("Content-Type", "application/json")
		w.Write([]byte(fmt.Sprintf("{\"error\":\"invalid key length, received %d\"}", len(key)))) // handle error?
		return
	}
	d, err = aes.NewCipher(key)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Add("Content-Type", "application/json")
		w.Write([]byte("{\"error\":\"" + err.Error() + "\"}")) // handle error?
		return
	}
	// TODO -- randomize the port, and only send the port
	w.WriteHeader(http.StatusOK)
	w.Header().Add("Content-Type", "application/json")
	w.Write([]byte("{\"address\":\"" + udpAddr + "\"}")) // handle error?
}

// api handler
func startAPI(uDied <-chan int, iDied chan<- error) {
	fmt.Println(" * starting API handler.")

	srv := &http.Server{Addr: apiAddr}
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		//
		io.WriteString(w, "hello world\n")
		//
	})
	http.HandleFunc("/session", kex)

	die := make(chan error)
	go func() {
		fmt.Println(" * API listening on " + apiAddr)
		err := srv.ListenAndServe()
		// err != http.ErrServerClosed
		die <- err
	}()
	go func() {
		select {
		case <-uDied:
			srv.Shutdown(context.TODO())
			break
		case iDied <- <-die:
			break
		}
	}()

	fmt.Println(" * API handler started.")
}

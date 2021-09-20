package main

import (
	"context"
	"fmt"
	"io"
	"math/big"
	"net/http"
)

// unchanged
var (
	p = big.NewInt(997) // modulus
	g = big.NewInt(100) // base
)

func kex(w http.ResponseWriter, r *http.Request) {
	/*
		we receive session key securely over HTTPS

		we use this session key to decrypt incoming UDP packets
	*/
	fmt.Printf("%v,%v", p, g)
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

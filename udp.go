package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"time"
)

// the block size == 16 bytes
var d cipher.Block

func decrypt(c []byte) []byte {
	// use key with AES-256
	p := make([]byte, len(c))
	numBlocks := len(c) / aes.BlockSize
	for i := 0; i < numBlocks; i++ {
		d.Decrypt(p[i*aes.BlockSize:(i+1)*aes.BlockSize], c[i*aes.BlockSize:(i+1)*aes.BlockSize])
	}
	return p
}

func toAngle(b []byte) string {
	val := binary.BigEndian.Uint32(b)
	i := float32((val & 0x07fe0000) >> 17)
	f := float32(val&0x0001ffff) / 100000
	l := i + f
	if val&0x08000000 > 0 {
		l *= -1
	}
	return fmt.Sprintf("%v", l)
}

func toTime(b []byte) string {
	val := binary.BigEndian.Uint64(b) // millis
	sec := val / 1000
	nsec := (val - sec*1000) * 1000000
	return time.Unix(int64(sec), int64(nsec)).String()
}

func handlePacket(packet []byte) error {
	if len(packet)%aes.BlockSize != 0 {
		return errors.New("invalid packet size")
	}
	// decrypt packet
	packet = decrypt(packet)
	fmt.Printf("%v\n", packet)
	// validate packet (length already handled, == 32)
	sum := md5.Sum(packet[:16])
	for i, b := range sum {
		if packet[i+16] != b {
			fmt.Printf("mismatched byte!!\n")
		}
	}
	// save somewhere
	var lat = toAngle(packet[0:4])
	var lon = toAngle(packet[4:8])
	var t = toTime(packet[8:16])
	// just print out data for now
	fmt.Printf("lat: %v\nlon: %v\ntime: %v\n", lat, lon, t)
	return nil
}

// udp handler
func startUDP(uDied <-chan int, iDied chan<- error) {
	fmt.Println(" * starting UDP handler.")

	pc, err := net.ListenPacket("udp", udpAddr)
	if err != nil {
		fmt.Printf(" ! could not start UDP handler.\n %v \n", err)
		iDied <- err
		return
	}

	buffer := make([]byte, maxBufferSize)
	die := make(chan error)
	go func() {
		fmt.Println(" * UDP listening on " + udpAddr)
		for {
			// if len(packet) > len(buffer) then n == len(buffer)
			// so we'll have to figure out how to receive larger packets
			n, addr, err := pc.ReadFrom(buffer)
			if len(key) < 32 {
				continue
			}
			fmt.Printf(" %v \n", n)
			if err != nil {
				die <- err
				return
			}

			if n != 32 {
				die <- errors.New("byte size invalid")
				return
				// todo
			}

			fmt.Printf(" > packet:\n     bytes:%v\n     from:%s\n", buffer[:n], addr.String())

			// do stuff with packet
			handlePacket(buffer[:n])
			// ...

			// send packet into consumer channel
			// deadline := time.Now().Add(5 * time.Second)
			// err = pc.SetWriteDeadline(deadline)
			// if err != nil {
			// 	ret <- err
			// 	return
			// }
			// n, err = pc.WriteTo(buffer[:n], addr)
			// if err != nil {
			// 	ret <- err
			// 	return
			// }
			// fmt.Printf(" > sent back:\n     #bytes:%v\n     to:%s\n", n, addr.String())
		}
	}()
	go func() {
		select {
		case <-uDied:
			pc.Close()
			break
		case iDied <- <-die:
			break
		}
	}()

	fmt.Println(" * UDP handler started.")
}

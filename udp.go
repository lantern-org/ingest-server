package main

import (
	"crypto/aes"
	"crypto/md5"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"time"
)

func (s Session) decrypt(c []byte) []byte {
	// use key with AES-256
	p := make([]byte, len(c))
	numBlocks := len(c) / aes.BlockSize
	for i := 0; i < numBlocks; i++ {
		s.decr.Decrypt(p[i*aes.BlockSize:(i+1)*aes.BlockSize], c[i*aes.BlockSize:(i+1)*aes.BlockSize])
	}
	return p
}

func toAngle(b []byte) float32 {
	val := binary.BigEndian.Uint32(b)
	i := float32((val & 0x07fe0000) >> 17)
	f := float32(val&0x0001ffff) / 100000
	l := i + f
	if val&0x08000000 > 0 {
		l *= -1
	}
	// return fmt.Sprintf("%v", l)
	return l
}

func toTime(b []byte) int64 {
	val := binary.BigEndian.Uint64(b) // millis
	sec := val / 1000
	nsec := (val - sec*1000) * 1000000
	return time.Unix(int64(sec), int64(nsec)).UnixMilli()
}

func (s Session) handlePacket(packet []byte) error {
	// TODO -- this should be safe inside a go-routine
	if len(packet)%aes.BlockSize != 0 {
		// ignore packet
		return errors.New("invalid packet size")
	}
	// decrypt packet
	packet = s.decrypt(packet) // might need to be locked -- should be okay though
	fmt.Printf("%v\n", packet)
	// validate packet (length already handled, == 32)
	sum := md5.Sum(packet[:16])
	for i, b := range sum {
		if packet[i+16] != b {
			fmt.Printf("mismatched byte!!\n")
		}
	}
	// save somewhere
	var t = toTime(packet[8:16])
	if _, ok := sessions[s.port].data[t]; ok {
		// ignore packet
		return errors.New("packet time already recorded")
	}
	var lat = toAngle(packet[0:4])
	var lon = toAngle(packet[4:8])
	// just print out data for now
	fmt.Printf("lat: %v\nlon: %v\ntime: %v\n", lat, lon, t)
	sessions[s.port].data[t] = Data{
		Time: t,
		Lat:  lat,
		Lon:  lon,
	}
	return nil
}

// udp handler
func (s Session) startUDP() bool {
	fmt.Println(" * starting UDP handler.")

	pc, err := net.ListenPacket("udp", s.addr)
	if err != nil {
		fmt.Printf(" ! could not start UDP handler.\n %v \n", err)
		return false
	}

	buffer := make([]byte, maxBufferSize)
	stop := make(chan int, 1)
	go func() {
		// THE UDP SERVER CANNOT DIE
		// unless API kills it
		fmt.Println(" * UDP listening on " + s.addr)
		flag := true
		for flag {
			select {
			case <-s.die:
				pc.Close()
				flag = false // close packet listener
			case <-kill:
				kill <- 1 // pass it thru
				pc.Close()
				flag = false // close packet listener
			}
		}
		stop <- 1
		fmt.Println(" * UDP listener on " + s.addr + " ended")
	}()

	go func() {
		for {
			select {
			case <-stop:
				return
			default:
				// handle packet
				// if len(packet) > len(buffer) then n == len(buffer)
				// so we'll have to figure out how to receive larger packets
				n, _, err := pc.ReadFrom(buffer) // len,addr,err
				if err != nil {
					fmt.Printf(" ! UDP:%v error = %v\n", s.port, err)
					continue
				}
				if n != 32 {
					fmt.Printf(" ! UDP:%v error = %v\n", s.port, errors.New("byte size invalid"))
					continue
				}
				// fmt.Printf(" > packet:\n     bytes:%v\n     from:%s\n", buffer[:n], addr.String())
				go func() { s.handlePacket(buffer[:n]) }()
			}
		}
	}()

	fmt.Println(" * UDP handler started.")
	return true
}

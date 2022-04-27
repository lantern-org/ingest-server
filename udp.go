package main

import (
	"crypto/aes"
	"crypto/md5"
	"encoding/binary"
	"errors"
	"log"
	"math"
	"net"
	"time"
)

func (s *Session) decrypt(c []byte) []byte {
	// use key with AES-256
	p := make([]byte, len(c))
	numBlocks := len(c) / aes.BlockSize
	for i := 0; i < numBlocks; i++ {
		s.decr.Decrypt(p[i*aes.BlockSize:(i+1)*aes.BlockSize], c[i*aes.BlockSize:(i+1)*aes.BlockSize])
	}
	return p
}

func bytesToFloat32(b []byte) float32 {
	val := binary.BigEndian.Uint32(b)
	// i := float32((val & 0x07fe0000) >> 17)
	// f := float32(val&0x0001ffff) / 100000
	// l := i + f
	// if val&0x08000000 > 0 {
	// 	l *= -1
	// }
	// // return fmt.Sprintf("%v", l)
	// return l
	return math.Float32frombits(val)
}

func bytesToTime(b []byte) int64 {
	// require transmission in BigEndian
	val := binary.BigEndian.Uint64(b) // millis
	sec := val / 1000
	nsec := (val - sec*1000) * 1000000
	return time.Unix(int64(sec), int64(nsec)).UnixMilli()
}

func bytesToInt32(b []byte) uint32 {
	return binary.BigEndian.Uint32(b)
}

func (s *Session) handlePacket(packet []byte) error {
	// TODO -- this should be safe inside a go-routine
	if len(packet)%aes.BlockSize != 0 {
		// ignore packet
		return errors.New("invalid packet size")
	}
	// decrypt packet
	// log.Printf(" > %v %v\n", s.addr, packet)
	packet = s.decrypt(packet) // might need to be locked -- should be okay though
	// log.Printf(" > %v %v\n", s.addr, packet)
	// validate packet (length already handled, == PACKET_LENGTH)
	sum := md5.Sum(packet[:PACKET_LENGTH-md5.Size])
	for i, b := range sum {
		if packet[i+PACKET_LENGTH-md5.Size] != b {
			// ignore packet
			return errors.New("mismatched byte")
		}
	}
	// save somewhere
	// can switch on the packet version here if later iterations require
	s.Data = append(s.Data, Data{
		Version:  bytesToInt32(packet[0:4]),
		Index:    bytesToInt32(packet[4:8]),
		Time:     bytesToTime(packet[8:16]),
		Lat:      bytesToFloat32(packet[16:20]),
		Lon:      bytesToFloat32(packet[20:24]),
		Acc:      bytesToFloat32(packet[24:28]),
		Internet: packet[28],
	})
	// log.Printf(" > handlepacket: %v\n", s)
	return nil
}

// udp handler
func (s *Session) startUDP() bool {
	log.Println(" * starting UDP handler.")

	pc, err := net.ListenPacket(udpType, s.addr)
	if err != nil {
		log.Printf(" ! could not start UDP handler.\n %v \n", err)
		return false
	}

	buffer := make([]byte, MAX_BUFFER_SIZE)
	stop := make(chan int, 1)
	go func() {
		// todo -- if udp server dies unexpectedly
		// re-make it
		// (ie, have a go-routine that routinely checks health of udp handlers)
		log.Println(" * UDP listening on " + s.addr)
		flag := true
		for flag {
			select {
			case <-s.die:
				flag = false
			case <-kill:
				kill <- 1 // pass it thru
				flag = false
			}
		}
		pc.Close() // close packet listener

		stop <- 1
		log.Println(" * UDP listener on " + s.addr + " ended")
	}()

	go func() {
		for {
			select {
			case <-stop:
				s.udpLoopEnd <- 1
				return
			default:
				// handle packet
				// if len(packet) > len(buffer) then n == len(buffer)
				// so we'll have to figure out how to receive larger packets
				n, _, err := pc.ReadFrom(buffer) // len,addr,err
				if err != nil {
					// log.Printf(" ! UDP: %v error = %v\n", s.addr, err)
					// not necessarily a packet error -- usually just a "read from closed connection" error
					continue // we could potentially just break here?
				}
				if n != PACKET_LENGTH {
					// log.Printf(" ! UDP: %v error = %v\n", s.addr, errors.New("byte size invalid"))
					s.PacErrs += 1
					continue
				}
				// fmt.Printf(" > packet:\n     bytes:%v\n     from:%s\n", buffer[:n], addr.String())
				if err := s.handlePacket(buffer[:n]); err != nil {
					s.PacErrs += 1
				}
				// could put buffer data into channel and have worker go thru channel?
				//  but that's just shifting the problem to someone else -- eventually the work will pile up anyway
			}
		}
	}()

	log.Println(" * UDP handler started.")
	return true
}

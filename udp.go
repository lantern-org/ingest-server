package main

import (
	"crypto/aes"
	"crypto/md5"
	"encoding/binary"
	"encoding/json"
	"errors"
	"log"
	"math"
	"net"
	"os"
	"sort"
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

func bytesToFloat32BE(b []byte) float32 {
	return math.Float32frombits(binary.BigEndian.Uint32(b))
}
func bytesToFloat32LE(b []byte) float32 {
	return math.Float32frombits(binary.LittleEndian.Uint32(b))
}

func uint64ToTime(v uint64) int64 {
	sec := v / 1000
	nsec := (v - sec*1000) * 1000000
	return time.Unix(int64(sec), int64(nsec)).UnixMilli()
}
func bytesToTimeBE(b []byte) int64 {
	return uint64ToTime(binary.BigEndian.Uint64(b)) // millis
}
func bytesToTimeLE(b []byte) int64 {
	return uint64ToTime(binary.LittleEndian.Uint64(b)) // millis
}

func bytesToUInt32BE(b []byte) uint32 {
	return binary.BigEndian.Uint32(b)
}
func bytesToUInt32LE(b []byte) uint32 {
	return binary.LittleEndian.Uint32(b)
}

func bytesToUInt16BE(b []byte) uint16 {
	return binary.BigEndian.Uint16(b)
}
func bytesToUInt16LE(b []byte) uint16 {
	return binary.LittleEndian.Uint16(b)
}

func (s *Session) handlePacket(packet []byte) error {
	// TODO -- this should be safe inside a go-routine
	if len(packet) != PACKET_LENGTH {
		return errors.New("received-bytes length invalid")
	}
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
	var d Data
	if packet[0]&0b10000000 == 0 { // endianness =0 Big, >0 Little
		// big-endian
		d = Data{
			Version:   bytesToUInt16BE(packet[2:4]),
			Index:     bytesToUInt32BE(packet[4:8]),
			Time:      bytesToTimeBE(packet[8:16]),
			Lat:       bytesToFloat32BE(packet[16:20]),
			Lon:       bytesToFloat32BE(packet[20:24]),
			Acc:       bytesToFloat32BE(packet[24:28]),
			Internet:  packet[0],
			Processed: time.Now().Unix(),
		}
	} else {
		// little-endian
		d = Data{
			Version:   bytesToUInt16LE(packet[2:4]),
			Index:     bytesToUInt32LE(packet[4:8]),
			Time:      bytesToTimeLE(packet[8:16]),
			Lat:       bytesToFloat32LE(packet[16:20]),
			Lon:       bytesToFloat32LE(packet[20:24]),
			Acc:       bytesToFloat32LE(packet[24:28]),
			Internet:  packet[0] ^ 0b10000000,
			Processed: time.Now().Unix(),
		}
	}
	s.dataLock.Lock()
	s.Data = append(s.Data, d)
	s.dataLock.Unlock()
	if <-s.paused {
		log.Printf(" * UDP listener (%v) unpaused\n", s.addr)
	}
	s.paused <- false
	s.pause.Reset(pauseDuration)
	// log.Printf(" > handlepacket: %v\n", s)
	return nil
}

// udp handler
func (s *Session) startUDP() bool {
	log.Printf(" * UDP listener (%v) starting\n", s.addr)

	pc, err := net.ListenPacket(udpType, s.addr)
	if err != nil {
		log.Printf(" ! could not start UDP listener (%v) with error %v \n", s.addr, err)
		return false
	}

	buffer := make([]byte, MAX_BUFFER_SIZE)
	stop := make(chan int, 1)
	s.pause = time.NewTicker(pauseDuration)
	s.paused <- false
	go func() {
		// todo -- if udp server dies unexpectedly
		// re-make it
		// (ie, have a go-routine that routinely checks health of udp handlers)
		flag := true
		for flag {
			select {
			case <-s.die:
				s.die <- 1 // pass it thru for api in case paused -> die
				flag = false
			case <-kill:
				kill <- 1 // pass it thru
				flag = false
			case <-s.pause.C:
				if <-s.paused {
					s.paused <- true
					// kill self
					s.die <- 1
					log.Printf(" * UDP listener (%v) paused for %v -- killing self\n", s.addr, stopDuration)
					flag = false // breaking the loop kills self
					// close(stop) ends udp loop and saves data
				} else {
					s.paused <- true
					// check back in an hour
					log.Printf(" * UDP listener (%v) inactive after %v -- setting pause state\n", s.addr, pauseDuration)
					s.pause.Reset(stopDuration)
				}
			}
		}
		pc.Close() // close packet listener
		s.pause.Stop()
		close(s.paused)
		close(stop) // stop <- 1
		s.stopUDP()
		log.Println(" * UDP listener on " + s.addr + " ended")
	}()

	go func() {
		log.Println(" * UDP listening on " + s.addr)
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
					// log.Printf(" ! UDP (%v) error = %v\n", s.addr, err)
					// not necessarily a packet error -- usually just a "read from closed connection" error
					continue // we could potentially just break here?
				}
				// fmt.Printf(" > packet:\n     bytes:%v\n     from:%s\n", buffer[:n], addr.String())
				if err := s.handlePacket(buffer[:n]); err != nil {
					// log.Printf(" ! UDP (%v) error = %v\n", s.addr, err)
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

func (s *Session) stopUDP() {
	log.Println(" * UDP listener on " + s.addr + " saving data")
	if udpType == "unixgram" {
		err := os.Remove(s.addr)
		if err != nil {
			// log it, but it's ok i guess
			log.Printf("couldn't remove socket file %s\n", s.addr)
		} else {
			udpPorts <- s.Port // free back the port (shouldn't block)
		}
	} else { // with ip addresses we don't have to remove a file
		udpPorts <- s.Port // free back the port (shouldn't block)
	}
	// export to file
	s.EndTime = time.Now()
	// s.StartTime.Format("2006-01-02_15-04-05")
	f, err := os.Create("data/" + s.token.String() + ".dat")
	if err != nil {
		log.Printf(" ! err: %v\n", err)
		return
	}
	sort.Slice(s.Data, func(i, j int) bool { return s.Data[i].Time < s.Data[j].Time }) // slow, but it's ok
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
	sessionsLock.Lock()
	delete(sessions, s.Port)
	sessionsLock.Unlock()
	codesLock.Lock()
	delete(codes, s.code)
	codesLock.Unlock()
}

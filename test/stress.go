package main

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net"
	"net/http"
	"sync"
	"time"
)

// TODO command-line args
const num_conns = 5
const num_packets = 32 // as this goes up, the time to write the server log file increases
// https://electronics.stackexchange.com/q/287794 // interesting discussion, max gps sampling rate is like 10-25 samp/sec
// const packets_per_second = 20000

type packet struct {
	v             uint32
	i             uint32
	t             int64
	lat, lon, acc float32
	sig           byte
	bytes         []byte // encrypted
}

func randPacket(i uint32, encr cipher.Block) packet {
	lat := rand.Float32()*180 - 90  // [0,1)*180-90 -> [-90,90)
	lon := rand.Float32()*360 - 180 // [0,1)*360-180 -> [-180,180)
	acc := rand.Float32()*10 - 5    // [0,1)*10-5 -> [-5,5)
	// time.Now().UnixMilli()
	t := int64(i)
	var buf bytes.Buffer
	var end binary.ByteOrder = binary.BigEndian
	if end == binary.BigEndian {
		binary.Write(&buf, binary.BigEndian, byte(4)) // 0000bbbb
	} else {
		binary.Write(&buf, binary.LittleEndian, byte(128+4)) // b000bbbb
	}
	binary.Write(&buf, end, byte(0))
	binary.Write(&buf, end, uint16(1))
	binary.Write(&buf, end, i)
	binary.Write(&buf, end, t)
	binary.Write(&buf, end, lat)
	binary.Write(&buf, end, lon)
	binary.Write(&buf, end, acc)
	binary.Write(&buf, end, byte(0)) // can't use uint64(0)
	binary.Write(&buf, end, byte(0))
	binary.Write(&buf, end, byte(0))
	binary.Write(&buf, end, byte(0))
	sum := md5.Sum(buf.Bytes())
	buf.Write(sum[:])
	p := buf.Bytes()
	numBlocks := len(p) / aes.BlockSize
	for j := 0; j < numBlocks; j++ { // re-using i
		encr.Encrypt(p[j*aes.BlockSize:(j+1)*aes.BlockSize], p[j*aes.BlockSize:(j+1)*aes.BlockSize])
	}
	return packet{
		1, i, t,
		lat, lon, acc,
		4,
		p,
	}
}

type stat struct {
	me               int
	recv             int
	pac_drop_percent float64
	succ             bool
	elapsed          time.Duration
	pac_per_sec      float64
}

func (s stat) String() string {
	return fmt.Sprintf("< thread:%d  succ:%v  recv:%d  time:%v  drop:%6.2f%%  pps:~%9.2f >", s.me, s.succ, s.recv, s.elapsed, s.pac_drop_percent, s.pac_per_sec)
}

var stats []stat

var statsLock = sync.RWMutex{}

func execute(me int) {
	fmt.Printf("thread %d starting\n", me)
	key := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	postBody, err := json.Marshal(map[string]string{
		"username": "test",
		"password": "test",
		"key":      key,
	})
	if err != nil {
		panic(err)
	}
	reqBody := bytes.NewBuffer(postBody)
	resp, err := http.Post("http://localhost:1025/session/start", "application/json", reqBody)
	if err != nil {
		panic(err)
	}
	var resBody struct {
		Port  int
		Token string
		Code  string
	}
	err = json.NewDecoder(resp.Body).Decode(&resBody)
	if err != nil {
		panic(err)
	}
	fmt.Printf("thread %d session start: %v \n", me, resBody)
	keyBytes, err := hex.DecodeString(key)
	if err != nil {
		panic(err)
	}
	encr, err := aes.NewCipher(keyBytes)
	if err != nil {
		panic(err)
	}
	trace := []packet{}
	for i := uint32(0); i < num_packets; i++ {
		trace = append(trace, randPacket(i, encr))
	}
	conn, err := net.Dial("udp", fmt.Sprintf(":%d", resBody.Port))
	if err != nil {
		panic(err)
	}
	defer conn.Close()
	// start timer
	/*
		N = num packets = num iterations
		T = totalTime sec = temp
		R = num packets / second
		C = time to send one packet sec
		w = wait time per packet (iteration) sec unknown

		N / T = R
		T = N * (C + w)

		N / (N * (C + w)) = R
		N = R * (N * (C + w))
		N = RNC + RNw
		N - RNC = RNw
		(N - RNC) / (RN) = w
		w = 1/R - C

		1e6 µs = 1 sec
		1e9 ns = 1 sec

		i'm dumb

		1/(C+w) = R
		1 = R(C+w)
		1 = RC + Rw
		1 - RC = Rw
		w = 1/R - C
	*/
	// C := 3500 * time.Nanosecond
	// wait := 1e9/packets_per_second - C
	// fmt.Println(wait) // -> 46.5µs
	// wait := 46500 * time.Nanosecond
	//
	// wait := num_conns * 1000 * time.Nanosecond // 150ns if one worker..?
	wait := 100 * time.Millisecond
	start := time.Now()
	// b := time.Duration(0)
	for i := 0; i < num_packets; i++ {
		// a := time.Now()
		conn.Write(trace[i].bytes) // ~ 3.5µs ~ 3500ns
		// b += time.Since(a)
		// sleep?
		// time.Sleep(time.Microsecond)
		time.Sleep(wait) // overhead of ~ idk?
	}
	// fmt.Println(b / num_packets)
	elapsed := time.Since(start)
	// end timer
	time.Sleep(time.Millisecond)
	postBody, err = json.Marshal(struct {
		Port  int
		Token string
	}{resBody.Port, resBody.Token})
	if err != nil {
		panic(err)
	}
	reqBody = bytes.NewBuffer(postBody)
	resp, err = http.Post("http://localhost:1025/session/stop", "application/json", reqBody)
	if err != nil {
		panic(err)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}
	fmt.Printf("thread %d session stop: %v \n", me, string(body))
	var data struct {
		Port  int
		Start string
		End   string
		Data  []struct {
			Version   uint32
			Index     uint32
			Time      int64
			Latitude  float32
			Longitude float32
			Accuracy  float32
			Internet  byte
		}
	}
	retries := -1
	for {
		retries += 1
		if retries > 10 {
			panic("failed.")
		}
		time.Sleep(3 * time.Second) // wait for server to write file
		resp, err = http.Get("http://localhost:1025/log/" + resBody.Token)
		if err != nil {
			continue
		}
		if resp.StatusCode != 200 {
			continue
		}
		// io.Copy(os.Stdout, resp.Body)
		err = json.NewDecoder(resp.Body).Decode(&data)
		if err != nil {
			continue
		}
		break
	}
	success := num_packets == len(data.Data)
	for i := 0; success && i < len(data.Data); i++ {
		success = trace[i].t == data.Data[i].Time &&
			math.Abs(float64(trace[i].lat-data.Data[i].Latitude)) < 0.0001 &&
			math.Abs(float64(trace[i].lon-data.Data[i].Longitude)) < 0.0001
	}
	// fmt.Printf("thread %d stats [\n\tsent:%d recv:%d packet_loss:%.2f%% matched:%v\n\tsend_time: %v\n\tserver handled ~%.2f packets/sec\n]\n", me, num_packets, len(data.Data), float64(num_packets-len(data.Data))/float64(num_packets)*100, success, elapsed, float64(len(data.Data))/elapsed.Seconds())
	statsLock.Lock()
	stats = append(stats, stat{
		me,
		len(data.Data),
		float64(num_packets-len(data.Data)) / float64(num_packets) * 100,
		success,
		elapsed,
		float64(len(data.Data)) / elapsed.Seconds(),
	})
	statsLock.Unlock()
}

func main() {
	var wg sync.WaitGroup
	for i := 0; i < num_conns; i++ {
		wg.Add(1)
		i := i
		go func() {
			defer wg.Done()
			execute(i)
		}()
	}
	wg.Wait()
	// packet []byte
	var totalRecv int
	var totalDuration time.Duration
	for _, s := range stats {
		totalRecv += s.recv
		totalDuration += s.elapsed
		fmt.Println(s)
	}
	fmt.Printf(
		"\noverall stats\n=============\nsent:%d packets\nrecv:[avg:%d] packets\ntime:[avg:%v]\n%6.2f%% packet loss\n~%.2f packets/sec\n",
		num_packets,
		totalRecv/num_conns,
		time.Duration(float64(totalDuration)/float64(num_conns)),
		float64(num_conns*num_packets-totalRecv)/float64(num_conns*num_packets)*100,
		float64(totalRecv)/totalDuration.Seconds(),
	)
	// fmt.Printf("overall stats [\n\tsent:%d recv:%d packet_loss:%.2f%% matched:%v\n\tsend_time: %v\n\tserver handled ~%.2f packets/sec\n]\n", me, num_packets, len(data.Data), float64(num_packets-len(data.Data))/float64(num_packets)*100, success, elapsed, float64(len(data.Data))/elapsed.Seconds())
}

// from what i'm gathering, the server can handle like
// * 20000 packets / second / connection
// * num_packets < pre-alloc'd packets: max connections < 16
// * num_packets > pre-alloc'd packets: max connections <  8
// roughly
// max_volume = 8 * 20000 = 160000 pps
// can the server handle your use-case?
//  -> num_conns * pps_per_conn < max_volume
// eg: 1000 connections at 10 packets per second = 10000 vol < 160000 vol
// so the server _should_ be able to handle this
// also consider all computations (server and tester) were executed on one 8-core machine

// the above was true for packet protocol V0
// for V1, we'd expect marginally less performance given the packet increased by 16 bytes

// Copyright 2022 Alim Zanibekov
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"flag"
	"github.com/alim-zanibekov/teltonika"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"
)

var decodeConfig = &teltonika.DecodeConfig{IoElementsAlloc: teltonika.OnReadBuffer}

type OnPacket func(imei string, pkt *teltonika.Packet)

type UDPServer struct {
	address     string
	logger      *Logger
	onPacket    OnPacket
	workerCount int
}

//goland:noinspection GoUnusedExportedFunction
func NewUDPServer(address string, workerCount int) *UDPServer {
	return &UDPServer{address: address, workerCount: workerCount, logger: &Logger{log.Default(), log.Default()}}
}

func NewUDPServerLogger(address string, workerCount int, logger *Logger) *UDPServer {
	return &UDPServer{address: address, workerCount: workerCount, logger: logger}
}

type Logger struct {
	Info  *log.Logger
	Error *log.Logger
}

func (r *UDPServer) OnPacket(handler OnPacket) {
	r.onPacket = handler
}

func (r *UDPServer) Run() {
	logger := r.logger
	hostStr, portStr, err := net.SplitHostPort(r.address)
	if err != nil {
		logger.Error.Println(err.Error())
		os.Exit(1)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		logger.Error.Println(err.Error())
		os.Exit(1)
	}
	ip := net.ParseIP(hostStr)
	udpConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: ip, Port: port, Zone: ""})
	if err != nil {
		logger.Error.Println(err.Error())
		os.Exit(1)
	}

	defer func(udpConn *net.UDPConn) {
		err := udpConn.Close()
		if err != nil {
			logger.Error.Println(err.Error())
		}
	}(udpConn)

	logger.Info.Printf("udp listening at %s", hostStr)

	type job struct {
		buffer []byte
		addr   *net.UDPAddr
	}
	jobs := make(chan job, r.workerCount)

	worker := func(conn *net.UDPConn) {
		for j := range jobs {
			r.handleConnection(conn, j.addr, j.buffer)
		}
	}

	for i := 0; i < r.workerCount; i++ {
		go worker(udpConn)
	}

	for {
		buf := make([]byte, 1300)
		n, addr, err := udpConn.ReadFromUDP(buf)
		if err != nil {
			logger.Error.Println(err.Error())
			os.Exit(1)
		}

		packet := make([]byte, n)
		copy(packet, buf[:n])

		jobs <- job{packet, addr}
	}
}

func (r *UDPServer) handleConnection(conn *net.UDPConn, addr *net.UDPAddr, packet []byte) {
	logger := r.logger
	_, res, err := teltonika.DecodeUDPFromSlice(packet, decodeConfig)
	if err != nil {
		logger.Error.Printf("[%s]: %v", addr.String(), err)
		return
	}

	if res.Response != nil {
		_, err = conn.WriteToUDP(res.Response, addr)
		if err != nil {
			logger.Error.Printf("[%s]: %v", addr.String(), err)
			return
		}
	}
	logger.Info.Printf("[%s]: message: %s", addr.String(), hex.EncodeToString(packet))
	jsonData, err := json.Marshal(res.Packet)
	if err != nil {
		logger.Error.Printf("[%s]: %v", addr.String(), err)
	}
	logger.Info.Printf("[%s]: decoded: %s", addr.String(), string(jsonData))

	if r.onPacket != nil {
		r.onPacket(res.Imei, res.Packet)
	}
}

func buildJsonPacket(imei string, pkt *teltonika.Packet) []byte {
	if pkt.Data == nil {
		return nil
	}
	gpsFrames := make([]interface{}, 0)
	for _, frame := range pkt.Data {
		gpsFrames = append(gpsFrames, map[string]interface{}{
			"timestamp": int64(frame.TimestampMs / 1000.0),
			"lat":       frame.Lat,
			"lon":       frame.Lng,
		})
	}
	if len(gpsFrames) == 0 {
		return nil
	}
	values := map[string]interface{}{
		"deveui": imei,
		"time":   time.Now().String(),
		"frames": map[string]interface{}{
			"gps": gpsFrames,
		},
	}
	jsonValue, _ := json.Marshal(values)
	return jsonValue
}

func main() {
	var address string
	var outHook string
	flag.StringVar(&address, "address", "0.0.0.0:8080", "server address")
	flag.StringVar(&outHook, "hook", "http://localhost:5000/api/v1/metric", "output hook")
	flag.Parse()

	logger := &Logger{
		Info:  log.New(os.Stdout, "INFO: ", log.Ldate|log.Ltime),
		Error: log.New(os.Stdout, "ERROR: ", log.Ldate|log.Ltime|log.Lshortfile),
	}

	type WPacket struct {
		Imei string
		Pkt  *teltonika.Packet
	}

	out := make(chan *WPacket, 1000)

	go func() {
		for msg := range out {
			jsonValue := buildJsonPacket(msg.Imei, msg.Pkt)
			if jsonValue == nil {
				continue
			}

			res, err := http.Post(outHook, "application/json", bytes.NewBuffer(jsonValue))
			if err != nil {
				logger.Error.Printf("http post error (%v)", err)
			} else {
				logger.Info.Printf("packet sent to output hook, status: %s", res.Status)
			}
		}
	}()

	server := NewUDPServerLogger(address, 20, logger)

	server.OnPacket(func(imei string, pkt *teltonika.Packet) {
		if pkt.Data != nil {
			out <- &WPacket{imei, pkt}
		}
	})

	server.Run()
}

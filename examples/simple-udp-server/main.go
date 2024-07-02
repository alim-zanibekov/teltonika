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
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/alim-zanibekov/teltonika"
	"github.com/alim-zanibekov/teltonika/ioelements"
)

var decodeConfig = &teltonika.DecodeConfig{IoElementsAlloc: teltonika.OnReadBuffer}

type Logger struct {
	Info  *log.Logger
	Error *log.Logger
}

type UDPServer struct {
	address     string
	logger      *Logger
	OnPacket    func(imei string, pkt *teltonika.Packet)
	workerCount int
}

//goland:noinspection GoUnusedExportedFunction
func NewUDPServer(address string, workerCount int) *UDPServer {
	return &UDPServer{address: address, workerCount: workerCount, logger: &Logger{log.Default(), log.Default()}}
}

func NewUDPServerLogger(address string, workerCount int, logger *Logger) *UDPServer {
	return &UDPServer{address: address, workerCount: workerCount, logger: logger}
}

func (r *UDPServer) Run() error {
	logger := r.logger
	hostStr, portStr, err := net.SplitHostPort(r.address)
	if err != nil {
		return fmt.Errorf("split host and port error (%v)", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return fmt.Errorf("port parse error (%v)", err)
	}
	ip := net.ParseIP(hostStr)
	udpConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: ip, Port: port, Zone: ""})
	if err != nil {
		return fmt.Errorf("listen udp error (%v)", err)
	}

	defer func() {
		_ = udpConn.Close()
	}()

	logger.Info.Printf("udp listening at %s", hostStr)

	type job struct {
		buffer []byte
		addr   *net.UDPAddr
	}

	jobs := make(chan job, r.workerCount)
	defer close(jobs)

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
			return fmt.Errorf("udp read packet error (%v)", err)
		}

		packet := make([]byte, n)
		copy(packet, buf[:n])

		jobs <- job{packet, addr}
	}
}

func (r *UDPServer) handleConnection(conn *net.UDPConn, addr *net.UDPAddr, packet []byte) {
	logger := r.logger
	client := addr.String()

	_, res, err := teltonika.DecodeUDPFromSlice(packet, decodeConfig)
	if err != nil {
		logger.Error.Printf("[%s]: packet decode error (%v)", client, err)
		return
	}

	if res.Response != nil {
		if _, err = conn.WriteToUDP(res.Response, addr); err != nil {
			logger.Error.Printf("[%s]: error writing response (%v)", client, err)
			return
		}
	}

	logger.Info.Printf("[%s]: message: %s", client, hex.EncodeToString(packet))
	jsonData, err := json.Marshal(res.Packet)
	if err != nil {
		logger.Error.Printf("[%s]: marshaling error (%v)", client, err)
	} else {
		logger.Info.Printf("[%s]: decoded: %s", client, string(jsonData))
		for i, data := range res.Packet.Data {
			elements := make([]string, len(data.Elements))
			for j, element := range data.Elements {
				it, err := ioelements.DefaultParser().Parse("*", element.Id, element.Value)
				if err != nil {
					break
				}
				elements[j] = it.String()
			}
			logger.Info.Printf("[%s]: io elements [frame #%d]: %s", client, i, strings.Join(elements, ", "))
		}
	}

	if r.OnPacket != nil {
		r.OnPacket(res.Imei, res.Packet)
	}
}

func main() {
	var address string
	var outHook string
	flag.StringVar(&address, "address", "0.0.0.0:8080", "server address")
	flag.StringVar(&outHook, "hook", "", "output hook\nfor example: http://localhost:8080/push")
	flag.Parse()

	logger := &Logger{
		Info:  log.New(os.Stdout, "INFO: ", log.Ldate|log.Ltime),
		Error: log.New(os.Stdout, "ERROR: ", log.Ldate|log.Ltime|log.Lshortfile),
	}

	server := NewUDPServerLogger(address, 20, logger)

	server.OnPacket = func(imei string, pkt *teltonika.Packet) {
		if pkt.Data != nil && outHook != "" {
			go hookSend(outHook, imei, pkt, logger)
		}
	}

	panic(server.Run())
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

func hookSend(outHook string, imei string, pkt *teltonika.Packet, logger *Logger) {
	jsonValue := buildJsonPacket(imei, pkt)
	if jsonValue == nil {
		return
	}
	res, err := http.Post(outHook, "application/json", bytes.NewBuffer(jsonValue))
	if err != nil {
		logger.Error.Printf("http post error (%v)", err)
	} else {
		logger.Info.Printf("packet sent to output hook, status: %s", res.Status)
	}
}

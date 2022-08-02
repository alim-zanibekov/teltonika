// Copyright 2022 Alim Zanibekov
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/alim-zanibekov/teltonika"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

var decodeConfig = &teltonika.DecodeConfig{IoElementsAlloc: teltonika.OnReadBuffer}

type OnPacket func(imei string, pkt *teltonika.Packet)
type OnClose func(imei string)
type OnConnect func(imei string)

type TCPServer struct {
	address   string
	clients   sync.Map
	logger    *Logger
	onClose   OnClose
	onPacket  OnPacket
	onConnect OnConnect
}

//goland:noinspection GoUnusedExportedFunction
func NewTCPServer(address string) *TCPServer {
	return &TCPServer{address: address, logger: &Logger{log.Default(), log.Default()}}
}

func NewTCPServerLogger(address string, logger *Logger) *TCPServer {
	return &TCPServer{address: address, logger: logger}
}

type TCPClient struct {
	conn net.Conn
	imei string
}

type Logger struct {
	Info  *log.Logger
	Error *log.Logger
}

func (r *TCPServer) OnConnect(handler OnConnect) {
	r.onConnect = handler
}

func (r *TCPServer) OnClose(handler OnClose) {
	r.onClose = handler
}

func (r *TCPServer) OnPacket(handler OnPacket) {
	r.onPacket = handler
}

func (r *TCPServer) Run() {
	logger := r.logger
	listener, err := net.Listen("tcp", r.address)
	if err != nil {
		logger.Error.Println(err.Error())
		os.Exit(1)
	}

	defer func(listener net.Listener) {
		err := listener.Close()
		if err != nil {
			logger.Error.Println(err.Error())
		}
	}(listener)

	logger.Info.Println("tcp server listening at " + r.address)

	for {
		conn, err := listener.Accept()
		if err != nil {
			logger.Error.Println(err.Error())
			os.Exit(1)
		}
		go r.handleConnection(conn)
	}
}

func (r *TCPServer) SendPacket(imei string, packet *teltonika.Packet) error {
	clientRaw, ok := r.clients.Load(imei)
	if !ok {
		return fmt.Errorf("client with imei '%s' not found", imei)
	}
	client := clientRaw.(*TCPClient)

	buf, err := teltonika.EncodePacket(packet)
	if err != nil {
		return err
	}

	if _, err = client.conn.Write(buf); err != nil {
		return err
	}

	return nil
}

func (r *TCPServer) ListClients() []*TCPClient {
	clients := make([]*TCPClient, 0, 10)
	r.clients.Range(func(key, value any) bool {
		clients = append(clients, value.(*TCPClient))
		return true
	})
	return clients
}

func (r *TCPServer) handleConnection(conn net.Conn) {
	logger := r.logger
	client := &TCPClient{conn, ""}
	imei := ""

	addr := conn.RemoteAddr().String()

	defer func(conn net.Conn) {
		if r.onClose != nil && imei != "" {
			r.onClose(imei)
		}
		r.clients.Delete(imei)
		err := conn.Close()
		if err != nil {
			logger.Error.Printf("[%s]: %v", addr, err)
		}
	}(conn)

	logger.Info.Printf("[%s]: connected", addr)

	buf := make([]byte, 100)
	size, err := conn.Read(buf) // Read imei
	if err != nil {
		logger.Error.Printf("[%s]: %v", addr, err)
		return
	}
	if size < 2 {
		logger.Error.Printf("[%s]: invalid first message")
		return
	}
	imeiLen := int(binary.BigEndian.Uint16(buf[:2]))
	buf = buf[2:]

	if len(buf) < imeiLen {
		logger.Error.Printf("[%s]: invalid imei size")
		return
	}

	imei = strings.TrimSpace(string(buf[:imeiLen]))
	client.imei = imei

	if r.onConnect != nil {
		r.onConnect(imei)
	}

	r.clients.Store(imei, client)

	logger.Info.Printf("[%s]: imei - %s", addr, client.imei)

	_, err = conn.Write([]byte{1}) // ack
	if err != nil {
		logger.Error.Printf("[%s]: %v", client.imei, err)
		return
	}

	readBuffer := make([]byte, 1300)
	for {
		read, res, err := teltonika.DecodeTCPFromReaderBuf(conn, readBuffer, decodeConfig)
		if err != nil {
			logger.Error.Printf("[%s]: %v", imei, err)
			return
		}

		if res.Response != nil {
			_, err = conn.Write(res.Response)
			if err != nil {
				logger.Error.Printf("[%s]: %v", imei, err)
				return
			}
		}

		logger.Info.Printf("[%s]: message: %s", imei, hex.EncodeToString(readBuffer[:read]))
		jsonData, err := json.Marshal(res.Packet)
		if err != nil {
			logger.Error.Printf("[%s]: %v", imei, err)
		}
		logger.Info.Printf("[%s]: decoded: %s", imei, string(jsonData))

		if r.onPacket != nil {
			r.onPacket(imei, res.Packet)
		}
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
	var httpAddress string
	var address string
	var outHook string
	flag.StringVar(&address, "address", "0.0.0.0:8080", "server address")
	flag.StringVar(&httpAddress, "http", "0.0.0.0:8081", "http server address")
	flag.StringVar(&outHook, "hook", "http://localhost:5000/api/v1/metric", "output hook")
	flag.Parse()

	logger := &Logger{
		Info:  log.New(os.Stdout, "INFO: ", log.Ldate|log.Ltime),
		Error: log.New(os.Stdout, "ERROR: ", log.Ldate|log.Ltime|log.Lshortfile),
	}

	handler := http.NewServeMux()

	server := NewTCPServerLogger(address, logger)

	handler.HandleFunc("/cmd", func(w http.ResponseWriter, r *http.Request) {
		params := r.URL.Query()
		imei := params.Get("imei")
		cmd := params.Get("cmd")

		packet := &teltonika.Packet{
			CodecID:  teltonika.Codec12,
			Data:     nil,
			Messages: []teltonika.Message{{Type: teltonika.TypeCommand, Text: strings.TrimSpace(cmd)}},
		}

		err := server.SendPacket(imei, packet)
		if err != nil {
			logger.Error.Printf("send packet error (%v)", err)
		} else {
			logger.Info.Printf("command %s sent to %s", cmd, imei)
		}
	})

	handler.HandleFunc("/list-clients", func(w http.ResponseWriter, r *http.Request) {
		for _, client := range server.ListClients() {
			_, err := w.Write([]byte(client.conn.RemoteAddr().String() + " - " + client.imei + "\n"))
			if err != nil {
				return
			}
		}
		w.WriteHeader(200)
	})

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

	server.OnPacket(func(imei string, pkt *teltonika.Packet) {
		if pkt.Data != nil {
			out <- &WPacket{imei, pkt}
		}
	})

	go server.Run()
	logger.Info.Println("http server listening at " + httpAddress)
	err := http.ListenAndServe(httpAddress, handler)
	if err != nil {
		logger.Error.Printf("http listen error (%v)", err)
	}
}

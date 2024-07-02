// Copyright 2022 Alim Zanibekov
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/alim-zanibekov/teltonika"
	"github.com/alim-zanibekov/teltonika/ioelements"
)

var decodeConfig = &teltonika.DecodeConfig{IoElementsAlloc: teltonika.OnReadBuffer}

type Logger struct {
	Info  *log.Logger
	Error *log.Logger
}

type TrackersHub interface {
	SendPacket(imei string, packet *teltonika.Packet) error
	ListClients() []*Client
}

type TCPServer struct {
	address      string
	clients      *sync.Map
	logger       *Logger
	readTimeout  time.Duration
	writeTimeout time.Duration
	sLock        sync.RWMutex
	OnPacket     func(imei string, pkt *teltonika.Packet)
	OnClose      func(imei string)
	OnConnect    func(imei string)
}

type TCPClient struct {
	conn        net.Conn
	connectedAt time.Time
	imei        string
}

type Client struct {
	Imei        string    `json:"imei"`
	Addr        string    `json:"addr"`
	ConnectedAt time.Time `json:"connectedAt"`
}

//goland:noinspection GoUnusedExportedFunction
func NewTCPServer(address string) *TCPServer {
	return &TCPServer{
		address: address, logger: &Logger{log.Default(), log.Default()}, clients: &sync.Map{},
		readTimeout: time.Minute * 5, writeTimeout: time.Minute * 5,
	}
}

func NewTCPServerLogger(address string, logger *Logger) *TCPServer {
	return &TCPServer{
		address: address, logger: logger, clients: &sync.Map{},
		readTimeout: time.Minute * 5, writeTimeout: time.Minute * 5,
	}
}

func (r *TCPServer) Run() error {
	logger := r.logger

	addr, err := net.ResolveTCPAddr("tcp", r.address)
	if err != nil {
		return fmt.Errorf("tcp address resolve error (%v)", err)
	}

	listener, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return fmt.Errorf("tcp listener create error (%v)", err)
	}

	defer func() {
		_ = listener.Close()
	}()

	logger.Info.Println("tcp server listening at " + r.address)

	for {
		conn, err := listener.Accept()
		if err != nil {
			return fmt.Errorf("tcp connection accept error (%v)", err)
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

func (r *TCPServer) ListClients() []*Client {
	clients := make([]*Client, 0, 10)
	r.clients.Range(func(key, value interface{}) bool {
		client := value.(*TCPClient)
		clients = append(clients, &Client{client.imei, client.conn.RemoteAddr().String(), client.connectedAt})
		return true
	})
	return clients
}

func (r *TCPServer) handleConnection(conn net.Conn) {
	logger := r.logger
	client := &TCPClient{conn, time.Now(), ""}
	addr := conn.RemoteAddr().String()
	logKey := addr
	imei := ""

	updateReadDeadline := func() bool {
		if err := conn.SetReadDeadline(time.Now().Add(r.readTimeout)); err != nil {
			logger.Error.Printf("[%s]: SetReadDeadline error (%v)", logKey, err)
			return false
		}
		return true
	}

	writeWithDeadline := func(data []byte) bool {
		if err := conn.SetWriteDeadline(time.Now().Add(r.writeTimeout)); err != nil {
			logger.Error.Printf("[%s]: SetWriteDeadline error (%v)", logKey, err)
			return false
		}
		if _, err := conn.Write(data); err != nil {
			logger.Error.Printf("[%s]: error writing response (%v)", logKey, err)
			return false
		}
		return true
	}

	defer func() {
		if r.OnClose != nil && imei != "" {
			r.OnClose(imei)
		}
		logger.Info.Printf("[%s]: client disconnected", logKey)
		if imei != "" {
			r.clients.Delete(imei)
		}
		if err := conn.Close(); err != nil {
			logger.Error.Printf("[%s]: connection close error (%v)", logKey, err)
		}
	}()

	logger.Info.Printf("[%s]: connected", logKey)
	buf := make([]byte, 256)
	if !updateReadDeadline() {
		return
	}
	size, err := conn.Read(buf[:2])
	if err != nil {
		logger.Error.Printf("[%s]: connection read error (%v)", logKey, err)
		return
	}
	if size != 2 {
		logger.Error.Printf("[%s]: invalid imei message (read: %s)", logKey, hex.EncodeToString(buf[:2]))
		return
	}
	imeiLen := int(binary.BigEndian.Uint16(buf))
	if imeiLen > len(buf) {
		logger.Error.Printf("[%s]: imei length too big (read: %s)", logKey, hex.EncodeToString(buf[:2]))
		return
	}
	if !updateReadDeadline() {
		return
	}
	size, err = conn.Read(buf[:imeiLen])
	if err != nil {
		logger.Error.Printf("[%s]: connection read error (%v)", logKey, err)
		return
	}
	if size < imeiLen {
		logger.Error.Printf("[%s]: invalid read imei size (read: %s)", logKey, hex.EncodeToString(buf[:imeiLen]))
		return
	}
	imei = strings.TrimSpace(string(buf[:imeiLen]))

	if imei == "" {
		logger.Error.Printf("[%s]: invalid imei '%s'", logKey, imei)
		return
	}

	client.imei = imei
	logKey = fmt.Sprintf("%s-%s", imei, addr)

	if r.OnConnect != nil {
		r.OnConnect(imei)
	}

	r.clients.Store(imei, client)

	logger.Info.Printf("[%s]: imei - %s", logKey, client.imei)

	if !writeWithDeadline([]byte{1}) {
		return
	}

	readBuffer := make([]byte, 1300)
	reader := bufio.NewReader(conn)
	for {
		if !updateReadDeadline() {
			return
		}

		peek, err := reader.Peek(1)
		if err != nil {
			logger.Error.Printf("[%s]: peek error (%v)", logKey, err)
			return
		}
		if peek[0] == 0xFF { // ping packet
			if _, err = reader.Discard(1); err != nil {
				logger.Error.Printf("[%s]: reader discard error (%v)", logKey, err)
				return
			}
			logger.Info.Printf("[%s]: received ping", logKey)
			continue
		}
		read, res, err := teltonika.DecodeTCPFromReaderBuf(reader, readBuffer, decodeConfig)
		if err != nil {
			logger.Error.Printf("[%s]: packet decode error (%v)", logKey, err)
			return
		}

		if res.Response != nil && !writeWithDeadline(res.Response) {
			return
		}

		logger.Info.Printf("[%s]: message: %s", logKey, hex.EncodeToString(readBuffer[:read]))
		jsonData, err := json.Marshal(res.Packet)
		if err != nil {
			logger.Error.Printf("[%s]: marshaling error (%v)", logKey, err)
		} else {
			logger.Info.Printf("[%s]: decoded: %s", logKey, string(jsonData))
			for i, data := range res.Packet.Data {
				elements := make([]string, len(data.Elements))
				for j, element := range data.Elements {
					it, err := ioelements.DefaultParser().Parse("*", element.Id, element.Value)
					if err != nil {
						break
					}
					elements[j] = it.String()
				}
				logger.Info.Printf("[%s]: io elements [frame #%d]: %s", logKey, i, strings.Join(elements, ", "))
			}
		}

		if r.OnPacket != nil {
			r.OnPacket(imei, res.Packet)
		}
	}
}

type HTTPServer struct {
	address  string
	hub      TrackersHub
	respChan *sync.Map
	logger   *Logger
}

//goland:noinspection GoUnusedExportedFunction
func NewHTTPServer(address string, hub TrackersHub) *HTTPServer {
	return &HTTPServer{address: address, respChan: &sync.Map{}, hub: hub}
}

func NewHTTPServerLogger(address string, hub TrackersHub, logger *Logger) *HTTPServer {
	return &HTTPServer{address: address, respChan: &sync.Map{}, hub: hub, logger: logger}
}

func (hs *HTTPServer) Run() error {
	logger := hs.logger

	handler := http.NewServeMux()

	handler.HandleFunc("/cmd", hs.handleCmd)

	handler.HandleFunc("/list-clients", hs.listClients)

	logger.Info.Println("http server listening at " + hs.address)

	err := http.ListenAndServe(hs.address, handler)
	if err != nil {
		return fmt.Errorf("http listen error (%v)", err)
	}
	return nil
}

func (hs *HTTPServer) WriteMessage(imei string, message *teltonika.Message) {
	ch, ok := hs.respChan.Load(imei)
	if ok {
		select {
		case ch.(chan *teltonika.Message) <- message:
		}
	}
}

func (hs *HTTPServer) ClientDisconnected(imei string) {
	ch, ok := hs.respChan.Load(imei)
	if ok {
		select {
		case ch.(chan *teltonika.Message) <- nil:
		}
	}
}

func (hs *HTTPServer) listClients(w http.ResponseWriter, _ *http.Request) {
	body, _ := json.Marshal(hs.hub.ListClients())
	w.Header().Set("Content-Type", "application/json")
	_, err := w.Write(body)
	if err != nil {
		hs.logger.Error.Printf("http write error (%v)", err)
		return
	}
}

func (hs *HTTPServer) handleCmd(w http.ResponseWriter, r *http.Request) {
	logger := hs.logger

	params := r.URL.Query()
	imei := params.Get("imei")
	buf := make([]byte, 512)
	n, _ := r.Body.Read(buf)
	cmd := string(buf[:n])

	packet := &teltonika.Packet{
		CodecID:  teltonika.Codec12,
		Data:     nil,
		Messages: []teltonika.Message{{Type: teltonika.TypeCommand, Text: strings.TrimSpace(cmd)}},
	}

	result := make(chan *teltonika.Message, 1)
	defer close(result)
	for {
		if _, loaded := hs.respChan.LoadOrStore(imei, result); !loaded {
			break
		}
		time.Sleep(time.Millisecond * 100)
	}

	defer hs.respChan.Delete(imei)

	w.Header().Set("Content-Type", "application/json")
	if err := hs.hub.SendPacket(imei, packet); err != nil {
		logger.Error.Printf("send packet error (%v)", err)
		w.WriteHeader(http.StatusBadRequest)
		body, _ := json.Marshal(map[string]interface{}{"error": err.Error()})
		_, err = w.Write(body)
		if err != nil {
			logger.Error.Printf("http write error (%v)", err)
		}
	} else {
		logger.Info.Printf("command '%s' sent to '%s'", cmd, imei)
		ticker := time.NewTimer(time.Minute * 3)
		defer ticker.Stop()

		select {
		case msg := <-result:
			if msg != nil {
				body, _ := json.Marshal(map[string]interface{}{"response": msg.Text})
				_, err = w.Write(body)
			} else {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, err = w.Write([]byte(`{"error":"tracker disconnected"}`))
			}
		case <-ticker.C:
			w.WriteHeader(http.StatusGatewayTimeout)
			_, err = w.Write([]byte(`{"error":"tracker response timeout exceeded"}`))
		}

		if err != nil {
			logger.Error.Printf("http write error (%v)", err)
		}
	}
}

func main() {
	var httpAddress string
	var tcpAddress string
	var outHook string
	var readTimeout time.Duration
	var writeTimeout time.Duration
	flag.StringVar(&tcpAddress, "address", "0.0.0.0:8080", "tcp server address")
	flag.StringVar(&httpAddress, "http", "0.0.0.0:8081", "http server address")
	flag.StringVar(&outHook, "hook", "", "output hook\nfor example: http://localhost:8080/push")
	flag.DurationVar(&readTimeout, "read-timeout", time.Minute*2, "receive timeout")
	flag.DurationVar(&writeTimeout, "write-timeout", time.Minute*2, "send timeout")
	flag.Parse()

	logger := &Logger{
		Info:  log.New(os.Stdout, "INFO: ", log.Ldate|log.Ltime),
		Error: log.New(os.Stdout, "ERROR: ", log.Ldate|log.Ltime|log.Lshortfile),
	}

	serverTcp := NewTCPServerLogger(tcpAddress, logger)
	serverHttp := NewHTTPServerLogger(httpAddress, serverTcp, logger)

	serverTcp.writeTimeout = writeTimeout
	serverTcp.readTimeout = readTimeout
	serverTcp.OnPacket = func(imei string, pkt *teltonika.Packet) {
		if pkt.Messages != nil && len(pkt.Messages) > 0 {
			serverHttp.WriteMessage(imei, &pkt.Messages[0])
		}
		if pkt.Data != nil && outHook != "" {
			go hookSend(outHook, imei, pkt, logger)
		}
	}

	serverTcp.OnClose = func(imei string) {
		serverHttp.ClientDisconnected(imei)
	}

	go func() {
		panic(serverTcp.Run())
	}()
	panic(serverHttp.Run())
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

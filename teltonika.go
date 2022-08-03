// Copyright 2022 Alim Zanibekov
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package teltonika

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
)

type GenerationType uint8

//goland:noinspection GoUnusedConst
const (
	OnExit     GenerationType = iota // 0
	OnEntrance                       // 1
	OnBoth                           // 2
	Reserved                         // 3
	Hysteresis                       // 4
	OnChange                         // 5
	Eventual                         // 6
	Periodical                       // 7
	Unknown    GenerationType = 0xFF // 255
)

type CodecId uint8

const (
	Codec8  CodecId = 0x08
	Codec8E CodecId = 0x8E
	Codec16 CodecId = 0x10
	Codec12 CodecId = 0x0C
	Codec13 CodecId = 0x0D
	Codec14 CodecId = 0x0E
)

type MessageType uint8

const (
	TypeCommand  MessageType = 0x05
	TypeResponse MessageType = 0x06
)

type IOElementsAlloc uint8

//goland:noinspection GoUnusedConst
const (
	OnHeap       IOElementsAlloc = iota // Alloc IOElement->Value on heap (make([]byte, x))
	OnReadBuffer                        // IOElement->Value = readBuffer[x:y]
)

type DecodedUDP struct {
	PacketId    uint16  `json:"packetId"`
	AvlPacketId uint8   `json:"avlPacketId"`
	Imei        string  `json:"imei"`
	Packet      *Packet `json:"packet"`
	Response    []byte  `json:"response"`
}

type DecodedTCP struct {
	Packet   *Packet `json:"packet"`
	Response []byte  `json:"response"`
}

type Packet struct {
	CodecID  CodecId   `json:"codecId"`
	Data     []Data    `json:"data,omitempty"`
	Messages []Message `json:"messages,omitempty"`
}

type Data struct {
	TimestampMs    uint64         `json:"timestampMs"`
	Lng            float64        `json:"lng"`
	Lat            float64        `json:"lat"`
	Altitude       int16          `json:"altitude"`
	Angle          uint16         `json:"angle"`
	EventID        uint16         `json:"event_id"`
	Speed          uint16         `json:"speed"`
	Satellites     uint8          `json:"satellites"`
	Priority       uint8          `json:"priority"`
	GenerationType GenerationType `json:"generationType"` // codec 16 else Unknown
	Elements       []IOElement    `json:"elements"`
}

type IOElement struct {
	Id    uint16 `json:"id"`
	Value []byte `json:"value"`
}

type Message struct {
	Timestamp uint32      `json:"timestamp,omitempty"` // codec 13 else 0
	Type      MessageType `json:"type"`
	Imei      string      `json:"imei,omitempty"` // codec 14 else ""
	Text      string      `json:"text"`
}

// DecodeConfig optional configuration that can be passed in all Decode* functions (last param).
// By default, used - DecodeConfig { IoElementsAlloc: OnHeap }
type DecodeConfig struct {
	IoElementsAlloc IOElementsAlloc // IOElement->Value allocation mode: `OnHeap` or `OnReadBuffer`
}

var defaultDecodeConfig = &DecodeConfig{
	IoElementsAlloc: OnHeap,
}

// DecodeTCPFromSlice
// decode (12, 13, 14, 8, 16, or 8 extended codec) tcp packet from slice
// returns the number of bytes read from 'inputBuffer' and decoded packet or an error
func DecodeTCPFromSlice(inputBuffer []byte, config ...*DecodeConfig) (int, *DecodedTCP, error) {
	if len(config) > 1 {
		return 0, nil, fmt.Errorf("too many arguments specified")
	}
	cfg := defaultDecodeConfig
	if len(config) == 1 && config[0] != nil {
		cfg = config[0]
	}
	buf, packet, err := decodeTCPInternal(nil, inputBuffer, nil, cfg)
	if err != nil {
		return 0, nil, err
	}
	return len(buf), packet, nil
}

// DecodeTCPFromReader
// decode (12, 13, 14, 8, 16, or 8 extended codec) tcp packet from io.Reader
// returns decoded packet or an error
func DecodeTCPFromReader(input io.Reader, config ...*DecodeConfig) ([]byte, *DecodedTCP, error) {
	if len(config) > 1 {
		return nil, nil, fmt.Errorf("too many arguments specified")
	}
	cfg := defaultDecodeConfig
	if len(config) == 1 && config[0] != nil {
		cfg = config[0]
	}
	return decodeTCPInternal(input, nil, nil, cfg)
}

// DecodeTCPFromReaderBuf
// decode (12, 13, 14, 8, 16, or 8 extended codec) tcp packet from io.Reader
// writes the read bytes to readBytes buffer (max packet size 1280 bytes)
// returns the number of bytes read and decoded packet or an error
func DecodeTCPFromReaderBuf(input io.Reader, readBytes []byte, config ...*DecodeConfig) (int, *DecodedTCP, error) {
	if len(config) > 1 {
		return 0, nil, fmt.Errorf("too many arguments specified")
	}
	if readBytes == nil {
		return 0, nil, fmt.Errorf("output readBytes is nil, use DecodeTCPFromReader if you dont want use fixed buffer to read")
	}
	cfg := defaultDecodeConfig
	if len(config) == 1 && config[0] != nil {
		cfg = config[0]
	}
	buf, packet, err := decodeTCPInternal(input, nil, readBytes, cfg)
	return len(buf), packet, err
}

// DecodeUDPFromSlice
// decode (12, 13, 14, 8, 16, or 8 extended codec) udp packet from slice
// returns the number of bytes read from 'inputBuffer' and decoded packet or an error
func DecodeUDPFromSlice(inputBuffer []byte, config ...*DecodeConfig) (int, *DecodedUDP, error) {
	if len(config) > 1 {
		return 0, nil, fmt.Errorf("too many arguments specified")
	}
	cfg := defaultDecodeConfig
	if len(config) == 1 && config[0] != nil {
		cfg = config[0]
	}
	buf, packet, err := decodeUDPInternal(nil, inputBuffer, nil, cfg)
	if err != nil {
		return 0, nil, err
	}
	return len(buf), packet, nil
}

// DecodeUDPFromReader
// decode (12, 13, 14, 8, 16, or 8 extended codec) udp packet from io.Reader
// returns the read buffer and decoded packet or an error
func DecodeUDPFromReader(input io.Reader, config ...*DecodeConfig) ([]byte, *DecodedUDP, error) {
	if len(config) > 1 {
		return nil, nil, fmt.Errorf("too many arguments specified")
	}

	cfg := defaultDecodeConfig
	if len(config) == 1 && config[0] != nil {
		cfg = config[0]
	}
	return decodeUDPInternal(input, nil, nil, cfg)
}

// DecodeUDPFromReaderBuf
// decode (12, 13, 14, 8, 16, or 8 extended codec) udp packet from io.Reader
// writes read bytes to readBytes slice (max packet size 1280 bytes)
// returns the number of bytes read and decoded packet or an error
func DecodeUDPFromReaderBuf(input io.Reader, readBytes []byte, config ...*DecodeConfig) (int, *DecodedUDP, error) {
	if len(config) > 1 {
		return 0, nil, fmt.Errorf("too many arguments specified")
	}
	if readBytes == nil {
		return 0, nil, fmt.Errorf("output readBytes is nil, use DecodeUDPFromReader if you don't want use fixed buffer to read")
	}
	cfg := defaultDecodeConfig
	if len(config) == 1 && config[0] != nil {
		cfg = config[0]
	}
	buf, packet, err := decodeUDPInternal(input, nil, readBytes, cfg)
	return len(buf), packet, err
}

// EncodePacket
// encode packet (12, 13, or 14 codec)
// returns an array of bytes with encoded data or an error
func EncodePacket(packet *Packet) ([]byte, error) {
	if packet.Data != nil {
		return nil, fmt.Errorf("encoding avl data packets is not supported")
	}
	if packet.Messages == nil {
		return nil, fmt.Errorf("nothing to encode, packet.Message is nil")
	}
	if !isCMDCodecId(uint8(packet.CodecID)) {
		return nil, fmt.Errorf("codec id %X encoding is not supported", packet.CodecID)
	}

	commandCount := len(packet.Messages)
	dataSize := 3
	for _, command := range packet.Messages {
		dataSize += len(command.Text) + 5
		if packet.CodecID == Codec14 {
			dataSize += 8
		}
		if packet.CodecID == Codec13 {
			dataSize += 4
		}
	}

	buf := make([]byte, dataSize+12)
	shift := 0

	binary.BigEndian.PutUint32(buf, 0)
	shift = 4

	binary.BigEndian.PutUint32(buf[shift:], uint32(dataSize))
	shift += 4

	buf[shift] = uint8(packet.CodecID)
	shift += 1

	buf[shift] = uint8(commandCount)
	shift += 1

	for _, message := range packet.Messages {
		size := len(message.Text)
		if !isMessageTypeSupported(uint8(message.Type)) {
			return nil, fmt.Errorf("message type %X is not supported", message.Type)
		}
		buf[shift] = uint8(message.Type)
		shift += 1

		if packet.CodecID == Codec14 {
			size += 8
		}

		if packet.CodecID == Codec13 {
			size += 4
		}

		binary.BigEndian.PutUint32(buf[shift:], uint32(size))
		shift += 4

		if packet.CodecID == Codec13 {
			binary.BigEndian.PutUint32(buf[shift:], message.Timestamp)
			shift += 4
		} else if packet.CodecID == Codec14 {
			var imei []byte
			var err error
			if (len(message.Imei) % 2) != 0 {
				imei, err = hex.DecodeString("0" + message.Imei)
			} else {
				imei, err = hex.DecodeString(message.Imei)
			}

			if err != nil {
				return nil, err
			}

			copy(buf[shift:], imei[:8])
			shift += 8
		}

		copy(buf[shift:], message.Text)
		shift += len(message.Text)
	}

	buf[shift] = uint8(commandCount)
	shift += 1

	crc := Crc16IBM(buf[8:shift])

	binary.BigEndian.PutUint32(buf[shift:], uint32(crc))
	shift += 4

	return buf, nil
}

func decodeTCPInternal(inputReader io.Reader, inputBuffer []byte, outputBuffer []byte, config *DecodeConfig) ([]byte, *DecodedTCP, error) {
	const headerSize = 8
	var err error
	var header []byte
	if inputBuffer == nil {
		if outputBuffer != nil {
			if len(outputBuffer) < headerSize {
				return nil, nil, fmt.Errorf("output buffer size lower than %v bytes, unable to read header", headerSize)
			}
			header = outputBuffer[:headerSize]
		} else {
			header = make([]byte, headerSize)
		}

		if err = readFromReader(inputReader, header); err != nil {
			return nil, nil, err
		}
	} else {
		if len(inputBuffer) < headerSize {
			return nil, nil, fmt.Errorf("input buffer size lower than %v bytes, unable to read header", headerSize)
		}
		header = inputBuffer[:headerSize]
	}

	preamble := int(binary.BigEndian.Uint32(header[:4]))

	if preamble != 0 {
		return nil, nil, fmt.Errorf("'Preamble' field must be equal to 0. received preamble is %v", preamble)
	}

	// Maximum AVL packet size is 1280 bytes. (Is it with or without CRC, Preamble, Data Field Length?)
	dataFieldLength := int(binary.BigEndian.Uint32(header[4:]))

	if dataFieldLength > 1280 {
		return nil, nil, fmt.Errorf("maximum AVL packet size is 1280 bytes. 'Data Field Length' equal to %v bytes", dataFieldLength)
	}

	remainingSize := dataFieldLength + 4 // + CRC size
	packetSize := remainingSize + headerSize

	var buffer []byte
	if inputBuffer == nil {
		if outputBuffer != nil {
			if len(outputBuffer) < packetSize {
				return nil, nil, fmt.Errorf("output buffer size lower than specified in packet. specified: %v, output size %v bytes", packetSize, len(outputBuffer))
			}
			buffer = outputBuffer[:packetSize]
		} else {
			buffer = make([]byte, packetSize)
			copy(buffer, header)
		}

		if err = readFromReader(inputReader, buffer[headerSize:]); err != nil {
			return nil, nil, err
		}
	} else {
		if len(inputBuffer) < packetSize {
			return nil, nil, fmt.Errorf("input buffer size lower than specified in packet. specified: %v, input size %v bytes", packetSize, len(inputBuffer))
		}
		buffer = inputBuffer[:packetSize]
	}

	reader := newByteReader(buffer[headerSize:], config.IoElementsAlloc == OnHeap)

	packet := &DecodedTCP{
		Packet: &Packet{},
	}

	if err = decodePacket(reader, packet.Packet); err != nil {
		return nil, nil, err
	}

	crcCalc := Crc16IBM(buffer[8 : len(buffer)-4])

	crc, err := reader.ReadUInt32BE()
	if err != nil {
		return nil, nil, err
	}

	if uint32(crcCalc) != crc {
		return nil, nil, fmt.Errorf("calculated CRC-16 sum '%v' is not equal to control CRC-16 sum '%v'", crcCalc, crc)
	}

	if !isCMDCodecId(uint8(packet.Packet.CodecID)) {
		packet.Response = []byte{0x00, 0x00, 0x00, uint8(len(packet.Packet.Data))}
	}

	return buffer, packet, nil
}

func decodeUDPInternal(inputReader io.Reader, inputBuffer []byte, outputBuffer []byte, config *DecodeConfig) ([]byte, *DecodedUDP, error) {
	const headerSize = 5
	var err error
	var header []byte
	if inputBuffer == nil {
		if outputBuffer != nil {
			if len(outputBuffer) < headerSize {
				return nil, nil, fmt.Errorf("output buffer size lower than %v bytes, unable to read header", headerSize)
			}
			header = outputBuffer[:headerSize]
		} else {
			header = make([]byte, headerSize)
		}
		if err = readFromReader(inputReader, header); err != nil {
			return nil, nil, err
		}
	} else {
		if len(inputBuffer) < headerSize {
			return nil, nil, fmt.Errorf("input buffer size lower than %v bytes, unable to read header", headerSize)
		}
		header = inputBuffer[:headerSize]
	}

	size := int(binary.BigEndian.Uint16(header[:2]))

	packetId := binary.BigEndian.Uint16(header[2:])

	// Maximum AVL packet size is 1280 bytes.
	if size > 1280 {
		return nil, nil, fmt.Errorf("maximum AVL packet size is 1280 bytes. 'Data Field Length' equal to %v bytes", size)
	}

	remainingSize := size - 3
	packetSize := remainingSize + headerSize

	var buffer []byte
	if inputBuffer == nil {
		if outputBuffer != nil {
			if len(outputBuffer) < packetSize {
				return nil, nil, fmt.Errorf("output buffer size lower than specified in packet. specified: %v, output size %v bytes", packetSize, len(outputBuffer))
			}
			buffer = outputBuffer[:packetSize]
		} else {
			buffer = make([]byte, packetSize)
			copy(buffer, header)
		}

		if err = readFromReader(inputReader, buffer[headerSize:]); err != nil {
			return nil, nil, err
		}
	} else {
		if len(inputBuffer) < packetSize {
			return nil, nil, fmt.Errorf("input buffer size lower than specified in packet. specified: %v, input size %v bytes", packetSize, len(inputBuffer))
		}
		buffer = inputBuffer[:packetSize]
	}

	reader := newByteReader(buffer[headerSize:], config.IoElementsAlloc == OnHeap)

	avlPacketId, err := reader.ReadUInt8BE()
	if err != nil {
		return nil, nil, err
	}

	imeiLen, err := reader.ReadUInt16BE()
	if err != nil {
		return nil, nil, err
	}

	imei, err := reader.ReadBytes(int(imeiLen))
	if err != nil {
		return nil, nil, err
	}

	packet := &DecodedUDP{
		PacketId:    packetId,
		AvlPacketId: avlPacketId,
		Imei:        string(imei),
		Packet:      &Packet{},
	}

	if err = decodePacket(reader, packet.Packet); err != nil {
		return nil, nil, err
	}

	if !isCMDCodecId(uint8(packet.Packet.CodecID)) {
		packet.Response = []byte{
			0x00,                           // Length
			0x05,                           // Length
			uint8(packetId >> 8),           // Packet ID
			uint8(packetId),                // Packet ID
			0x01,                           // Not usable byte
			avlPacketId,                    // AVL packet ID
			uint8(len(packet.Packet.Data)), // Number of Accepted Data
		}
	}

	return buffer, packet, nil
}

func decodePacket(reader *byteReader, packet *Packet) error {
	codecId, err := reader.ReadUInt8BE()
	if err != nil {
		return err
	}
	dataCount, err := reader.ReadUInt8BE()
	if err != nil {
		return err
	}

	if !isCodecSupported(codecId) {
		return fmt.Errorf("codec id %X is not supported", codecId)
	}

	packet.CodecID = CodecId(codecId)

	if isCMDCodecId(codecId) {
		packet.Messages = make([]Message, dataCount)
		for i := 0; i < int(dataCount); i++ {
			if err = decodeCommand(packet.CodecID, reader, &packet.Messages[i]); err != nil {
				return err
			}
		}
	} else {
		packet.Data = make([]Data, dataCount)
		for i := 0; i < int(dataCount); i++ {
			if err = decodeData(packet.CodecID, reader, &packet.Data[i]); err != nil {
				return err
			}
		}
	}

	dataCountCheck, err := reader.ReadUInt8BE()
	if err != nil {
		return err
	}

	if dataCountCheck != dataCount {
		return fmt.Errorf("'Number of Data 1' is not equal to 'Number of Data 2'. %v != %v", dataCount, dataCountCheck)
	}

	return nil
}

func decodeData(codecId CodecId, reader *byteReader, data *Data) error {
	timestampMs, err := reader.ReadUInt64BE()
	if err != nil {
		return err
	}
	priority, err := reader.ReadUInt8BE()
	if err != nil {
		return err
	}
	lng, err := reader.ReadInt32BE()
	if err != nil {
		return err
	}
	lat, err := reader.ReadInt32BE()
	if err != nil {
		return err
	}
	altitude, err := reader.ReadInt16BE()
	if err != nil {
		return err
	}
	angle, err := reader.ReadUInt16BE()
	if err != nil {
		return err
	}
	satellites, err := reader.ReadUInt8BE()
	if err != nil {
		return err
	}
	speed, err := reader.ReadUInt16BE()
	if err != nil {
		return err
	}

	data.TimestampMs = timestampMs
	data.Lng = float64(lng) / 10000000.0
	data.Lat = float64(lat) / 10000000.0
	data.Altitude = altitude
	data.Angle = angle
	data.Speed = speed
	data.Satellites = satellites
	data.Priority = priority

	if codecId == Codec8 {
		return decodeElementsCodec8(reader, data)
	} else if codecId == Codec8E {
		return decodeElementsCodec8E(reader, data)
	} else if codecId == Codec16 {
		return decodeElementsCodec16(reader, data)
	}

	return nil
}

func decodeCommand(codecId CodecId, reader *byteReader, data *Message) error {
	commandType, err := reader.ReadUInt8BE()
	if err != nil {
		return err
	}

	if !isMessageTypeSupported(commandType) {
		return fmt.Errorf("command type %X is not supported", commandType)
	}

	data.Type = MessageType(commandType)

	commandSize, err := reader.ReadUInt32BE()
	if err != nil {
		return err
	}

	if codecId == Codec13 {
		timestamp, err := reader.ReadUInt32BE()
		if err != nil {
			return err
		}
		data.Timestamp = timestamp
	} else if codecId == Codec14 {
		imei, err := reader.ReadBytes(8)
		if err != nil {
			return err
		}
		data.Imei = strings.TrimLeft(hex.EncodeToString(imei), "0")
		commandSize -= 8
	}

	command, err := reader.ReadBytes(int(commandSize))
	if err != nil {
		return err
	}
	data.Text = string(command)
	return nil
}

func decodeElementsCodec8(reader *byteReader, data *Data) error {
	eventId, err := reader.ReadUInt8BE()
	if err != nil {
		return err
	}

	ioCount, err := reader.ReadUInt8BE()
	if err != nil {
		return err
	}

	data.GenerationType = Unknown
	data.EventID = uint16(eventId)
	data.Elements = make([]IOElement, ioCount)

	var n uint8
	var id uint8
	var value []byte
	var k = 0

	for i := 1; i <= 8; i *= 2 {
		n, err = reader.ReadUInt8BE()
		if err != nil {
			return err
		}
		for j := 0; j < int(n); j++ {
			id, err = reader.ReadUInt8BE()
			if err != nil {
				return err
			}
			value, err = reader.ReadBytes(i)
			if err != nil {
				return err
			}

			data.Elements[k] = IOElement{
				Id:    uint16(id),
				Value: value,
			}
			k++
		}
	}

	return nil
}

func decodeElementsCodec16(reader *byteReader, data *Data) error {
	eventId, err := reader.ReadUInt16BE()
	if err != nil {
		return err
	}

	generationType, err := reader.ReadUInt8BE()
	if err != nil {
		return err
	}

	ioCount, err := reader.ReadUInt8BE()
	if err != nil {
		return err
	}

	if generationType > 7 {
		return fmt.Errorf("invalid generation type, must be number from 0 to 7, got %v", generationType)
	}

	data.GenerationType = GenerationType(generationType)
	data.EventID = eventId
	data.Elements = make([]IOElement, ioCount)

	var n uint8
	var id uint16
	var value []byte
	var k = 0

	for i := 1; i <= 8; i *= 2 {
		n, err = reader.ReadUInt8BE()
		if err != nil {
			return err
		}
		for j := 0; j < int(n); j++ {
			id, err = reader.ReadUInt16BE()
			if err != nil {
				return err
			}
			value, err = reader.ReadBytes(i)
			if err != nil {
				return err
			}

			data.Elements[k] = IOElement{
				Id:    id,
				Value: value,
			}
			k++
		}
	}

	return nil
}

func decodeElementsCodec8E(reader *byteReader, data *Data) error {
	eventId, err := reader.ReadUInt16BE()
	if err != nil {
		return err
	}

	ioCount, err := reader.ReadUInt16BE()
	if err != nil {
		return err
	}

	data.GenerationType = Unknown
	data.EventID = eventId
	data.Elements = make([]IOElement, ioCount)

	var n uint16
	var id uint16
	var value []byte
	var k = 0

	for i := 1; i <= 8; i *= 2 {
		n, err = reader.ReadUInt16BE()
		if err != nil {
			return err
		}
		for j := 0; j < int(n); j++ {
			id, err = reader.ReadUInt16BE()
			if err != nil {
				return err
			}
			value, err = reader.ReadBytes(i)
			if err != nil {
				return err
			}

			data.Elements[k] = IOElement{
				Id:    id,
				Value: value,
			}
			k++
		}
	}

	var length uint16
	var ioCountNX uint16

	ioCountNX, err = reader.ReadUInt16BE()
	if err != nil {
		return err
	}

	for i := k; i < int(ioCountNX); i++ {
		id, err = reader.ReadUInt16BE()
		if err != nil {
			return err
		}
		length, err = reader.ReadUInt16BE()
		if err != nil {
			return err
		}
		value, err = reader.ReadBytes(int(length))
		if err != nil {
			return err
		}
		data.Elements[i] = IOElement{
			Id:    id,
			Value: value,
		}
	}

	return nil
}

func isCodecSupported(id uint8) bool {
	return id == uint8(Codec8) || id == uint8(Codec8E) || id == uint8(Codec16) ||
		id == uint8(Codec12) || id == uint8(Codec13) || id == uint8(Codec14)
}

func isMessageTypeSupported(id uint8) bool {
	return id == uint8(TypeCommand) || id == uint8(TypeResponse)
}

func isCMDCodecId(id uint8) bool {
	return id == uint8(Codec12) || id == uint8(Codec13) || id == uint8(Codec14)
}

func readFromReader(input io.Reader, buffer []byte) error {
	size := len(buffer)
	read := 0
	for read < size {
		n, err := input.Read(buffer[read:])
		if n == 0 || errors.Is(err, io.EOF) {
			return fmt.Errorf("unable to read packet. received %v of %v bytes", read, size)
		}
		if err != nil {
			return err
		}
		read += n
	}
	return nil
}

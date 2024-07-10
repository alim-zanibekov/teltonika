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
	Codec15 CodecId = 0x0F
)

type MessageType uint8

const (
	TypeCommand     MessageType = 0x05
	TypeResponse    MessageType = 0x06
	TypeNotExecuted MessageType = 0x11
)

type IOElementsAlloc uint8

//goland:noinspection GoUnusedConst
const (
	OnHeap       IOElementsAlloc = iota // Alloc IOElement->Value on heap (make([]byte, x))
	OnReadBuffer                        // IOElement->Value = readBuffer[x:y]
)

type PacketResponse []byte

type DecodedUDP struct {
	PacketId    uint16         `json:"packetId"`
	AvlPacketId uint8          `json:"avlPacketId"`
	Imei        string         `json:"imei"`
	Packet      *Packet        `json:"packet"`
	Response    PacketResponse `json:"response"`
}

type DecodedTCP struct {
	Packet   *Packet        `json:"packet"`
	Response PacketResponse `json:"response"`
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

type IOElementValue []byte

type IOElement struct {
	Id    uint16         `json:"id"`
	Value IOElementValue `json:"value"`
}

type Message struct {
	Timestamp uint32      `json:"timestamp,omitempty"` // if codec is 13 or 15 else 0
	Type      MessageType `json:"type"`                // may contain an arbitrary value if codec is 15
	Imei      string      `json:"imei,omitempty"`      // if codec is 14 or 15 else ""
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

func (r IOElementValue) MarshalJSON() ([]byte, error) {
	return []byte(`"` + hex.EncodeToString(r) + `"`), nil
}

func (r IOElementValue) UnmarshalJSON(data []byte) error {
	_, err := hex.Decode(data, r)
	return err
}

func (r PacketResponse) MarshalJSON() ([]byte, error) {
	return []byte(`"` + hex.EncodeToString(r) + `"`), nil
}

func (r PacketResponse) UnmarshalJSON(data []byte) error {
	_, err := hex.Decode(data, r)
	return err
}

func (r *GenerationType) MarshalJSON() ([]byte, error) {
	switch *r {
	case OnExit:
		return []byte(`"OnExit"`), nil
	case OnEntrance:
		return []byte(`"OnEntrance"`), nil
	case OnBoth:
		return []byte(`"OnBoth"`), nil
	case Reserved:
		return []byte(`"Reserved"`), nil
	case Hysteresis:
		return []byte(`"Hysteresis"`), nil
	case OnChange:
		return []byte(`"OnChange"`), nil
	case Eventual:
		return []byte(`"Eventual"`), nil
	case Periodical:
		return []byte(`"Periodical"`), nil
	case Unknown:
		return []byte(`"Unknown"`), nil
	default:
		return nil, fmt.Errorf("unknown generation type %d", r)
	}
}

func (r *GenerationType) UnmarshalJSON(data []byte) error {
	if len(data) < 4 {
		return fmt.Errorf("unknown generation type '%s'", string(data))
	}
	key := string(data[1 : len(data)-1])
	switch key {
	case "OnExit":
		*r = OnExit
	case "OnEntrance":
		*r = OnEntrance
	case "OnBoth":
		*r = OnBoth
	case "Reserved":
		*r = Reserved
	case "Hysteresis":
		*r = Hysteresis
	case "OnChange":
		*r = OnChange
	case "Eventual":
		*r = Eventual
	case "Periodical":
		*r = Periodical
	case "Unknown":
		*r = Unknown
	default:
		return fmt.Errorf("unknown generation type '%s'", key)
	}
	return nil
}

// DecodeTCPFromSlice
// decode (12, 13, 14, 15, 8, 16, or 8 extended codec) tcp packet from slice
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
// decode (12, 13, 14, 15, 8, 16, or 8 extended codec) tcp packet from io.Reader
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
// decode (12, 13, 14, 15, 8, 16, or 8 extended codec) tcp packet from io.Reader
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
// decode (12, 13, 14, 15, 8, 16, or 8 extended codec) udp packet from slice
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
// decode (12, 13, 14, 15, 8, 16, or 8 extended codec) udp packet from io.Reader
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
// decode (12, 13, 14, 15, 8, 16, or 8 extended codec) udp packet from io.Reader
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

// EncodePacketTCP
// encode packet (12, 13, 14, 15, 8, 16, or 8 extended codec)
// returns an array of bytes with encoded data or an error
// note: implementations for 8, 16, 8E are practically not needed, they are made only for testing
func EncodePacketTCP(packet *Packet) ([]byte, error) {
	return encodeTCPInternal(packet)
}

// EncodePacketUDP
// encode packet (12, 13, 14, 15, 8, 16, or 8 extended codec)
// returns an array of bytes with encoded data or an error
// note: implementations for 8, 16, 8E are practically not needed, they are made only for testing
func EncodePacketUDP(imei string, packetId uint16, avlPacketId uint8, packet *Packet) ([]byte, error) {
	return encodeUDPInternal(imei, packetId, avlPacketId, packet)
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

	crcCalc := Crc16IBM(reader.input[:reader.pos])

	crc, err := reader.ReadUInt32BE()
	if err != nil {
		return nil, nil, err
	}

	if uint32(crcCalc) != crc {
		return nil, nil, fmt.Errorf("calculated CRC-16 sum '%08X' is not equal to control CRC-16 sum '%08X'", crcCalc, crc)
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
		return fmt.Errorf("codec %d is not supported", codecId)
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

	return fmt.Errorf("unknown codec %d", codecId)
}

func decodeCommand(codecId CodecId, reader *byteReader, data *Message) error {
	commandType, err := reader.ReadUInt8BE()
	if err != nil {
		return err
	}

	if codecId == Codec12 && commandType != uint8(TypeResponse) && commandType != uint8(TypeCommand) ||
		codecId == Codec13 && commandType != uint8(TypeResponse) ||
		codecId == Codec14 && commandType != uint8(TypeResponse) && commandType != uint8(TypeCommand) && commandType != uint8(TypeNotExecuted) {
		return fmt.Errorf("message type 0x%X is not supported with codec %d", commandType, codecId)
	}

	data.Type = MessageType(commandType)

	commandSize, err := reader.ReadUInt32BE()
	if err != nil {
		return err
	}

	if codecId == Codec13 || codecId == Codec15 {
		timestamp, err := reader.ReadUInt32BE()
		if err != nil {
			return err
		}
		data.Timestamp = timestamp
		commandSize -= 4
	}
	if codecId == Codec14 || codecId == Codec15 {
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
			if k >= int(ioCount) {
				return fmt.Errorf("too many i/o elements, expected at most %d, found %d", ioCount, k)
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
			if k >= int(ioCount) {
				return fmt.Errorf("too many i/o elements, expected at most %d, found %d", ioCount, k)
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
			if k >= int(ioCount) {
				return fmt.Errorf("too many i/o elements, expected at most %d, found %d", ioCount, k)
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

	for i := 0; i < int(ioCountNX); i++ {
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
		if k >= int(ioCount) {
			return fmt.Errorf("too many i/o elements, expected at most %d, found %d", ioCount, i)
		}
		data.Elements[k] = IOElement{
			Id:    id,
			Value: value,
		}
		k++
	}

	return nil
}

func encodeTCPInternal(packet *Packet) ([]byte, error) {
	if !isCodecSupported(uint8(packet.CodecID)) {
		return nil, fmt.Errorf("codec %d is not supported", packet.CodecID)
	}
	if packet.Messages != nil && packet.Data != nil {
		return nil, fmt.Errorf("invalid packet. Only one of packet.Message, packet.Data should contain data")
	}
	if isCMDCodecId(uint8(packet.CodecID)) && packet.Messages == nil {
		return nil, fmt.Errorf("nothing to encode, packet.Messages is nil")
	}
	if !isCMDCodecId(uint8(packet.CodecID)) && packet.Data == nil {
		return nil, fmt.Errorf("nothing to encode, packet.Data is nil")
	}

	dataSize := 3 // packet fields size
	if isCMDCodecId(uint8(packet.CodecID)) {
		for _, command := range packet.Messages {
			dataSize += len(command.Text) + 5
			if packet.CodecID == Codec14 {
				dataSize += 8
			} else if packet.CodecID == Codec13 {
				dataSize += 4
			} else if packet.CodecID == Codec15 {
				dataSize += 12
			}
		}
	} else {
		var n int
		var err error
		for _, data := range packet.Data {
			dataSize += 24 // data fields size
			if packet.CodecID == Codec8 {
				n, err = calculateElementsSizeCodec8(data.Elements)
			} else if packet.CodecID == Codec8E {
				n, err = calculateElementsSizeCodec8E(data.Elements)
			} else if packet.CodecID == Codec16 {
				n, err = calculateElementsSizeCodec16(data.Elements)
			}
			if err != nil {
				return nil, err
			}
			dataSize += n
		}
	}

	buf := make([]byte, dataSize+12)
	binary.BigEndian.PutUint32(buf, 0)
	pos := 4

	binary.BigEndian.PutUint32(buf[pos:], uint32(dataSize))
	pos += 4

	n, err := encodePacket(packet, buf[pos:])
	if err != nil {
		return nil, err
	}
	pos += n

	crc := Crc16IBM(buf[8:pos])
	binary.BigEndian.PutUint32(buf[pos:], uint32(crc))

	return buf, nil
}

func encodeUDPInternal(imei string, packetId uint16, avlPacketId uint8, packet *Packet) ([]byte, error) {
	if !isCodecSupported(uint8(packet.CodecID)) {
		return nil, fmt.Errorf("codec %d is not supported", packet.CodecID)
	}
	if packet.Messages != nil && packet.Data != nil {
		return nil, fmt.Errorf("invalid packet. Only one of packet.Message, packet.Data should contain data")
	}
	if isCMDCodecId(uint8(packet.CodecID)) && packet.Messages == nil {
		return nil, fmt.Errorf("nothing to encode, packet.Messages is nil")
	}
	if !isCMDCodecId(uint8(packet.CodecID)) && packet.Data == nil {
		return nil, fmt.Errorf("nothing to encode, packet.Data is nil")
	}

	dataSize := 11 + len(imei) // packet fields(3) + header(5) + imeiLen(2) + avlPacketId(1) + len(imei)

	if isCMDCodecId(uint8(packet.CodecID)) {
		for _, command := range packet.Messages {
			dataSize += len(command.Text) + 5
			if packet.CodecID == Codec14 {
				dataSize += 8
			} else if packet.CodecID == Codec13 {
				dataSize += 4
			} else if packet.CodecID == Codec15 {
				dataSize += 12
			}
		}
	} else {
		var n int
		var err error
		for _, data := range packet.Data {
			dataSize += 24 // data fields size
			if packet.CodecID == Codec8 {
				n, err = calculateElementsSizeCodec8(data.Elements)
			} else if packet.CodecID == Codec8E {
				n, err = calculateElementsSizeCodec8E(data.Elements)
			} else if packet.CodecID == Codec16 {
				n, err = calculateElementsSizeCodec16(data.Elements)
			}
			if err != nil {
				return nil, err
			}
			dataSize += n
		}
	}

	// Maximum AVL packet size is 1280 bytes.
	if dataSize > 1280 {
		return nil, fmt.Errorf("maximum AVL packet size is 1280 bytes. Estimated size is equal to %v bytes", dataSize)
	}

	buf := make([]byte, dataSize)
	binary.BigEndian.PutUint16(buf, uint16(dataSize-2))
	binary.BigEndian.PutUint16(buf[2:], packetId)
	buf[4] = 0x01
	buf[5] = avlPacketId
	binary.BigEndian.PutUint16(buf[6:], uint16(len(imei)))
	copy(buf[8:], imei)
	pos := 8 + len(imei)
	_, err := encodePacket(packet, buf[pos:])
	if err != nil {
		return nil, err
	}
	return buf, nil
}

func encodePacket(packet *Packet, buf []byte) (int, error) {
	if !isCodecSupported(uint8(packet.CodecID)) {
		return 0, fmt.Errorf("codec %d is not supported", packet.CodecID)
	}

	if len(buf) < 3 {
		return 0, fmt.Errorf("output buffer too small, expected at least 3 bytes for output buffer, got %d", len(buf))
	}
	buf[0] = uint8(packet.CodecID)

	pos := 2
	if isCMDCodecId(uint8(packet.CodecID)) {
		if len(packet.Messages) > 255 {
			return 0, fmt.Errorf("too many messages - %d (> 255)", len(packet.Messages))
		}
		buf[1] = uint8(len(packet.Messages))
		for _, message := range packet.Messages {
			n, err := encodeMessage(packet.CodecID, &message, buf[pos:])
			if err != nil {
				return 0, err
			}
			pos += n
		}
		if pos >= len(buf) {
			return 0, fmt.Errorf("output buffer is too small, expected at least %d bytes for output buffer, got %d", pos+1, len(buf))
		}
		buf[pos] = uint8(len(packet.Messages))
		pos++
	} else {
		if len(packet.Data) > 255 {
			return 0, fmt.Errorf("too many data frames - %d (> 255)", len(packet.Data))
		}
		buf[1] = uint8(len(packet.Data))
		for _, data := range packet.Data {
			n, err := encodeData(packet.CodecID, &data, buf[pos:])
			if err != nil {
				return 0, err
			}
			pos += n
		}
		if pos >= len(buf) {
			return 0, fmt.Errorf("output buffer is too small, expected at least %d bytes for output buffer, got %d", pos+1, len(buf))
		}
		buf[pos] = uint8(len(packet.Data))
		pos++
	}

	return pos, nil
}

func encodeMessage(codecId CodecId, message *Message, buf []byte) (int, error) {
	if codecId == Codec12 && message.Type != TypeResponse && message.Type != TypeCommand ||
		codecId == Codec13 && message.Type != TypeResponse ||
		codecId == Codec14 && message.Type != TypeResponse && message.Type != TypeCommand && message.Type != TypeNotExecuted {
		return 0, fmt.Errorf("message type 0x%X is not supported with codec %d", message.Type, codecId)
	}

	size := len(message.Text)
	if codecId == Codec14 {
		size += 8
	} else if codecId == Codec13 {
		size += 4
	} else if codecId == Codec15 {
		size += 12
	}

	if len(buf) < size+5 {
		return 0, fmt.Errorf("output buffer is too small, expected at least %d bytes for output buffer, got %d", size+5, len(buf))
	}

	buf[0] = uint8(message.Type)
	binary.BigEndian.PutUint32(buf[1:], uint32(size))
	pos := 5

	if codecId == Codec13 || codecId == Codec15 {
		binary.BigEndian.PutUint32(buf[pos:], message.Timestamp)
		pos += 4
	}
	if codecId == Codec14 || codecId == Codec15 {
		var imei []byte
		var err error
		if (len(message.Imei) % 2) != 0 {
			imei, err = hex.DecodeString("0" + message.Imei)
		} else {
			imei, err = hex.DecodeString(message.Imei)
		}

		if err != nil {
			return 0, err
		}

		copy(buf[pos:], imei[:8])
		pos += 8
	}

	copy(buf[pos:], message.Text)
	pos += len(message.Text)

	return pos, nil
}

func encodeData(codecId CodecId, data *Data, buf []byte) (int, error) {
	if len(buf) < 24 {
		return 0, fmt.Errorf("output buffer is too small, expected at least 24 bytes for output buffer, got %d", len(buf))
	}

	binary.BigEndian.PutUint64(buf, data.TimestampMs)
	buf[8] = data.Priority
	binary.BigEndian.PutUint32(buf[9:], uint32(data.Lng*10000000.0))
	binary.BigEndian.PutUint32(buf[13:], uint32(data.Lat*10000000.0))
	binary.BigEndian.PutUint16(buf[17:], uint16(data.Altitude))
	binary.BigEndian.PutUint16(buf[19:], data.Angle)
	buf[21] = data.Satellites
	binary.BigEndian.PutUint16(buf[22:], data.Speed)

	var n int
	var err error
	if codecId == Codec8 {
		n, err = encodeElementsCodec8(data, buf[24:])
	} else if codecId == Codec8E {
		n, err = encodeElementsCodec8E(data, buf[24:])
	} else if codecId == Codec16 {
		n, err = encodeElementsCodec16(data, buf[24:])
	} else {
		return 0, fmt.Errorf("unsupported codec %d", codecId)
	}

	if err != nil {
		return 0, err
	}

	return n + 24, err
}

func encodeElementsCodec8(data *Data, buf []byte) (int, error) {
	if data.EventID > 255 {
		return 0, fmt.Errorf("event id (%d) is too large (> 255)", data.EventID)
	}
	ioCount := len(data.Elements)
	if ioCount > 255 {
		return 0, fmt.Errorf("too many i/o elements - %d (> 255)", ioCount)
	}
	expectedSize, err := calculateElementsSizeCodec8(data.Elements)
	if err != nil {
		return 0, err
	}

	if len(buf) < expectedSize {
		return 0, fmt.Errorf("output buffer is too small, expected %d bytes for output buffer, got %d", expectedSize, len(buf))
	}

	buf[0] = uint8(data.EventID)
	buf[1] = uint8(ioCount)
	pos := 2
	for i := 1; i <= 8; i *= 2 {
		written := 0
		dataCountPos := pos
		pos++
		for j := 0; j < ioCount; j++ {
			length := len(data.Elements[j].Value)
			if length == i {
				buf[pos] = uint8(data.Elements[j].Id)
				pos++
				copy(buf[pos:], data.Elements[j].Value)
				pos += length
				written++
			}
		}
		buf[dataCountPos] = uint8(written) // written <= 255
	}

	return pos, nil
}

func calculateElementsSizeCodec8(elements []IOElement) (int, error) {
	expectedSize := 6 // eventId, ioCount (2) + 4 for each group uint8 size
	for i := 0; i < len(elements); i++ {
		if elements[i].Id > 255 {
			return 0, fmt.Errorf("IOElement[%d].Id (%d) is too large (> 255)", i, elements[i].Id)
		}
		length := len(elements[i].Value)
		if length != 1 && length != 2 && length != 4 && length != 8 {
			return 0, fmt.Errorf("IOElement[%d].Value has invalid size %d (allowed: 1,2,4,8)", i, length)
		}
		expectedSize += length + 1
	}
	return expectedSize, nil
}

func encodeElementsCodec16(data *Data, buf []byte) (int, error) {
	ioCount := len(data.Elements)
	if ioCount > 255 {
		return 0, fmt.Errorf("too many i/o elements - %d (> 255)", ioCount)
	}

	if data.GenerationType > 7 {
		return 0, fmt.Errorf("invalid generation type, must be number from 0 to 7, got %v", data.GenerationType)
	}

	expectedSize, err := calculateElementsSizeCodec16(data.Elements)
	if err != nil {
		return 0, err
	}

	if len(buf) < expectedSize {
		return 0, fmt.Errorf("output buffer is too small, expected %d bytes for output buffer, got %d", expectedSize, len(buf))
	}

	binary.BigEndian.PutUint16(buf, data.EventID)
	buf[2] = uint8(data.GenerationType)
	buf[3] = uint8(ioCount)

	pos := 4
	for i := 1; i <= 8; i *= 2 {
		written := 0
		dataCountPos := pos
		pos++
		for j := 0; j < ioCount; j++ {
			length := len(data.Elements[j].Value)
			if length == i {
				binary.BigEndian.PutUint16(buf[pos:], data.Elements[j].Id)
				pos += 2
				copy(buf[pos:], data.Elements[j].Value)
				pos += length
				written++
			}
		}
		buf[dataCountPos] = uint8(written) // written <= 255
	}

	return pos, nil
}

func calculateElementsSizeCodec16(elements []IOElement) (int, error) {
	expectedSize := 8 // eventId, ioCount, generationType (4) + 4 for each group uint8 size
	for i := 0; i < len(elements); i++ {
		length := len(elements[i].Value)
		if length != 1 && length != 2 && length != 4 && length != 8 {
			return 0, fmt.Errorf("IOElement[%d].Value has invalid size %d (allowed: 1,2,4,8)", i, length)
		}
		expectedSize += length + 2
	}
	return expectedSize, nil
}

func encodeElementsCodec8E(data *Data, buf []byte) (int, error) {
	ioCount := len(data.Elements)
	if ioCount > 65535 {
		return 0, fmt.Errorf("too many i/o elements - %d (> 65535)", ioCount)
	}

	expectedSize, err := calculateElementsSizeCodec8E(data.Elements)
	if err != nil {
		return 0, err
	}

	if len(buf) < expectedSize {
		return 0, fmt.Errorf("output buffer is too small, expected %d bytes for output buffer, got %d", expectedSize, len(buf))
	}

	binary.BigEndian.PutUint16(buf, data.EventID)
	binary.BigEndian.PutUint16(buf[2:], uint16(ioCount))

	pos := 4
	for i := 1; i <= 8; i *= 2 {
		written := 0
		dataCountPos := pos
		pos += 2
		for j := 0; j < ioCount; j++ {
			length := len(data.Elements[j].Value)
			if length == i {
				binary.BigEndian.PutUint16(buf[pos:], data.Elements[j].Id)
				pos += 2
				copy(buf[pos:], data.Elements[j].Value)
				pos += length
				written++
			}
		}
		binary.BigEndian.PutUint16(buf[dataCountPos:], uint16(written)) // written <= 65535
	}

	ioCountNX := 0
	ioCountNXPos := pos
	pos += 2
	for j := 0; j < ioCount; j++ {
		length := len(data.Elements[j].Value)
		if length != 1 && length != 2 && length != 4 && length != 8 {
			binary.BigEndian.PutUint16(buf[pos:], data.Elements[j].Id)
			pos += 2
			binary.BigEndian.PutUint16(buf[pos:], uint16(length))
			pos += 2
			copy(buf[pos:], data.Elements[j].Value)
			pos += length

			ioCountNX++
		}
	}
	binary.BigEndian.PutUint16(buf[ioCountNXPos:], uint16(ioCountNX))

	return pos, nil
}

func calculateElementsSizeCodec8E(elements []IOElement) (int, error) {
	expectedSize := 14 // eventId, ioCount, ioCountNX (6) + 8 for each group uint8 size
	for i := 0; i < len(elements); i++ {
		length := len(elements[i].Value)
		if length > 65535 {
			return 0, fmt.Errorf("IOElement[%d].Value length %d too high (> 65535)", i, length)
		}
		if length == 0 {
			return 0, fmt.Errorf("IOElement[%d].Value length is 0", i)
		}

		if length != 1 && length != 2 && length != 4 && length != 8 {
			expectedSize += length + 4
		} else {
			expectedSize += length + 2
		}
	}
	return expectedSize, nil
}

func isCodecSupported(id uint8) bool {
	return id == uint8(Codec8) || id == uint8(Codec8E) || id == uint8(Codec16) ||
		id == uint8(Codec12) || id == uint8(Codec13) || id == uint8(Codec14) || id == uint8(Codec15)
}

func isCMDCodecId(id uint8) bool {
	return id == uint8(Codec12) || id == uint8(Codec13) || id == uint8(Codec14) || id == uint8(Codec15)
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

# Teltonika codecs

The `teltonika` package provides an implementation of
[Teltonika](https://teltonika-gps.com/) tracker codecs,
the library supports decoding regular messages from trackers
and encoding/decoding commands and command responses

Implemented Codec 8, 8E, 16 (tcp/udp) decode and Codec 12, 13, 14 encode/decode

The `ioelements` package can be used to represent IO Elements in human-readable
format, see [tools](/tools)

### Example

> more examples in [examples](/examples) folder (see [examples/README.md](/examples/README.md))

```go
package main

import (
    "encoding/hex"
    "encoding/json"
    "fmt"

    "github.com/alim-zanibekov/teltonika"
    "github.com/alim-zanibekov/teltonika/ioelements"
)

func main() {
    packetHex := "00000000000000A98E020000017357633410000F0DC39B2095964A00AC00F80B00000000000B000500F00100150400C8000" +
        "04501007156000500B5000500B600040018000000430FE00044011B000100F10000601B000000000000017357633BE1000F0DC39B209" +
        "5964A00AC00F80B000001810001000000000000000000010181002D11213102030405060708090A0B0C0D0E0F104545010ABC2121020" +
        "30405060708090A0B0C0D0E0F10020B010AAD020000BF30"
    packet, _ := hex.DecodeString(packetHex)
    _, decoded, err := teltonika.DecodeTCPFromSlice(packet)
    if err != nil {
        panic(err)
    }
    res, _ := json.Marshal(decoded)
    fmt.Printf("%s\n", res)

    parser := ioelements.DefaultParser()

    elements := make(map[int][]*ioelements.IOElement)
    for i, data := range decoded.Packet.Data {
        elements[i] = make([]*ioelements.IOElement, len(data.Elements))
        for j, element := range data.Elements {
            elements[i][j], err = parser.Parse("*", element.Id, element.Value)
            if err != nil {
                panic(err)
            }
        }
    }

    for i, items := range elements {
        fmt.Printf("Frame #%d\n", i)
        for _, element := range items {
            fmt.Printf("    %s\n", element)
        }
    }
}
```

<details>
<summary>Output (byte arrays in hex)</summary>

```json
{
  "packet": {
    "codecId": 142,
    "data": [
      {
        "timestampMs": 1594898986000,
        "lng": 25.2560283,
        "lat": 54.667425,
        "altitude": 172,
        "angle": 248,
        "event_id": 0,
        "speed": 0,
        "satellites": 11,
        "priority": 0,
        "generationType": 255,
        "elements": [
          {
            "id": 240,
            "value": "01"
          },
          {
            "id": 21,
            "value": "04"
          },
          {
            "id": 200,
            "value": "00"
          },
          {
            "id": 69,
            "value": "01"
          },
          {
            "id": 113,
            "value": "56"
          },
          {
            "id": 181,
            "value": "0005"
          },
          {
            "id": 182,
            "value": "0004"
          },
          {
            "id": 24,
            "value": "0000"
          },
          {
            "id": 67,
            "value": "0fe0"
          },
          {
            "id": 68,
            "value": "011b"
          },
          {
            "id": 241,
            "value": "0000601b"
          }
        ]
      },
      {
        "timestampMs": 1594898988001,
        "lng": 25.2560283,
        "lat": 54.667425,
        "altitude": 172,
        "angle": 248,
        "event_id": 385,
        "speed": 0,
        "satellites": 11,
        "priority": 0,
        "generationType": 255,
        "elements": [
          {
            "id": 385,
            "value": "11213102030405060708090a0b0c0d0e0f104545010abc212102030405060708090a0b0c0d0e0f10020b010aad"
          }
        ]
      }
    ]
  },
  "response": "00000002"
}
```

```text
Frame #0
    Movement: true
    GSM Signal: 4
    Sleep Mode: 0
    GNSS Status: 1
    Battery Level: 86%
    GNSS PDOP: 0.500
    GNSS HDOP: 0.400
    Speed: 0km/h
    Battery Voltage: 4.064V
    Battery Current: 0.283A
    Active GSM Operator: 24603
Frame #1
    Beacon: 11213102030405060708090a0b0c0d0e0f104545010abc212102030405060708090a0b0c0d0e0f10020b010aad
```

</details>

### API

Package `teltonika`

Data structures:

```go
package teltonika

type PacketResponse []byte

type DecodedUDP struct {
    PacketId    uint16         // Packet ID
    AvlPacketId uint8          // AVL Packet ID
    Imei        string         // Device IMEI
    Packet      *Packet        // Decoded Packet
    Response    PacketResponse // Response to received packet
}

type DecodedTCP struct {
    Packet   *Packet        // Decoded Packet
    Response PacketResponse // Response to received packet (4 bytes, len(Packet.Data))
}

type Packet struct {
    CodecID  CodecId   // Codec ID, if 8, 8E or 16 Data field is not nil, if 12, 13 or 14 Messages field is not nil
    Data     []Data    // Packet AVLData array
    Messages []Message // Packet Messages array (max 1 message)
}

type Data struct {
    TimestampMs    uint64         // UNIX timestamp in milliseconds
    Lng            float64        // Longitude, east – west position
    Lat            float64        // Latitude, north – south position
    Altitude       int16          // Meters above sea level
    Angle          uint16         // Angle in degrees from the North Pole (clock-wise)
    EventID        uint16         // If data is acquired on event this field contains IOElement id else 0
    Speed          uint16         // Speed calculated from satellites (km/h)
    Satellites     uint8          // Number of visible satellites
    Priority       uint8          // Priority (0 Low, 1 High, 2 Panic)
    GenerationType GenerationType // Codec 16 generation type
    Elements       []IOElement    // Array containing IO Elements
}

type IOElementValue []byte

type IOElement struct {
    Id    uint16         // IO element ID
    Value IOElementValue // Value of the element (for codec 16 and 8 1-8 bytes, for codec 8E 1-X bytes)
}

type Message struct {
    Timestamp uint32      // UNIX timestamp in milliseconds (only codec 13)
    Type      MessageType // Type (Command or Response)
    Imei      string      // Device IMEI (only codec 14)
    Text      string      // Command or Response represented as string
}

// DecodeConfig optional configuration that can be passed in all Decode* functions (last param).
// By default, used - DecodeConfig { ioElementsAlloc: OnHeap }
type DecodeConfig struct {
    ioElementsAlloc IOElementsAlloc // IOElement->Value allocation mode: `OnHeap` or `OnReadBuffer`
}

```

Methods:

```go
package teltonika

// DecodeTCPFromSlice
// decode (12, 13, 14, 8, 16, or 8 extended codec) tcp packet from slice
// returns the number of bytes read from 'inputBuffer' and decoded packet or an error
func DecodeTCPFromSlice(inputBuffer []byte, config ...*DecodeConfig) (int, *DecodedTCP, error)

// DecodeTCPFromReader
// decode (12, 13, 14, 8, 16, or 8 extended codec) tcp packet from io.Reader
// returns decoded packet or an error
func DecodeTCPFromReader(input io.Reader, config ...*DecodeConfig) ([]byte, *DecodedTCP, error)

// DecodeTCPFromReaderBuf
// decode (12, 13, 14, 8, 16, or 8 extended codec) tcp packet from io.Reader
// writes the read bytes to readBytes buffer (max packet size 1280 bytes)
// returns the number of bytes read and decoded packet or an error
func DecodeTCPFromReaderBuf(input io.Reader, readBytes []byte, config ...*DecodeConfig) (int, *DecodedTCP, error)

// DecodeUDPFromSlice
// decode (12, 13, 14, 8, 16, or 8 extended codec) udp packet from slice
// returns the number of bytes read from 'inputBuffer' and decoded packet or an error
func DecodeUDPFromSlice(inputBuffer []byte, config ...*DecodeConfig) (int, *DecodedUDP, error)

// DecodeUDPFromReader
// decode (12, 13, 14, 8, 16, or 8 extended codec) udp packet from io.Reader
// returns the read buffer and decoded packet or an error
func DecodeUDPFromReader(input io.Reader, config ...*DecodeConfig) ([]byte, *DecodedUDP, error)

// DecodeUDPFromReaderBuf
// decode (12, 13, 14, 8, 16, or 8 extended codec) udp packet from io.Reader
// writes read bytes to readBytes slice (max packet size 1280 bytes)
// returns the number of bytes read and decoded packet or an error
func DecodeUDPFromReaderBuf(input io.Reader, readBytes []byte, config ...*DecodeConfig) (int, *DecodedUDP, error)

// EncodePacket
// encode packet (12, 13, or 14 codec)
// returns an array of bytes with encoded data or an error
func EncodePacket(packet *Packet) ([]byte, error)
```

Package `ioelements`

Data structures:

```go
package ioelements

type IOElementDefinition struct {
    Id              uint16      // I/O Element id
    Name            string      // Element name
    NumBytes        int         // Bytes count
    Type            ElementType // Element type (Signed, Unsigned, HEX, ASCII)
    Min             float64     // Min value if number
    Max             float64     // Max value if number
    Multiplier      float64     // Multiplier (used for numbers)
    Units           string      // Element units
    Description     string      // Element full description
    SupportedModels []string    // List of device names that support this I/O element
    Groups          []string    // Element groups
}

type IOElement struct {
    Id         uint16               // I/O Element id
    Value      interface{}          // Value (float64, int64, uint64, string)
    Definition *IOElementDefinition // I/O Element definition
}
```

Methods:

```go
package ioelements

// NewParser create new Parser
func NewParser(definitions []IOElementDefinition) *Parser

// DefaultParser returns default parser with IO Element definitions represented in `ioelements_dump.go` file
func DefaultParser() *Parser

// GetElementInfo returns full description of IO Element by its id and model name
// If you don't know the model name, you can skip the model name check by passing '*' as the model name
func (r *Parser) GetElementInfo(modelName string, id uint16) (*IOElementDefinition, error)

// Parse parses IO Element (result can be represented in numan-readable format)
// If you don't know the model name, you can skip the model name check by passing '*' as the model name
func (r *Parser) Parse(modelName string, id uint16, buffer []byte) (*IOElement, error)
```

### Simple benchmarks (go test -bench)

Command:

```shell
go test -bench=. -benchmem
```

Output:

```text
goos: darwin
goarch: amd64
pkg: github.com/alim-zanibekov/teltonika
cpu: Intel(R) Core(TM) i9-9880H CPU @ 2.30GHz
BenchmarkTCPDecode-16                                             542722              1982 ns/op             405 B/op         11 allocs/op
BenchmarkTCPDecodeReader-16                                       524685              2034 ns/op             517 B/op         13 allocs/op
BenchmarkUDPDecodeSlice-16                                       1000000              1022 ns/op            1350 B/op         38 allocs/op
BenchmarkUDPDecodeReader-16                                      1115318              1106 ns/op            1551 B/op         40 allocs/op
BenchmarkTCPDecodeAllocElementsOnReadBuffer-16                    574172              1932 ns/op             382 B/op          5 allocs/op
BenchmarkTCPDecodeReaderAllocElementsOnReadBuffer-16              618633              1955 ns/op             498 B/op          7 allocs/op
BenchmarkUDPDecodeSliceAllocElementsOnReadBuffer-16              1876750               634.1 ns/op          1267 B/op          6 allocs/op
BenchmarkUDPDecodeReaderAllocElementsOnReadBuffer-16             1657054               712.0 ns/op          1469 B/op          8 allocs/op
BenchmarkEncode-16                                               7849704               145.8 ns/op            36 B/op          1 allocs/op
PASS
ok      github.com/alim-zanibekov/teltonika     14.699s
```

As you can see from the results, passing the `&teltonika.DecodeConfig{teltonika.OnReadBuffer}`
parameter to the Decode* function noticeably speeds them up, but this prevents the
garbage collector from removing the byte array from which the packet
was read until all references to it (Packet->Data->Elements->Value) are removed,
so this option should be used if long-term packet storage in
RAM is not required (almost always?)

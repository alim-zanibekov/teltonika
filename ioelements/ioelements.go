// Copyright 2022-2024 Alim Zanibekov
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package ioelements

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"
)

type ElementType uint8

const (
	IOElementSigned ElementType = iota
	IOElementUnsigned
	IOElementHEX
	IOElementASCII
)

type IOElementDefinition struct {
	Id              uint16      `json:"id"`
	Name            string      `json:"name"`
	NumBytes        int         `json:"numBytes"`
	Type            ElementType `json:"type"`
	Min             float64     `json:"min"`
	Max             float64     `json:"max"`
	Multiplier      float64     `json:"multiplier"`
	Units           string      `json:"units"`
	Description     string      `json:"description"`
	SupportedModels []string    `json:"supportedModels"`
	Groups          []string    `json:"groups"`
}

type IOElement struct {
	Id         uint16               `json:"id,omitempty"`
	Value      interface{}          `json:"value,omitempty"`
	Definition *IOElementDefinition `json:"definition,omitempty"`
}

type Decoder struct {
	definitions     []IOElementDefinition
	supportedModels map[string]bool
}

var defaultDecoder = &Decoder{ioElementDefinitions, supportedModels}

// Teltonika documents sensor status values as raw integers before scaling.
var sensorStatusCodes = map[int64]string{
	850:   "Sensor not ready",
	2000:  "Value read error",
	3000:  "Not connected",
	4000:  "ID failed",
	5000:  "Sensor not ready",
	32767: "Sensor not found",
	32766: "Failed sensor data parsing",
	32765: "Abnormal sensor state",
	32764: "Abnormal sensor state",
}

func (r *IOElement) String() string {
	if r.Definition == nil {
		return fmt.Sprintf("IO %d: %v", r.Id, r.Value)
	}
	switch r.Value.(type) {
	case float64:
		return fmt.Sprintf("%s: %.3f%s", r.Definition.Name, r.Value, r.Definition.Units)
	default:
		return fmt.Sprintf("%s: %v%s", r.Definition.Name, r.Value, r.Definition.Units)
	}
}

// NewDecoder create new Decoder
func NewDecoder(definitions []IOElementDefinition) *Decoder {
	allSupportedModels := map[string]bool{}
	for _, it := range definitions {
		for _, model := range it.SupportedModels {
			allSupportedModels[model] = true
		}
	}
	return &Decoder{definitions, allSupportedModels}
}

// DefaultDecoder returns a decoder with I/O Element definitions represented in `ioelements_dump.go` file
func DefaultDecoder() *Decoder {
	return defaultDecoder
}

func definitionSupportsModel(def *IOElementDefinition, modelName string) bool {
	for _, model := range def.SupportedModels {
		if model == modelName {
			return true
		}
	}
	return false
}

func isVariableLengthType(t ElementType) bool {
	return t == IOElementHEX || t == IOElementASCII
}

func definitionMatchesWireSize(def *IOElementDefinition, wireSize int) bool {
	if wireSize <= 0 {
		return false
	}
	if isVariableLengthType(def.Type) {
		if def.NumBytes <= 0 {
			return true
		}
		return def.NumBytes == wireSize
	}
	if def.NumBytes > 0 {
		return def.NumBytes == wireSize
	}
	return wireSize == 1 || wireSize == 2 || wireSize == 4 || wireSize == 8
}

func (r *Decoder) selectDefinition(modelName string, id uint16, wireSize int) (*IOElementDefinition, error) {
	if modelName != "*" && !r.supportedModels[modelName] {
		return nil, fmt.Errorf("model '%s' is not supported", modelName)
	}

	var modelFallback *IOElementDefinition
	var wildcardSizeMatch *IOElementDefinition
	var wildcardFallback *IOElementDefinition

	for i := range r.definitions {
		def := &r.definitions[i]
		if def.Id != id {
			continue
		}

		if modelName != "*" {
			if !definitionSupportsModel(def, modelName) {
				continue
			}
			if wireSize > 0 && definitionMatchesWireSize(def, wireSize) {
				return def, nil
			}
			if modelFallback == nil {
				modelFallback = def
			}
			continue
		}

		if wireSize > 0 && definitionMatchesWireSize(def, wireSize) {
			if wildcardSizeMatch == nil {
				wildcardSizeMatch = def
			}
			continue
		}
		if wildcardFallback == nil {
			wildcardFallback = def
		}
	}

	if modelName != "*" {
		if modelFallback != nil {
			return modelFallback, nil
		}
		return nil, fmt.Errorf("element with id %v not found for model '%s'", id, modelName)
	}
	if wildcardSizeMatch != nil {
		return wildcardSizeMatch, nil
	}
	if wildcardFallback != nil {
		return wildcardFallback, nil
	}
	return nil, fmt.Errorf("element with id %v not found", id)
}

// GetElementInfo returns full description of I/O Element by its id and model name.
// If you don't know the model name, you can skip the model name check by passing '*' as the model name.
// When multiple definitions share an IO ID, pass the expected on-wire byte length via GetElementInfoWithSize.
func (r *Decoder) GetElementInfo(modelName string, id uint16) (*IOElementDefinition, error) {
	return r.selectDefinition(modelName, id, 0)
}

// GetElementInfoWithSize returns the I/O Element definition that best matches the on-wire byte length.
func (r *Decoder) GetElementInfoWithSize(modelName string, id uint16, wireSize int) (*IOElementDefinition, error) {
	return r.selectDefinition(modelName, id, wireSize)
}

// Decode decodes an I/O Element by model name and id (result can be represented in human-readable format).
// Wire byte length is used to disambiguate IO IDs that map to multiple catalog entries.
func (r *Decoder) Decode(modelName string, id uint16, buffer []byte) (*IOElement, error) {
	def, err := r.selectDefinition(modelName, id, len(buffer))
	if err != nil {
		return nil, err
	}
	return r.DecodeByDefinition(def, buffer)
}

func decodeNumericUnsigned(buffer []byte) uint64 {
	switch len(buffer) {
	case 1:
		return uint64(buffer[0])
	case 2:
		return uint64(binary.BigEndian.Uint16(buffer))
	case 4:
		return uint64(binary.BigEndian.Uint32(buffer))
	default:
		return binary.BigEndian.Uint64(buffer)
	}
}

func decodeNumericSigned(buffer []byte) int64 {
	switch len(buffer) {
	case 1:
		return int64(int8(buffer[0]))
	case 2:
		return int64(int16(binary.BigEndian.Uint16(buffer)))
	case 4:
		return int64(int32(binary.BigEndian.Uint32(buffer)))
	default:
		return int64(binary.BigEndian.Uint64(buffer))
	}
}

func definitionUsesSensorStatusCodes(def *IOElementDefinition) bool {
	if strings.Contains(def.Name, "Temperature") {
		return true
	}
	desc := def.Description
	return strings.Contains(desc, "850") ||
		strings.Contains(desc, "3000") ||
		strings.Contains(desc, "32767")
}

func interpretSensorStatus(def *IOElementDefinition, raw int64) (interface{}, bool) {
	if !definitionUsesSensorStatusCodes(def) {
		return nil, false
	}
	if msg, ok := sensorStatusCodes[raw]; ok {
		return msg, true
	}
	return nil, false
}

func isBooleanDefinition(def *IOElementDefinition, wireSize int) bool {
	return wireSize == 1 &&
		def.NumBytes == 1 &&
		def.Type == IOElementUnsigned &&
		def.Min == 0 &&
		def.Max == 1
}

// DecodeByDefinition decodes an I/O Element according to a given definition.
// Numeric decoding uses the on-wire byte length from the AVL packet.
func (r *Decoder) DecodeByDefinition(def *IOElementDefinition, buffer []byte) (*IOElement, error) {
	if len(buffer) == 0 {
		return nil, fmt.Errorf("unable to decode io element with id %v: empty buffer", def.Id)
	}

	var res interface{}

	switch def.Type {
	case IOElementUnsigned, IOElementSigned:
		if isBooleanDefinition(def, len(buffer)) {
			res = buffer[0] == 1
			break
		}

		size := len(buffer)
		if size != 1 && size != 2 && size != 4 && size != 8 {
			return nil, fmt.Errorf("unsupported numeric io element size %d for id %v", size, def.Id)
		}

		if def.Type == IOElementUnsigned {
			raw := int64(decodeNumericUnsigned(buffer))
			if status, ok := interpretSensorStatus(def, raw); ok {
				res = status
				break
			}
			if def.Multiplier != 1.0 {
				res = float64(raw) * def.Multiplier
			} else {
				res = raw
			}
		} else {
			raw := decodeNumericSigned(buffer)
			if status, ok := interpretSensorStatus(def, raw); ok {
				res = status
				break
			}
			if def.Multiplier != 1.0 {
				res = float64(raw) * def.Multiplier
			} else {
				res = raw
			}
		}
	case IOElementHEX:
		res = hex.EncodeToString(buffer)
	case IOElementASCII:
		res = string(buffer)
	}

	if res == nil {
		return nil, fmt.Errorf("unable to proceed io element with id %v for buffer '%s'", def.Id, hex.EncodeToString(buffer))
	}

	return &IOElement{
		def.Id, res, def,
	}, nil
}

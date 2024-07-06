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

func (r *IOElement) String() string {
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

// GetElementInfo returns full description of I/O Element by its id and model name
// If you don't know the model name, you can skip the model name check by passing '*' as the model name
func (r *Decoder) GetElementInfo(modelName string, id uint16) (*IOElementDefinition, error) {
	if modelName != "*" && !r.supportedModels[modelName] {
		return nil, fmt.Errorf("model '%s' is not supported", modelName)
	}

	for _, e := range r.definitions {
		if e.Id != id {
			continue
		}
		if modelName == "*" {
			return &e, nil
		}

		for _, v := range e.SupportedModels {
			if v != modelName {
				continue
			}
			return &e, nil
		}
	}

	return nil, fmt.Errorf("element with id %v not found", id)
}

// Decode decodes an I/O Element by model name and id (result can be represented in numan-readable format)
// If you don't know the model name, you can skip the model name check by passing '*' as the model name
func (r *Decoder) Decode(modelName string, id uint16, buffer []byte) (*IOElement, error) {
	def, err := r.GetElementInfo(modelName, id)
	if err != nil {
		return nil, err
	}
	return r.DecodeByDefinition(def, buffer)
}

// DecodeByDefinition decodes an I/O Element according to a given definition
func (r *Decoder) DecodeByDefinition(def *IOElementDefinition, buffer []byte) (*IOElement, error) {
	var res interface{}

	size := len(buffer)
	if size == 1 && def.Min == 0 && def.Max == 1 && def.Type == IOElementUnsigned {
		res = buffer[0] == 1
	} else if (size == 1 || size == 2 || size == 4 || size == 8) && (def.Type == IOElementUnsigned || def.Type == IOElementSigned) {
		buf := make([]byte, 8)
		copy(buf[8-size:], buffer)

		if def.Type == IOElementUnsigned {
			v := binary.BigEndian.Uint64(buf)
			if def.Multiplier != 1.0 {
				res = float64(v) * def.Multiplier
			} else {
				res = v
			}
		} else if def.Type == IOElementSigned {
			if (buffer[0] >> 7) == 1 {
				buf[8-size] &= 0x7F
				buf[0] |= 0x80
			}
			v := int64(binary.BigEndian.Uint64(buf))
			if def.Multiplier != 1.0 {
				res = float64(v) * def.Multiplier
			} else {
				res = v
			}
		}
	} else if def.Type == IOElementHEX {
		res = hex.EncodeToString(buffer)
	} else if def.Type == IOElementASCII {
		res = string(buffer)
	}

	if res == nil {
		return nil, fmt.Errorf("unable to proceed io element with id %v for buffer '%s'", def.Id, hex.EncodeToString(buffer))
	}

	return &IOElement{
		def.Id, res, def,
	}, nil
}

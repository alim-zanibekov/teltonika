// Copyright 2022-2024 Alim Zanibekov
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package ioelements

import (
	"testing"
)

func TestDecodeIgnitionBoolean(t *testing.T) {
	dec := DefaultDecoder()
	el, err := dec.Decode("FMB920", 239, []byte{1})
	if err != nil {
		t.Fatal(err)
	}
	if el.Value != true {
		t.Fatalf("expected true ignition, got %v", el.Value)
	}
}

func TestDecodeTotalOdometer(t *testing.T) {
	dec := DefaultDecoder()
	el, err := dec.Decode("FMB920", 16, []byte{0x01, 0x5E, 0x2C, 0x88})
	if err != nil {
		t.Fatal(err)
	}
	if el.Value.(int64) != 22949000 {
		t.Fatalf("expected 22949000, got %v", el.Value)
	}
}

func TestDecodeAxisXWithSizeDisambiguation(t *testing.T) {
	dec := DefaultDecoder()
	el, err := dec.Decode("FMB920", 17, []byte{0x00, 0x1D})
	if err != nil {
		t.Fatal(err)
	}
	if el.Definition.Name != "Axis X" {
		t.Fatalf("expected Axis X, got %q", el.Definition.Name)
	}
	if el.Value != int64(29) {
		t.Fatalf("expected 29, got %v", el.Value)
	}
}

func TestWildcardLookupPrefersWireSize(t *testing.T) {
	dec := DefaultDecoder()

	def8, err := dec.GetElementInfoWithSize("*", 8, 8)
	if err != nil {
		t.Fatal(err)
	}
	if def8.Name != "Authorized iButton" {
		t.Fatalf("expected 8-byte definition, got %q", def8.Name)
	}

	def2, err := dec.GetElementInfoWithSize("*", 8, 2)
	if err != nil {
		t.Fatal(err)
	}
	if def2.Name != "Dallas Temperature 6" {
		t.Fatalf("expected 2-byte definition, got %q", def2.Name)
	}
}

func TestDecodeDallasTemperatureErrorCode(t *testing.T) {
	dec := DefaultDecoder()
	el, err := dec.Decode("FMB640", 6, []byte{0x03, 0x52}) // 850
	if err != nil {
		t.Fatal(err)
	}
	if el.Value != "Sensor not ready" {
		t.Fatalf("expected sensor status string, got %v", el.Value)
	}
}

func TestDecodeNXHexElement(t *testing.T) {
	dec := DefaultDecoder()
	payload := []byte{0x11, 0x21, 0x31, 0x02, 0x03, 0x04}
	el, err := dec.Decode("*", 385, payload)
	if err != nil {
		t.Fatal(err)
	}
	if el.Value != "112131020304" {
		t.Fatalf("unexpected hex value %v", el.Value)
	}
}

func TestDecodeAnalogInput(t *testing.T) {
	dec := DefaultDecoder()
	el, err := dec.Decode("FMB920", 9, []byte{0x00, 0x2B})
	if err != nil {
		t.Fatal(err)
	}
	v, ok := el.Value.(float64)
	if !ok || v < 0.042999 || v > 0.043001 {
		t.Fatalf("expected ~0.043, got %v", el.Value)
	}
}

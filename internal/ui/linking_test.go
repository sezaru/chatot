package ui

import (
	"bytes"
	"testing"
)

var pngSignature = []byte{0x89, 'P', 'N', 'G'}

func TestQRPNGValidImage(t *testing.T) {
	png, err := qrPNG("test")
	if err != nil {
		t.Fatalf("qrPNG: %v", err)
	}
	if len(png) == 0 {
		t.Fatal("qrPNG returned no bytes")
	}
	if !bytes.HasPrefix(png, pngSignature) {
		t.Errorf("qrPNG output missing PNG signature: % x", png[:4])
	}
}

func TestQRPNGEmptyPayloadErrors(t *testing.T) {
	png, err := qrPNG("")
	if err == nil {
		t.Fatal("expected an error for empty payload")
	}
	if png != nil {
		t.Errorf("expected nil bytes on error, got %d bytes", len(png))
	}
}

func TestFormatPairCode(t *testing.T) {
	got := formatPairCode("ABCD-1234")
	want := "On your phone, enter this code: ABCD-1234"
	if got != want {
		t.Errorf("formatPairCode = %q, want %q", got, want)
	}
}

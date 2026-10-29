package encoding_test

import (
	"testing"

	"filex/internal/encoding"
)

func TestDetectUTF8(t *testing.T) {
	data := []byte("Hello, World!")
	result := encoding.Detect(data)
	if result.Encoding != encoding.EncUTF8 {
		t.Errorf("expected utf8, got %s", result.Encoding)
	}
	if result.BOM {
		t.Error("expected no BOM")
	}
}

func TestDetectUTF8BOM(t *testing.T) {
	data := append(encoding.BOMUTF8, []byte("Hello")...)
	result := encoding.Detect(data)
	if result.Encoding != encoding.EncUTF8BOM {
		t.Errorf("expected utf8bom, got %s", result.Encoding)
	}
	if !result.BOM {
		t.Error("expected BOM")
	}
}

func TestDetectUTF16LE(t *testing.T) {
	data := []byte{0xFF, 0xFE, 0x48, 0x00, 0x69, 0x00}
	result := encoding.Detect(data)
	if result.Encoding != encoding.EncUTF16LE {
		t.Errorf("expected utf16le, got %s", result.Encoding)
	}
	if !result.BOM {
		t.Error("expected BOM")
	}
}

func TestDetectUTF16BE(t *testing.T) {
	data := []byte{0xFE, 0xFF, 0x00, 0x48, 0x00, 0x69}
	result := encoding.Detect(data)
	if result.Encoding != encoding.EncUTF16BE {
		t.Errorf("expected utf16be, got %s", result.Encoding)
	}
	if !result.BOM {
		t.Error("expected BOM")
	}
}

func TestEditPreservesBytes(t *testing.T) {
	data := []byte("Hello, World! This is a test.")
	find := "World"
	replace := "FileX"

	result, editResult, err := encoding.Edit(data, find, replace, encoding.EncUTF8, false)
	if err != nil {
		t.Fatalf("edit failed: %v", err)
	}

	if !editResult.Modified {
		t.Error("expected modified")
	}

	expected := "Hello, FileX! This is a test."
	if string(result) != expected {
		t.Errorf("expected %q, got %q", expected, string(result))
	}

	if editResult.BytesBefore != len(data) {
		t.Errorf("bytes_before mismatch: %d != %d", editResult.BytesBefore, len(data))
	}
}

func TestEditUTF16(t *testing.T) {
	data := []byte{0xFF, 0xFE, 0x48, 0x00, 0x69, 0x00}
	find := "Hi"
	replace := "Yo"

	result, editResult, err := encoding.Edit(data, find, replace, encoding.EncUTF16LE, false)
	if err != nil {
		t.Fatalf("edit failed: %v", err)
	}

	if !editResult.Modified {
		t.Error("expected modified")
	}

	if result[0] != 0xFF || result[1] != 0xFE {
		t.Error("BOM not preserved")
	}
}

func TestEditNotFound(t *testing.T) {
	data := []byte("Hello")
	_, _, err := encoding.Edit(data, "World", "FileX", encoding.EncUTF8, false)
	if err != encoding.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestEditNotUnique(t *testing.T) {
	data := []byte("Hello Hello Hello")
	_, _, err := encoding.Edit(data, "Hello", "Hi", encoding.EncUTF8, false)
	if err != encoding.ErrNotUnique {
		t.Errorf("expected ErrNotUnique, got %v", err)
	}
}

func TestEditAll(t *testing.T) {
	data := []byte("Hello Hello Hello")
	result, editResult, err := encoding.Edit(data, "Hello", "Hi", encoding.EncUTF8, true)
	if err != nil {
		t.Fatalf("edit failed: %v", err)
	}

	if editResult.Matches != 3 {
		t.Errorf("expected 3 matches, got %d", editResult.Matches)
	}

	expected := "Hi Hi Hi"
	if string(result) != expected {
		t.Errorf("expected %q, got %q", expected, string(result))
	}
}

package soundboard

import (
	"strings"
	"testing"
)

func TestAudioExt(t *testing.T) {
	// OGG magic bytes
	ogg := []byte{0x4F, 0x67, 0x67, 0x53, 0, 0, 0, 0, 0, 0, 0, 0}
	if ext, ok := audioExt(ogg); !ok || ext != ".ogg" {
		t.Errorf("ogg: got (%q, %v)", ext, ok)
	}

	// MP3 ID3v2
	mp3 := []byte{0x49, 0x44, 0x33, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	if ext, ok := audioExt(mp3); !ok || ext != ".mp3" {
		t.Errorf("mp3 id3: got (%q, %v)", ext, ok)
	}

	// MP3 raw MPEG frame sync (no ID3 header)
	// 0xFF 0xFB: sync word (0xFB & 0xE0 == 0xE0), MPEG version 3 (0xFB & 0x18 == 0x10 != 0x08), layer 3 (0xFB & 0x06 == 0x02 != 0x00)
	mp3Raw := []byte{0xFF, 0xFB, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	if ext, ok := audioExt(mp3Raw); !ok || ext != ".mp3" {
		t.Errorf("mp3 raw: got (%q, %v)", ext, ok)
	}

	// WAV RIFF....WAVE
	wav := []byte{0x52, 0x49, 0x46, 0x46, 0, 0, 0, 0, 0x57, 0x41, 0x56, 0x45}
	if ext, ok := audioExt(wav); !ok || ext != ".wav" {
		t.Errorf("wav: got (%q, %v)", ext, ok)
	}

	// PNG (not an audio file)
	png := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0, 0, 0, 0}
	if _, ok := audioExt(png); ok {
		t.Error("png should not be accepted")
	}

	// Too short
	if _, ok := audioExt([]byte{0x4F, 0x67}); ok {
		t.Error("too-short slice should not be accepted")
	}
}

func TestValidateName(t *testing.T) {
	if err := ValidateName(""); err == nil {
		t.Error("empty name should fail")
	}
	if err := ValidateName("airhorn"); err != nil {
		t.Errorf("valid name: %v", err)
	}
	if err := ValidateName(strings.Repeat("x", 33)); err == nil {
		t.Error("33-char name should fail")
	}
	if err := ValidateName(strings.Repeat("x", 32)); err != nil {
		t.Errorf("32-char name should pass: %v", err)
	}
}

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

func TestValidateEmoji(t *testing.T) {
	// valid: empty string (optional)
	if err := ValidateEmoji(""); err != nil {
		t.Errorf("empty emoji: unexpected error: %v", err)
	}
	// valid: single emoji
	if err := ValidateEmoji("😄"); err != nil {
		t.Errorf("single emoji: unexpected error: %v", err)
	}
	// valid: exactly 64 runes
	emoji64 := strings.Repeat("a", 64)
	if err := ValidateEmoji(emoji64); err != nil {
		t.Errorf("64-char emoji: unexpected error: %v", err)
	}
	// invalid: 65 runes
	emoji65 := strings.Repeat("a", 65)
	if err := ValidateEmoji(emoji65); err == nil {
		t.Error("65-char emoji: expected error, got nil")
	}
}

func TestReadAll(t *testing.T) {
	t.Run("under limit", func(t *testing.T) {
		data, exceeded, err := ReadAll(strings.NewReader("hello"), 10)
		if err != nil || exceeded || string(data) != "hello" {
			t.Fatalf("unexpected: data=%q exceeded=%v err=%v", data, exceeded, err)
		}
	})
	t.Run("exact limit", func(t *testing.T) {
		data, exceeded, err := ReadAll(strings.NewReader("hello"), 5)
		if err != nil || exceeded || string(data) != "hello" {
			t.Fatalf("unexpected: data=%q exceeded=%v err=%v", data, exceeded, err)
		}
	})
	t.Run("over limit", func(t *testing.T) {
		_, exceeded, err := ReadAll(strings.NewReader("hello world"), 5)
		if err != nil || !exceeded {
			t.Fatalf("expected exceeded=true, got exceeded=%v err=%v", exceeded, err)
		}
	})
}

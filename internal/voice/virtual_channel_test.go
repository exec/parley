package voice

import "testing"

func TestParseVirtualChannel(t *testing.T) {
	tests := []struct {
		in       string
		wantKind Kind
		wantID   int64
		wantErr  bool
	}{
		{"s:42", KindServer, 42, false},
		{"dm:1234567890", KindDM, 1234567890, false},
		{"s:0", KindServer, 0, false},
		{"", 0, 0, true},
		{"42", 0, 0, true},
		{"x:42", 0, 0, true},
		{"s:", 0, 0, true},
		{"s:abc", 0, 0, true},
		{"dm:-1", KindDM, -1, false}, // negative IDs are accepted; semantic check is downstream
	}
	for _, tt := range tests {
		got, err := ParseVirtualChannel(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Errorf("Parse(%q): expected error, got %+v", tt.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("Parse(%q): unexpected error %v", tt.in, err)
			continue
		}
		if got.Kind != tt.wantKind || got.ID != tt.wantID {
			t.Errorf("Parse(%q) = %+v, want {Kind:%v ID:%d}", tt.in, got, tt.wantKind, tt.wantID)
		}
	}
}

func TestVirtualChannelString(t *testing.T) {
	tests := []struct {
		vc   VirtualChannel
		want string
	}{
		{VirtualChannel{Kind: KindServer, ID: 42}, "s:42"},
		{VirtualChannel{Kind: KindDM, ID: 7}, "dm:7"},
	}
	for _, tt := range tests {
		if got := tt.vc.String(); got != tt.want {
			t.Errorf("String(%+v) = %q, want %q", tt.vc, got, tt.want)
		}
	}
}

func TestRoundtrip(t *testing.T) {
	for _, in := range []string{"s:1", "dm:99", "s:9999999999"} {
		vc, err := ParseVirtualChannel(in)
		if err != nil {
			t.Fatalf("Parse(%q): %v", in, err)
		}
		if got := vc.String(); got != in {
			t.Errorf("roundtrip %q -> %q", in, got)
		}
	}
}

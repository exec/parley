package voice

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Kind discriminates the type of voice room a VirtualChannel addresses.
type Kind int

const (
	KindServer Kind = iota
	KindDM
)

// VirtualChannel is the namespaced identity of a voice room (server VC or DM/GC call).
// Use the String form (e.g. "s:42", "dm:7") as the LiveKit room name, the Redis presence
// key, and the WS broadcast topic so the backend never has to know the difference.
type VirtualChannel struct {
	Kind Kind
	ID   int64
}

func (v VirtualChannel) String() string {
	switch v.Kind {
	case KindServer:
		return "s:" + strconv.FormatInt(v.ID, 10)
	case KindDM:
		return "dm:" + strconv.FormatInt(v.ID, 10)
	}
	panic(fmt.Sprintf("voice: unknown VirtualChannel kind %d", v.Kind))
}

// ParseVirtualChannel parses an "s:N" or "dm:N" string into a VirtualChannel.
func ParseVirtualChannel(s string) (VirtualChannel, error) {
	var kind Kind
	var rest string
	switch {
	case strings.HasPrefix(s, "dm:"):
		kind, rest = KindDM, s[3:]
	case strings.HasPrefix(s, "s:"):
		kind, rest = KindServer, s[2:]
	default:
		return VirtualChannel{}, errors.New("invalid virtual channel id: missing prefix")
	}
	if rest == "" {
		return VirtualChannel{}, errors.New("invalid virtual channel id: empty id")
	}
	id, err := strconv.ParseInt(rest, 10, 64)
	if err != nil {
		return VirtualChannel{}, fmt.Errorf("invalid virtual channel id: %w", err)
	}
	return VirtualChannel{Kind: kind, ID: id}, nil
}

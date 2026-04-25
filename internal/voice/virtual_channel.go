package voice

import (
	"errors"
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
	return ""
}

// ParseVirtualChannel parses an "s:N" or "dm:N" string into a VirtualChannel.
func ParseVirtualChannel(s string) (VirtualChannel, error) {
	var prefix, rest string
	if r, ok := strings.CutPrefix(s, "dm:"); ok {
		prefix, rest = "dm", r
	} else if r, ok := strings.CutPrefix(s, "s:"); ok {
		prefix, rest = "s", r
	} else {
		return VirtualChannel{}, errors.New("invalid virtual channel id: missing prefix")
	}
	if rest == "" {
		return VirtualChannel{}, errors.New("invalid virtual channel id: empty id")
	}
	id, err := strconv.ParseInt(rest, 10, 64)
	if err != nil {
		return VirtualChannel{}, errors.New("invalid virtual channel id: " + err.Error())
	}
	switch prefix {
	case "s":
		return VirtualChannel{Kind: KindServer, ID: id}, nil
	case "dm":
		return VirtualChannel{Kind: KindDM, ID: id}, nil
	}
	return VirtualChannel{}, errors.New("unreachable")
}

package account

import "errors"

// ErrInvalidConfirmation is returned when the supplied confirm_username does
// not match the authenticated user's username. Mapped to 400 by the handler.
var ErrInvalidConfirmation = errors.New("invalid confirmation")

// ErrHasBlockers is returned when the user owns servers and/or group DMs that
// still have other members. The user must transfer ownership or disband those
// before deletion can proceed. Mapped to 409 by the handler. Use errors.As
// against *BlockersError to surface the offending IDs/names.
var ErrHasBlockers = errors.New("has blockers")

// BlockersError carries the list of servers and group DMs that block deletion.
// Wraps ErrHasBlockers so callers can errors.Is(err, ErrHasBlockers) for the
// kind check and errors.As(err, &blockersErr) for the payload.
type BlockersError struct {
	Servers  []BlockerInfo
	GroupDMs []BlockerInfo
}

// BlockerInfo is the per-entity payload returned in the 409 response body.
type BlockerInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Error implements error.
func (e *BlockersError) Error() string { return "account has blockers" }

// Unwrap returns ErrHasBlockers so errors.Is works for the kind check.
func (e *BlockersError) Unwrap() error { return ErrHasBlockers }

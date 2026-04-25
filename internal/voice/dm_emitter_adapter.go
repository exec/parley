package voice

import (
	"context"
)

// dmServiceLike is the slice of dm.Service methods the adapter forwards to.
// Defined here so this package doesn't import internal/dm; cmd/api wiring
// passes the real *dm.Service which satisfies this implicit interface.
type dmServiceLike interface {
	EmitCallStarted(ctx context.Context, channelID, actorUserID, startedAtMs int64) error
	EmitCallEnded(ctx context.Context, channelID, lastLeaverUserID, durationMs, startedAtMs int64) error
	EmitCallMissed(ctx context.Context, channelID, callerUserID int64) error
	EmitCallDeclined(ctx context.Context, channelID, callerUserID, declinerUserID int64) error
}

// DmEmitterFromService wraps a dm.Service-shaped value into a DmEmitter.
func DmEmitterFromService(svc dmServiceLike) DmEmitter {
	return &dmEmitterAdapter{svc: svc}
}

type dmEmitterAdapter struct {
	svc dmServiceLike
}

func (a *dmEmitterAdapter) Started(ctx context.Context, channelID, actorUserID, startedAtMs int64) error {
	return a.svc.EmitCallStarted(ctx, channelID, actorUserID, startedAtMs)
}
func (a *dmEmitterAdapter) Ended(ctx context.Context, channelID, lastLeaverUserID, durationMs, startedAtMs int64) error {
	return a.svc.EmitCallEnded(ctx, channelID, lastLeaverUserID, durationMs, startedAtMs)
}
func (a *dmEmitterAdapter) Missed(ctx context.Context, channelID, callerUserID int64) error {
	return a.svc.EmitCallMissed(ctx, channelID, callerUserID)
}
func (a *dmEmitterAdapter) Declined(ctx context.Context, channelID, callerUserID, declinerUserID int64) error {
	return a.svc.EmitCallDeclined(ctx, channelID, callerUserID, declinerUserID)
}

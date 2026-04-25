package voice_test

import (
	"testing"

	"parley/internal/dm"
	"parley/internal/voice"
)

// TestDmServiceSatisfiesAdapter is a compile-time check: if dm.Service ever
// drifts from the dmServiceLike interface, the build will fail with a clear error here.
func TestDmServiceSatisfiesAdapter(t *testing.T) {
	var _ voice.DmEmitter = voice.DmEmitterFromService((*dm.Service)(nil))
}

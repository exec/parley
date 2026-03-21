package audit

import (
	"context"
	"log"
)

// AuditRepository is satisfied by *db.Repository.
type AuditRepository interface {
	CreateAuditLog(ctx context.Context, e Entry) error
}

// Entry is one audit log record.
type Entry struct {
	ServerID      int64
	ActorID       *int64 // nil = unknown/system
	ActorUsername string
	Action        string
	TargetID      string
	TargetType    string
	TargetName    string
	Changes       any    // marshalled to JSONB; nil = omit
	Reason        string
}

// AuditService writes audit log entries. Failures are logged and swallowed
// so they never break the primary action.
type AuditService struct {
	repo AuditRepository
}

func NewAuditService(repo AuditRepository) *AuditService {
	return &AuditService{repo: repo}
}

func (s *AuditService) Log(ctx context.Context, e Entry) {
	if err := s.repo.CreateAuditLog(ctx, e); err != nil {
		log.Printf("audit log write failed (action=%s): %v", e.Action, err)
	}
}

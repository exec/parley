package projects

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"sync"

	"parley/internal/db"
	"parley/internal/permissions"
	"parley/internal/synthesis"
	ws "parley/internal/websocket"
)

// Sentinel errors returned by Service methods.
var (
	ErrProjectNotFound = errors.New("project not found")
	ErrServerNotFound  = errors.New("server not found")
	ErrNotMember       = errors.New("not a server member")
	ErrForbidden       = errors.New("forbidden")
	ErrInvalidInput    = errors.New("invalid input")
	ErrPresetNotFound  = errors.New("preset not found")
)

// Allowed skill levels — mirrors the DB CHECK constraint.
var validSkillLevels = map[string]bool{
	"beginner":     true,
	"intermediate": true,
	"expert":       true,
	"auto":         true,
	"custom":       true,
}

// CreateInput carries the full create payload from handler to service.
type CreateInput struct {
	ServerID    int64
	Name        string
	Description string
	ClaudeMD    string
	SkillLevel  string
	PresetSlug  string // optional
	VCChannelID *int64 // optional
	Repos       []db.ProjectRepo
	Skills      []string
}

// UpdateInput carries the patch payload. Empty/nil means "leave alone";
// ClearPreset/ClearVC = true means "set to NULL".
type UpdateInput struct {
	Name        string
	Description string // pass "__SKIP__" to leave unchanged (allows clearing to "")
	SkillLevel  string
	PresetSlug  string // pass "" to leave alone, or "__CLEAR__" to null
	VCChannelID *int64 // nil + ClearVC=false means leave alone
	ClearVC     bool
	Repos       *[]db.ProjectRepo // nil = leave; non-nil = replace
	Skills      *[]string         // nil = leave; non-nil = replace
}

// Service provides project management operations with permission gates and
// WebSocket broadcasting.
type Service struct {
	mu        sync.RWMutex
	repo      *db.Repository
	hub       *ws.Hub
	synthesis *synthesis.Service
}

func NewService(repo *db.Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) SetHub(hub *ws.Hub) {
	s.mu.Lock()
	s.hub = hub
	s.mu.Unlock()
}

// SetSynthesis wires the synthesis service used by SynthesizeClaudeMD.
// Pass nil to disable synthesis (handler returns 503).
func (s *Service) SetSynthesis(svc *synthesis.Service) {
	s.mu.Lock()
	s.synthesis = svc
	s.mu.Unlock()
}

func (s *Service) broadcast(serverID int64, event string, data interface{}) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.hub == nil {
		return
	}
	payload, err := json.Marshal(data)
	if err != nil {
		log.Printf("projects.Service.broadcast: marshal error: %v", err)
		return
	}
	topic := "server:" + strconv.FormatInt(serverID, 10)
	s.hub.BroadcastToChannel(topic, event, payload)
}

// requireMember returns ErrNotMember if user is not a member of the server.
// Server owner is always considered a member.
func (s *Service) requireMember(ctx context.Context, serverID, userID int64) error {
	srv, err := s.repo.GetServerByID(ctx, serverID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return ErrServerNotFound
		}
		return err
	}
	if srv.OwnerID == userID {
		return nil
	}
	if _, err := s.repo.GetMember(ctx, serverID, userID); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return ErrNotMember
		}
		return err
	}
	return nil
}

// requireManageChannels returns ErrForbidden unless the user is the
// server owner or has PermManageChannels.
func (s *Service) requireManageChannels(ctx context.Context, serverID, userID int64) error {
	srv, err := s.repo.GetServerByID(ctx, serverID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return ErrServerNotFound
		}
		return err
	}
	if srv.OwnerID == userID {
		return nil
	}
	allowed, err := permissions.HasPermission(ctx, s.repo, serverID, userID, srv.OwnerID, permissions.PermManageChannels)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrForbidden
	}
	return nil
}

// CreateProject creates a project on a server. Caller must have ManageChannels
// (or be the server owner).
func (s *Service) CreateProject(ctx context.Context, userIDStr string, in CreateInput) (*db.Project, error) {
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		return nil, ErrInvalidInput
	}
	if in.Name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrInvalidInput)
	}
	if in.SkillLevel == "" {
		in.SkillLevel = "auto"
	}
	if !validSkillLevels[in.SkillLevel] {
		return nil, fmt.Errorf("%w: invalid skill_level", ErrInvalidInput)
	}

	if err := s.requireManageChannels(ctx, in.ServerID, userID); err != nil {
		return nil, err
	}

	var presetID *int64
	if in.PresetSlug != "" {
		preset, err := s.repo.GetProjectPresetBySlug(ctx, in.PresetSlug)
		if err != nil {
			if errors.Is(err, db.ErrNotFound) {
				return nil, ErrPresetNotFound
			}
			return nil, err
		}
		presetID = &preset.ID
	}

	p, err := s.repo.CreateProject(ctx, in.ServerID, userID, in.Name, in.Description, in.ClaudeMD, in.SkillLevel, presetID, in.VCChannelID)
	if err != nil {
		return nil, err
	}

	for _, r := range in.Repos {
		if r.Provider == "" || r.Owner == "" || r.Repo == "" {
			continue
		}
		if err := s.repo.AddProjectRepo(ctx, p.ID, r.Provider, r.Owner, r.Repo); err != nil {
			return nil, err
		}
	}

	if len(in.Skills) > 0 {
		if err := s.repo.SetProjectSkills(ctx, p.ID, in.Skills); err != nil {
			return nil, err
		}
	}

	full, err := s.repo.GetProject(ctx, p.ID)
	if err != nil {
		return nil, err
	}

	s.broadcast(full.ServerID, ws.EventProjectCreate, full)
	return full, nil
}

// GetProject returns the project if the caller is a server member.
func (s *Service) GetProject(ctx context.Context, projectIDStr, userIDStr string) (*db.Project, error) {
	projectID, err := strconv.ParseInt(projectIDStr, 10, 64)
	if err != nil {
		return nil, ErrInvalidInput
	}
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		return nil, ErrInvalidInput
	}
	p, err := s.repo.GetProject(ctx, projectID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil, ErrProjectNotFound
		}
		return nil, err
	}
	if err := s.requireMember(ctx, p.ServerID, userID); err != nil {
		// Hide existence from non-members.
		if errors.Is(err, ErrNotMember) || errors.Is(err, ErrServerNotFound) {
			return nil, ErrProjectNotFound
		}
		return nil, err
	}
	return p, nil
}

// ListServerProjects returns all projects for a server. Caller must be a member.
func (s *Service) ListServerProjects(ctx context.Context, serverIDStr, userIDStr string) ([]db.Project, error) {
	serverID, err := strconv.ParseInt(serverIDStr, 10, 64)
	if err != nil {
		return nil, ErrInvalidInput
	}
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		return nil, ErrInvalidInput
	}
	if err := s.requireMember(ctx, serverID, userID); err != nil {
		return nil, err
	}
	out, err := s.repo.GetServerProjects(ctx, serverID)
	if err != nil {
		return nil, err
	}
	if out == nil {
		out = []db.Project{}
	}
	return out, nil
}

// UpdateProject patches name/description/skill/preset/vc/repos/skills and
// re-broadcasts the full project.
func (s *Service) UpdateProject(ctx context.Context, projectIDStr, userIDStr string, in UpdateInput) (*db.Project, error) {
	projectID, err := strconv.ParseInt(projectIDStr, 10, 64)
	if err != nil {
		return nil, ErrInvalidInput
	}
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		return nil, ErrInvalidInput
	}
	ownerID, serverID, err := s.repo.GetProjectOwnerAndServer(ctx, projectID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil, ErrProjectNotFound
		}
		return nil, err
	}
	if userID != ownerID {
		if err := s.requireManageChannels(ctx, serverID, userID); err != nil {
			return nil, err
		}
	}

	if in.SkillLevel != "" && !validSkillLevels[in.SkillLevel] {
		return nil, fmt.Errorf("%w: invalid skill_level", ErrInvalidInput)
	}

	var presetID *int64
	clearPreset := false
	switch in.PresetSlug {
	case "":
		// leave alone
	case "__CLEAR__":
		clearPreset = true
	default:
		preset, err := s.repo.GetProjectPresetBySlug(ctx, in.PresetSlug)
		if err != nil {
			if errors.Is(err, db.ErrNotFound) {
				return nil, ErrPresetNotFound
			}
			return nil, err
		}
		presetID = &preset.ID
	}

	if _, err := s.repo.UpdateProject(ctx, projectID, in.Name, in.Description, in.SkillLevel, presetID, in.VCChannelID, clearPreset, in.ClearVC); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil, ErrProjectNotFound
		}
		return nil, err
	}

	if in.Repos != nil {
		// Replace: clear all then re-add. Cheap because cardinality is tiny.
		current, err := s.repo.GetProjectRepos(ctx, projectID)
		if err != nil {
			return nil, err
		}
		for _, r := range current {
			if err := s.repo.RemoveProjectRepo(ctx, projectID, r.Provider, r.Owner, r.Repo); err != nil {
				return nil, err
			}
		}
		for _, r := range *in.Repos {
			if r.Provider == "" || r.Owner == "" || r.Repo == "" {
				continue
			}
			if err := s.repo.AddProjectRepo(ctx, projectID, r.Provider, r.Owner, r.Repo); err != nil {
				return nil, err
			}
		}
	}
	if in.Skills != nil {
		if err := s.repo.SetProjectSkills(ctx, projectID, *in.Skills); err != nil {
			return nil, err
		}
	}

	full, err := s.repo.GetProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	s.broadcast(full.ServerID, ws.EventProjectUpdate, full)
	return full, nil
}

// UpdateClaudeMD overwrites the live CLAUDE.md and appends a new version row.
func (s *Service) UpdateClaudeMD(ctx context.Context, projectIDStr, userIDStr, content string) (*db.Project, int, error) {
	projectID, err := strconv.ParseInt(projectIDStr, 10, 64)
	if err != nil {
		return nil, 0, ErrInvalidInput
	}
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		return nil, 0, ErrInvalidInput
	}
	ownerID, serverID, err := s.repo.GetProjectOwnerAndServer(ctx, projectID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil, 0, ErrProjectNotFound
		}
		return nil, 0, err
	}
	if userID != ownerID {
		if err := s.requireManageChannels(ctx, serverID, userID); err != nil {
			return nil, 0, err
		}
	}

	if _, _, err := s.repo.UpdateProjectClaudeMD(ctx, projectID, content, userID); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil, 0, ErrProjectNotFound
		}
		return nil, 0, err
	}

	full, err := s.repo.GetProject(ctx, projectID)
	if err != nil {
		return nil, 0, err
	}
	s.broadcast(full.ServerID, ws.EventProjectUpdate, full)
	return full, full.VersionCount, nil
}

// DeleteProject removes a project. Owner or ManageChannels required.
func (s *Service) DeleteProject(ctx context.Context, projectIDStr, userIDStr string) error {
	projectID, err := strconv.ParseInt(projectIDStr, 10, 64)
	if err != nil {
		return ErrInvalidInput
	}
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		return ErrInvalidInput
	}
	ownerID, serverID, err := s.repo.GetProjectOwnerAndServer(ctx, projectID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return ErrProjectNotFound
		}
		return err
	}
	if userID != ownerID {
		if err := s.requireManageChannels(ctx, serverID, userID); err != nil {
			return err
		}
	}
	if err := s.repo.DeleteProject(ctx, projectID); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return ErrProjectNotFound
		}
		return err
	}
	s.broadcast(serverID, ws.EventProjectDelete, map[string]string{
		"id":        projectIDStr,
		"server_id": strconv.FormatInt(serverID, 10),
	})
	return nil
}

// SynthesizeInput is the API-layer request for SynthesizeClaudeMD.
type SynthesizeInput struct {
	ServerID    int64
	PresetSlug  string
	ProjectName string
	Description string
	SkillLevel  string
	Freeform    string
}

// SynthesizeResult is the API-layer response.
type SynthesizeResult struct {
	ClaudeMD string `json:"claude_md"`
	Provider string `json:"provider"`
}

// SynthesizeClaudeMD generates a CLAUDE.md from the user's intent. Gated
// the same as project create: caller must hold PermManageChannels on the
// target server (or be the owner). Returns ErrInvalidInput on bad inputs,
// synthesis.ErrProviderUnavailable when no synthesizer is configured.
func (s *Service) SynthesizeClaudeMD(ctx context.Context, userIDStr string, in SynthesizeInput) (*SynthesizeResult, error) {
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		return nil, ErrInvalidInput
	}
	if in.ProjectName == "" {
		return nil, fmt.Errorf("%w: name is required", ErrInvalidInput)
	}
	if in.SkillLevel == "" {
		in.SkillLevel = "auto"
	}
	if !validSkillLevels[in.SkillLevel] {
		return nil, fmt.Errorf("%w: invalid skill_level", ErrInvalidInput)
	}

	if err := s.requireManageChannels(ctx, in.ServerID, userID); err != nil {
		return nil, err
	}

	s.mu.RLock()
	syn := s.synthesis
	s.mu.RUnlock()
	if syn == nil {
		return nil, synthesis.ErrProviderUnavailable
	}

	// Resolve preset to its human-readable name + description (better signal
	// for the model than just the slug).
	var presetName, presetDesc string
	if in.PresetSlug != "" {
		preset, err := s.repo.GetProjectPresetBySlug(ctx, in.PresetSlug)
		if err != nil {
			if errors.Is(err, db.ErrNotFound) {
				return nil, ErrPresetNotFound
			}
			return nil, err
		}
		presetName = preset.Name
		presetDesc = preset.Description
	}

	out, err := syn.SynthesizeClaudeMD(ctx, synthesis.Input{
		PresetSlug:        in.PresetSlug,
		PresetName:        presetName,
		PresetDescription: presetDesc,
		SkillLevel:        in.SkillLevel,
		ProjectName:       in.ProjectName,
		Description:       in.Description,
		Freeform:          in.Freeform,
	})
	if err != nil {
		return nil, err
	}
	return &SynthesizeResult{ClaudeMD: out, Provider: syn.ProviderName()}, nil
}

// ListPresets returns all built-in presets. Auth-only — no perm gate.
func (s *Service) ListPresets(ctx context.Context) ([]db.ProjectPreset, error) {
	out, err := s.repo.ListProjectPresets(ctx)
	if err != nil {
		return nil, err
	}
	if out == nil {
		out = []db.ProjectPreset{}
	}
	return out, nil
}

// GetClaudeMDVersions returns the version history for a project's CLAUDE.md.
// Caller must be a server member.
func (s *Service) GetClaudeMDVersions(ctx context.Context, projectIDStr, userIDStr string) ([]db.ProjectClaudeMDVersion, error) {
	projectID, err := strconv.ParseInt(projectIDStr, 10, 64)
	if err != nil {
		return nil, ErrInvalidInput
	}
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		return nil, ErrInvalidInput
	}
	ownerID, serverID, err := s.repo.GetProjectOwnerAndServer(ctx, projectID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil, ErrProjectNotFound
		}
		return nil, err
	}
	_ = ownerID
	if err := s.requireMember(ctx, serverID, userID); err != nil {
		if errors.Is(err, ErrNotMember) || errors.Is(err, ErrServerNotFound) {
			return nil, ErrProjectNotFound
		}
		return nil, err
	}
	out, err := s.repo.GetProjectClaudeMDVersions(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if out == nil {
		out = []db.ProjectClaudeMDVersion{}
	}
	return out, nil
}

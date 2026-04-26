package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"strconv"

	"parley/internal/db"
	ws "parley/internal/websocket"
)

func (s *ServerService) CreateServer(ctx context.Context, name, iconURL string, ownerID string) (*Server, error) {
	if name == "" {
		return nil, errors.New("server name is required")
	}
	if ownerID == "" {
		return nil, errors.New("owner ID is required")
	}

	ownerIDInt, err := idToInt64(ownerID)
	if err != nil {
		return nil, errors.New("invalid owner ID format")
	}

	server := &db.Server{
		Name:    name,
		IconURL: nullString(iconURL),
		OwnerID: ownerIDInt,
	}
	if err = s.repo.CreateServer(ctx, server); err != nil {
		return nil, err
	}

	if err = s.repo.CreateEveryoneRole(ctx, server.ID); err != nil {
		return nil, err
	}

	member := &db.ServerMember{
		ServerID: server.ID,
		UserID:   ownerIDInt,
		Nickname: "",
	}
	if err = s.repo.AddMember(ctx, member); err != nil {
		return nil, err
	}

	return dbServerToService(server), nil
}

func (s *ServerService) GetServer(ctx context.Context, id string) (*Server, error) {
	if id == "" {
		return nil, errors.New("server ID is required")
	}

	serverID, err := idToInt64(id)
	if err != nil {
		return nil, errors.New("invalid server ID format")
	}

	server, err := s.repo.GetServerByID(ctx, serverID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil, ErrServerNotFound
		}
		return nil, err
	}

	return dbServerToService(server), nil
}

// ReorderUserServers persists per-user sidebar ordering for the given user
// and broadcasts USER_SERVERS_REORDER to all of that user's open clients so
// other tabs/apps update live. orderedServerIDs is first-to-last sidebar order.
func (s *ServerService) ReorderUserServers(ctx context.Context, userID string, orderedServerIDs []string) error {
	uID, err := idToInt64(userID)
	if err != nil {
		return errors.New("invalid user ID")
	}
	ids := make([]int64, 0, len(orderedServerIDs))
	for _, sid := range orderedServerIDs {
		n, err := idToInt64(sid)
		if err != nil {
			return errors.New("invalid server ID: " + sid)
		}
		ids = append(ids, n)
	}
	if err := s.repo.ReorderUserServers(ctx, uID, ids); err != nil {
		return err
	}
	if s.hub != nil {
		payload, err := json.Marshal(map[string]any{"server_ids": orderedServerIDs})
		if err == nil {
			_ = s.hub.SendToUser(userID, ws.EventUserServersReorder, payload)
		}
	}
	return nil
}

func (s *ServerService) GetUserServers(ctx context.Context, userID string) ([]*Server, error) {
	if userID == "" {
		return nil, errors.New("user ID is required")
	}

	userIDInt, err := idToInt64(userID)
	if err != nil {
		return nil, errors.New("invalid user ID format")
	}

	servers, err := s.repo.GetServersByUserID(ctx, userIDInt)
	if err != nil {
		return nil, err
	}

	result := make([]*Server, len(servers))
	for i, server := range servers {
		result[i] = dbServerToService(server)
	}
	return result, nil
}

func (s *ServerService) UpdateServer(ctx context.Context, id, name, iconURL, description string, isPublic bool) (*Server, error) {
	if id == "" {
		return nil, errors.New("server ID is required")
	}
	if name == "" {
		return nil, errors.New("server name is required")
	}
	if len(description) > 200 {
		return nil, errors.New("description must be 200 characters or fewer")
	}

	serverID, err := idToInt64(id)
	if err != nil {
		return nil, errors.New("invalid server ID format")
	}

	server, err := s.repo.GetServerByID(ctx, serverID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil, ErrServerNotFound
		}
		return nil, err
	}

	// is_public requires a vanity URL
	if isPublic && !server.VanityURL.Valid {
		return nil, errors.New("a vanity URL is required to list your server publicly")
	}

	server.Name = name
	server.IconURL = nullString(iconURL)
	server.Description = sql.NullString{String: description, Valid: description != ""}
	server.IsPublic = isPublic

	if err = s.repo.UpdateServer(ctx, server); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil, ErrServerNotFound
		}
		return nil, err
	}

	srv := dbServerToService(server)
	if s.hub != nil {
		if payload, err := json.Marshal(srv); err == nil {
			s.hub.BroadcastToChannel("server:"+id, ws.EventServerUpdate, payload)
		}
	}
	return srv, nil
}

func (s *ServerService) DeleteServer(ctx context.Context, id string) error {
	if id == "" {
		return errors.New("server ID is required")
	}

	serverID, err := idToInt64(id)
	if err != nil {
		return errors.New("invalid server ID format")
	}

	if err = s.repo.DeleteServer(ctx, serverID); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return ErrServerNotFound
		}
		return err
	}

	if s.hub != nil {
		payload, err := json.Marshal(map[string]string{"server_id": id})
		if err != nil {
			log.Printf("Failed to marshal server delete event: %v", err)
		} else {
			s.hub.BroadcastToChannel("server:"+id, ws.EventServerDelete, payload)
		}
	}
	return nil
}

func (s *ServerService) GetServerByVanityURL(ctx context.Context, vanityURL string) (*Server, error) {
	srv, err := s.repo.GetServerByVanityURL(ctx, vanityURL)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil, ErrServerNotFound
		}
		return nil, err
	}
	return dbServerToService(srv), nil
}

// ListServerCategories returns all admin-managed server categories.
func (s *ServerService) ListServerCategories(ctx context.Context) ([]db.ServerCategory, error) {
	cats, err := s.repo.GetServerCategories(ctx)
	if err != nil {
		return nil, err
	}
	if cats == nil {
		cats = []db.ServerCategory{}
	}
	return cats, nil
}

// SetServerCategories replaces the category assignments for a server.
// Authorization (owner check) is handled at the handler layer.
// Returns the list of category IDs assigned BEFORE the update along with the
// new full ServerCategory rows AFTER the update so the caller can audit-log
// the diff.
func (s *ServerService) SetServerCategories(ctx context.Context, serverID string, categoryIDs []int64) ([]int64, []db.ServerCategory, error) {
	if len(categoryIDs) > 3 {
		return nil, nil, errors.New("maximum 3 categories allowed")
	}
	id, err := idToInt64(serverID)
	if err != nil {
		return nil, nil, errors.New("invalid server ID")
	}
	beforeCats, err := s.repo.GetServerCategoryAssignments(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	beforeIDs := make([]int64, 0, len(beforeCats))
	for _, c := range beforeCats {
		beforeIDs = append(beforeIDs, c.ID)
	}
	if err := s.repo.SetServerCategories(ctx, id, categoryIDs); err != nil {
		return nil, nil, err
	}
	afterCats, err := s.repo.GetServerCategoryAssignments(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	if afterCats == nil {
		afterCats = []db.ServerCategory{}
	}
	return beforeIDs, afterCats, nil
}

// GetServerCategoryAssignments returns the categories assigned to a specific server.
func (s *ServerService) GetServerCategoryAssignments(ctx context.Context, serverID string) ([]db.ServerCategory, error) {
	id, err := idToInt64(serverID)
	if err != nil {
		return nil, errors.New("invalid server ID")
	}
	cats, err := s.repo.GetServerCategoryAssignments(ctx, id)
	if err != nil {
		return nil, err
	}
	if cats == nil {
		cats = []db.ServerCategory{}
	}
	return cats, nil
}

const discoverPageSize = 24

// Discover returns paginated public servers.
func (s *ServerService) Discover(ctx context.Context, categoryID *int64, q string, page int) ([]PublicServer, int, error) {
	if page < 1 {
		page = 1
	}
	if len(q) > 100 {
		q = q[:100]
	}
	offset := (page - 1) * discoverPageSize

	rows, total, err := s.repo.GetPublicServers(ctx, categoryID, q, discoverPageSize, offset)
	if err != nil {
		return nil, 0, err
	}

	serverIDs := make([]int64, len(rows))
	for i, row := range rows {
		serverIDs[i] = row.ID
	}
	catsByServer, err := s.repo.GetBulkServerCategoryAssignments(ctx, serverIDs)
	if err != nil {
		return nil, 0, err
	}

	servers := make([]PublicServer, 0, len(rows))
	for _, row := range rows {
		id := strconv.FormatInt(row.ID, 10)
		cats := catsByServer[row.ID]
		if cats == nil {
			cats = []db.ServerCategory{}
		}
		servers = append(servers, PublicServer{
			ID:          id,
			Name:        row.Name,
			IconURL:     row.IconURL.String,
			VanityURL:   row.VanityURL.String,
			Description: row.Description.String,
			MemberCount: row.MemberCount,
			Categories:  cats,
		})
	}
	return servers, total, nil
}

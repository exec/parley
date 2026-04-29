package db

import (
	"context"
	"database/sql"
)

// CreateProject inserts a new project and version 1 of its CLAUDE.md in
// the same transaction. presetID and vcChannelID are nil-safe; pass nil
// to leave the column NULL.
func (r *Repository) CreateProject(ctx context.Context, serverID, ownerUserID int64, name, description, claudeMD, skillLevel string, presetID, vcChannelID *int64) (*Project, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck

	var p Project
	err = tx.QueryRowContext(ctx,
		`INSERT INTO projects (server_id, name, description, claude_md, skill_level, preset_id, vc_channel_id, owner_user_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 RETURNING id, server_id, name, description, claude_md, skill_level, preset_id, vc_channel_id, owner_user_id, created_at, updated_at`,
		serverID, name, description, claudeMD, skillLevel, presetID, vcChannelID, ownerUserID,
	).Scan(
		&p.ID, &p.ServerID, &p.Name, &p.Description, &p.ClaudeMD,
		&p.SkillLevel, &p.PresetID, &p.VCChannelID, &p.OwnerUserID,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO project_claude_md_versions (project_id, version, content, edited_by)
		 VALUES ($1, 1, $2, $3)`,
		p.ID, claudeMD, ownerUserID,
	)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &p, nil
}

// GetProject fetches a project by ID with computed owner/preset/repos/skills.
func (r *Repository) GetProject(ctx context.Context, projectID int64) (*Project, error) {
	var p Project
	var presetSlug, presetName sql.NullString
	err := r.db.QueryRowContext(ctx, `
		SELECT
			p.id, p.server_id, p.name, p.description, p.claude_md,
			p.skill_level, p.preset_id, p.vc_channel_id, p.owner_user_id,
			p.created_at, p.updated_at,
			u.username, COALESCE(u.avatar_url, ''),
			pp.slug, pp.name,
			(SELECT COUNT(*) FROM project_claude_md_versions WHERE project_id = p.id) AS version_count
		FROM projects p
		JOIN users u ON u.id = p.owner_user_id
		LEFT JOIN project_presets pp ON pp.id = p.preset_id
		WHERE p.id = $1
	`, projectID).Scan(
		&p.ID, &p.ServerID, &p.Name, &p.Description, &p.ClaudeMD,
		&p.SkillLevel, &p.PresetID, &p.VCChannelID, &p.OwnerUserID,
		&p.CreatedAt, &p.UpdatedAt,
		&p.OwnerUsername, &p.OwnerAvatarURL,
		&presetSlug, &presetName,
		&p.VersionCount,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	p.PresetSlug = presetSlug.String
	p.PresetName = presetName.String

	repos, err := r.GetProjectRepos(ctx, projectID)
	if err != nil {
		return nil, err
	}
	p.Repos = repos

	skills, err := r.GetProjectSkills(ctx, projectID)
	if err != nil {
		return nil, err
	}
	p.Skills = skills

	return &p, nil
}

// GetServerProjects lists all projects in a server, newest first. Owner
// avatar/username and preset metadata are joined in; repos/skills are not
// loaded here (list view doesn't need them).
func (r *Repository) GetServerProjects(ctx context.Context, serverID int64) ([]Project, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT
			p.id, p.server_id, p.name, p.description, p.claude_md,
			p.skill_level, p.preset_id, p.vc_channel_id, p.owner_user_id,
			p.created_at, p.updated_at,
			u.username, COALESCE(u.avatar_url, ''),
			pp.slug, pp.name,
			(SELECT COUNT(*) FROM project_claude_md_versions WHERE project_id = p.id) AS version_count
		FROM projects p
		JOIN users u ON u.id = p.owner_user_id
		LEFT JOIN project_presets pp ON pp.id = p.preset_id
		WHERE p.server_id = $1
		ORDER BY p.created_at DESC
	`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Project
	for rows.Next() {
		var p Project
		var presetSlug, presetName sql.NullString
		if err := rows.Scan(
			&p.ID, &p.ServerID, &p.Name, &p.Description, &p.ClaudeMD,
			&p.SkillLevel, &p.PresetID, &p.VCChannelID, &p.OwnerUserID,
			&p.CreatedAt, &p.UpdatedAt,
			&p.OwnerUsername, &p.OwnerAvatarURL,
			&presetSlug, &presetName,
			&p.VersionCount,
		); err != nil {
			return nil, err
		}
		p.PresetSlug = presetSlug.String
		p.PresetName = presetName.String
		out = append(out, p)
	}
	return out, rows.Err()
}

// UpdateProject patches mutable project fields. Pass nil for presetID or
// vcChannelID to clear them; pass empty strings for name/description/skill
// to leave them unchanged via COALESCE on the empty-marker side. The
// caller (service.go) is expected to validate inputs before calling.
func (r *Repository) UpdateProject(ctx context.Context, projectID int64, name, description, skillLevel string, presetID, vcChannelID *int64, clearPreset, clearVC bool) (*Project, error) {
	var p Project
	err := r.db.QueryRowContext(ctx, `
		UPDATE projects SET
			name          = COALESCE(NULLIF($1, ''), name),
			description   = COALESCE(NULLIF($2, '__SKIP__'), description),
			skill_level   = COALESCE(NULLIF($3, ''), skill_level),
			preset_id     = CASE WHEN $4::BOOLEAN THEN NULL ELSE COALESCE($5::INT, preset_id) END,
			vc_channel_id = CASE WHEN $6::BOOLEAN THEN NULL ELSE COALESCE($7::BIGINT, vc_channel_id) END,
			updated_at    = NOW()
		WHERE id = $8
		RETURNING id, server_id, name, description, claude_md, skill_level, preset_id, vc_channel_id, owner_user_id, created_at, updated_at
	`, name, description, skillLevel, clearPreset, presetID, clearVC, vcChannelID, projectID).Scan(
		&p.ID, &p.ServerID, &p.Name, &p.Description, &p.ClaudeMD,
		&p.SkillLevel, &p.PresetID, &p.VCChannelID, &p.OwnerUserID,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// UpdateProjectClaudeMD overwrites the live CLAUDE.md and appends a new
// version row in one transaction. Returns the new version number.
func (r *Repository) UpdateProjectClaudeMD(ctx context.Context, projectID int64, content string, editedBy int64) (*Project, int, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback() //nolint:errcheck

	var latest int
	err = tx.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(version), 0) FROM project_claude_md_versions WHERE project_id = $1`,
		projectID,
	).Scan(&latest)
	if err != nil {
		return nil, 0, err
	}
	newVersion := latest + 1

	var p Project
	err = tx.QueryRowContext(ctx,
		`UPDATE projects SET claude_md = $1, updated_at = NOW() WHERE id = $2
		 RETURNING id, server_id, name, description, claude_md, skill_level, preset_id, vc_channel_id, owner_user_id, created_at, updated_at`,
		content, projectID,
	).Scan(
		&p.ID, &p.ServerID, &p.Name, &p.Description, &p.ClaudeMD,
		&p.SkillLevel, &p.PresetID, &p.VCChannelID, &p.OwnerUserID,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, 0, ErrNotFound
	}
	if err != nil {
		return nil, 0, err
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO project_claude_md_versions (project_id, version, content, edited_by)
		 VALUES ($1, $2, $3, $4)`,
		projectID, newVersion, content, editedBy,
	)
	if err != nil {
		return nil, 0, err
	}

	if err := tx.Commit(); err != nil {
		return nil, 0, err
	}
	return &p, newVersion, nil
}

// DeleteProject removes a project. Cascades clean repos/skills/versions.
func (r *Repository) DeleteProject(ctx context.Context, projectID int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM projects WHERE id = $1`, projectID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// GetProjectOwnerAndServer returns ownerUserID + serverID for a project.
// Used by service.go to decide perm gates without a full GetProject load.
func (r *Repository) GetProjectOwnerAndServer(ctx context.Context, projectID int64) (ownerUserID, serverID int64, err error) {
	err = r.db.QueryRowContext(ctx,
		`SELECT owner_user_id, server_id FROM projects WHERE id = $1`, projectID,
	).Scan(&ownerUserID, &serverID)
	if err == sql.ErrNoRows {
		return 0, 0, ErrNotFound
	}
	return ownerUserID, serverID, err
}

// ---- Repos ----

// GetProjectRepos returns the linked external repos for a project.
func (r *Repository) GetProjectRepos(ctx context.Context, projectID int64) ([]ProjectRepo, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT project_id, provider, owner, repo FROM project_repos WHERE project_id = $1`,
		projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ProjectRepo
	for rows.Next() {
		var pr ProjectRepo
		if err := rows.Scan(&pr.ProjectID, &pr.Provider, &pr.Owner, &pr.Repo); err != nil {
			return nil, err
		}
		out = append(out, pr)
	}
	return out, rows.Err()
}

// AddProjectRepo links an external repo to a project. Idempotent on the
// composite primary key.
func (r *Repository) AddProjectRepo(ctx context.Context, projectID int64, provider, owner, repo string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO project_repos (project_id, provider, owner, repo)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT DO NOTHING`,
		projectID, provider, owner, repo,
	)
	return err
}

// RemoveProjectRepo unlinks an external repo from a project.
func (r *Repository) RemoveProjectRepo(ctx context.Context, projectID int64, provider, owner, repo string) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM project_repos WHERE project_id = $1 AND provider = $2 AND owner = $3 AND repo = $4`,
		projectID, provider, owner, repo,
	)
	return err
}

// ---- Skills ----

// GetProjectSkills returns the slug list of skills attached to a project.
func (r *Repository) GetProjectSkills(ctx context.Context, projectID int64) ([]string, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT skill_id FROM project_skills WHERE project_id = $1 ORDER BY skill_id`,
		projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// SetProjectSkills replaces the skill set for a project atomically.
func (r *Repository) SetProjectSkills(ctx context.Context, projectID int64, skills []string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.ExecContext(ctx, `DELETE FROM project_skills WHERE project_id = $1`, projectID); err != nil {
		return err
	}
	for _, s := range skills {
		if s == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO project_skills (project_id, skill_id) VALUES ($1, $2)
			 ON CONFLICT DO NOTHING`,
			projectID, s,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ---- Presets ----

// ListProjectPresets returns all built-in presets sorted by id (insertion order).
func (r *Repository) ListProjectPresets(ctx context.Context) ([]ProjectPreset, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, slug, name, description, is_builtin FROM project_presets ORDER BY id`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ProjectPreset
	for rows.Next() {
		var p ProjectPreset
		if err := rows.Scan(&p.ID, &p.Slug, &p.Name, &p.Description, &p.IsBuiltin); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// GetProjectPresetBySlug fetches a preset by slug. Returns ErrNotFound if absent.
func (r *Repository) GetProjectPresetBySlug(ctx context.Context, slug string) (*ProjectPreset, error) {
	var p ProjectPreset
	err := r.db.QueryRowContext(ctx,
		`SELECT id, slug, name, description, is_builtin FROM project_presets WHERE slug = $1`,
		slug,
	).Scan(&p.ID, &p.Slug, &p.Name, &p.Description, &p.IsBuiltin)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// ---- CLAUDE.md versions ----

// GetProjectClaudeMDVersions returns all versions for a project, newest first.
func (r *Repository) GetProjectClaudeMDVersions(ctx context.Context, projectID int64) ([]ProjectClaudeMDVersion, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, project_id, version, content, edited_by, created_at
		 FROM project_claude_md_versions WHERE project_id = $1
		 ORDER BY version DESC`,
		projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ProjectClaudeMDVersion
	for rows.Next() {
		var v ProjectClaudeMDVersion
		if err := rows.Scan(&v.ID, &v.ProjectID, &v.Version, &v.Content, &v.EditedBy, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

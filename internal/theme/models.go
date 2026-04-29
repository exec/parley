package theme

import "time"

type UserTheme struct {
	ID                int64     `json:"id"`
	UserID            int64     `json:"-"`
	Name              string    `json:"name"`
	CSS               string    `json:"css"`
	BaseTheme         string    `json:"base_theme"`
	BackgroundURL     *string   `json:"background_url"`
	ShareToken        *string   `json:"share_token"`
	SourceShareToken  *string   `json:"source_share_token,omitempty"`
	IsPublished       bool      `json:"is_published"`
	IsFeatured        bool      `json:"is_featured"`
	AuthorUsername    string    `json:"author_username,omitempty"`
	AuthorDisplayName string    `json:"author_display_name,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
}

type ThemeRepoResponse struct {
	Themes []UserTheme `json:"themes"`
	Total  int         `json:"total"`
}

type UserPreferences struct {
	ActiveTheme         string      `json:"active_theme"`
	ActiveCustomThemeID *int        `json:"active_custom_theme_id"`
	BetaFeatures        bool        `json:"beta_features"`
	CustomThemes        []UserTheme `json:"custom_themes"`
}

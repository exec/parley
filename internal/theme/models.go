package theme

import "time"

type UserTheme struct {
	ID             int64     `json:"id"`
	UserID         int64     `json:"-"`
	Name           string    `json:"name"`
	CSS            string    `json:"css"`
	BaseTheme      string    `json:"base_theme"`
	BackgroundURL  *string   `json:"background_url"`
	ShareToken     *string   `json:"share_token"`
	AuthorUsername string    `json:"author_username,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

type UserPreferences struct {
	ActiveTheme         string      `json:"active_theme"`
	ActiveCustomThemeID *int        `json:"active_custom_theme_id"`
	CustomThemes        []UserTheme `json:"custom_themes"`
}

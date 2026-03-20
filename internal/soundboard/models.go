package soundboard

import "time"

// Sound is a single soundboard entry.
type Sound struct {
	ID         int64     `json:"id,string"`
	ServerID   int64     `json:"server_id,string"`
	UploaderID int64     `json:"uploader_id,string"`
	Name       string    `json:"name"`
	Emoji      string    `json:"emoji,omitempty"`
	FileURL    string    `json:"file_url"`
	FileKey    string    `json:"-"` // never expose the storage key to clients
	CreatedAt  time.Time `json:"created_at"`
}

// SoundWithServer adds the server name for cross-server listing.
type SoundWithServer struct {
	Sound
	ServerName string `json:"server_name"`
}

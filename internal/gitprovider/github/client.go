// Package github implements gitprovider.Provider for github.com.
//
// Auth model (V1, public repos only):
//
//   - We hold a parley-owned GitHub App. The App is installed on a single
//     account/org we control; the installation is just a credential vehicle.
//   - On startup we validate the App private key and record the App ID and
//     Installation ID. No network call yet.
//   - On first request we mint a short-lived JWT signed with the App key,
//     exchange it for a 1-hour installation access token, and cache that
//     token in memory. Refreshed at 50 minutes.
//   - All subsequent /repos/* calls use the installation token.
//   - When config is missing, the client falls back to unauthenticated
//     api.github.com calls (60/hr/IP). Logged once at startup so dev
//     environments stay frictionless.
package github

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/golang-jwt/jwt/v5"

	"parley/internal/gitprovider"
)

const (
	apiBaseURL    = "https://api.github.com"
	userAgent     = "parley/1.x (+https://parley.byexec.com)"
	httpTimeout   = 10 * time.Second
	jwtTTL        = 9 * time.Minute  // GitHub allows up to 10; leave clock-skew margin
	installTokenRefreshAt = 50 * time.Minute // installation tokens live 1h
)

// Config carries the GitHub App credentials. Any zero field disables
// authentication; the client falls back to unauthenticated calls.
type Config struct {
	AppID          int64           // numeric GitHub App ID
	InstallationID int64           // ID of the parley installation we use
	PrivateKey     *rsa.PrivateKey // App private key (RSA)
}

// LoadConfigFromEnv reads GITHUB_APP_ID, GITHUB_APP_INSTALLATION_ID, and
// either GITHUB_APP_PRIVATE_KEY (PEM contents) or GITHUB_APP_PRIVATE_KEY_PATH
// (file path). Returns a zero Config (no error) if all three are unset, so
// dev environments can run unauthenticated. Returns an error only when
// config is partially set or malformed.
func LoadConfigFromEnv(getenv func(string) string, readFile func(string) ([]byte, error)) (Config, error) {
	idStr := getenv("GITHUB_APP_ID")
	instStr := getenv("GITHUB_APP_INSTALLATION_ID")
	keyStr := getenv("GITHUB_APP_PRIVATE_KEY")
	keyPath := getenv("GITHUB_APP_PRIVATE_KEY_PATH")

	// All unset → fully unauth mode. Not an error.
	if idStr == "" && instStr == "" && keyStr == "" && keyPath == "" {
		return Config{}, nil
	}

	if idStr == "" || instStr == "" || (keyStr == "" && keyPath == "") {
		return Config{}, errors.New("github: partial App config — set all of GITHUB_APP_ID, GITHUB_APP_INSTALLATION_ID, and GITHUB_APP_PRIVATE_KEY[_PATH] (or none)")
	}

	var appID, instID int64
	if _, err := fmt.Sscan(idStr, &appID); err != nil || appID <= 0 {
		return Config{}, fmt.Errorf("github: invalid GITHUB_APP_ID %q", idStr)
	}
	if _, err := fmt.Sscan(instStr, &instID); err != nil || instID <= 0 {
		return Config{}, fmt.Errorf("github: invalid GITHUB_APP_INSTALLATION_ID %q", instStr)
	}

	var pemBytes []byte
	if keyStr != "" {
		// Allow either a literal multi-line PEM or one with `\n` escapes.
		pemBytes = []byte(strings.ReplaceAll(keyStr, `\n`, "\n"))
	} else {
		b, err := readFile(keyPath)
		if err != nil {
			return Config{}, fmt.Errorf("github: read private key %q: %w", keyPath, err)
		}
		pemBytes = b
	}

	priv, err := jwt.ParseRSAPrivateKeyFromPEM(pemBytes)
	if err != nil {
		return Config{}, fmt.Errorf("github: parse private key: %w", err)
	}
	return Config{AppID: appID, InstallationID: instID, PrivateKey: priv}, nil
}

// Client is the GitHub-backed gitprovider.Provider implementation.
type Client struct {
	cfg  Config
	http *http.Client

	// Installation token cache. Held under mu.
	mu          sync.Mutex
	tokenValue  string
	tokenExpiry time.Time
}

// NewClient builds a Client. cfg may be zero-valued (unauthenticated mode).
func NewClient(cfg Config) *Client {
	c := &Client{
		cfg:  cfg,
		http: &http.Client{Timeout: httpTimeout},
	}
	if cfg.PrivateKey == nil {
		log.Printf("gitprovider/github: no GitHub App credentials — running unauthenticated (60/hr/IP). Set GITHUB_APP_ID, GITHUB_APP_INSTALLATION_ID, GITHUB_APP_PRIVATE_KEY_PATH to enable 5000/hr.")
	}
	return c
}

// Name implements gitprovider.Provider.
func (c *Client) Name() string { return "github" }

// installationToken returns a valid installation token, minting/refreshing
// as needed. Empty string + nil error means unauthenticated mode.
func (c *Client) installationToken(ctx context.Context) (string, error) {
	if c.cfg.PrivateKey == nil {
		return "", nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.tokenValue != "" && time.Now().Before(c.tokenExpiry) {
		return c.tokenValue, nil
	}

	// Mint App JWT.
	now := time.Now()
	claims := jwt.RegisteredClaims{
		Issuer:    fmt.Sprintf("%d", c.cfg.AppID),
		IssuedAt:  jwt.NewNumericDate(now.Add(-30 * time.Second)), // clock-skew margin
		ExpiresAt: jwt.NewNumericDate(now.Add(jwtTTL)),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	signed, err := tok.SignedString(c.cfg.PrivateKey)
	if err != nil {
		return "", fmt.Errorf("sign App JWT: %w", err)
	}

	// Exchange for installation token.
	url := fmt.Sprintf("%s/app/installations/%d/access_tokens", apiBaseURL, c.cfg.InstallationID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+signed)
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("exchange installation token: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", fmt.Errorf("installation token: %s: %s", resp.Status, string(body))
	}
	var out struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("decode installation token: %w", err)
	}
	c.tokenValue = out.Token
	// Refresh slightly before actual expiry; clamp to 50min if GH ever returns longer.
	exp := out.ExpiresAt
	maxExp := now.Add(installTokenRefreshAt)
	if exp.After(maxExp) {
		exp = maxExp
	}
	c.tokenExpiry = exp
	return c.tokenValue, nil
}

// do performs an authenticated GET. Returns the parsed JSON body via dst, or
// a typed error (gitprovider.ErrNotFound, gitprovider.ErrRateLimited).
func (c *Client) do(ctx context.Context, path string, query url.Values, dst any) error {
	tok, err := c.installationToken(ctx)
	if err != nil {
		return err
	}

	full := apiBaseURL + path
	if len(query) > 0 {
		full += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, full, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if tok != "" {
		req.Header.Set("Authorization", "token "+tok)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		if dst == nil {
			return nil
		}
		return json.NewDecoder(resp.Body).Decode(dst)
	case http.StatusNotFound:
		return gitprovider.ErrNotFound
	case http.StatusForbidden, http.StatusTooManyRequests:
		// 403 with x-ratelimit-remaining=0 is rate limiting. 403 without that
		// is auth/permission — treat both as rate-limited from the embed
		// degradation perspective.
		return gitprovider.ErrRateLimited
	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("github %s: %s: %s", path, resp.Status, string(body))
	}
}

// --- gitprovider.Provider implementation ---

type apiRepo struct {
	Name        string `json:"name"`
	FullName    string `json:"full_name"`
	Description string `json:"description"`
	Private     bool   `json:"private"`
	HTMLURL     string `json:"html_url"`
	Owner       struct {
		Login     string `json:"login"`
		AvatarURL string `json:"avatar_url"`
	} `json:"owner"`
	DefaultBranch   string    `json:"default_branch"`
	Language        string    `json:"language"`
	StargazersCount int       `json:"stargazers_count"`
	ForksCount      int       `json:"forks_count"`
	PushedAt        time.Time `json:"pushed_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func (c *Client) GetRepo(ctx context.Context, owner, repo string) (*gitprovider.Repo, error) {
	if err := gitprovider.ValidateOwnerRepo(owner, repo); err != nil {
		return nil, err
	}
	var ar apiRepo
	if err := c.do(ctx, "/repos/"+owner+"/"+repo, nil, &ar); err != nil {
		return nil, err
	}
	return &gitprovider.Repo{
		Owner:          ar.Owner.Login,
		Name:           ar.Name,
		Description:    ar.Description,
		DefaultBranch:  ar.DefaultBranch,
		Language:       ar.Language,
		OwnerAvatarURL: ar.Owner.AvatarURL,
		HTMLURL:        ar.HTMLURL,
		Stars:          ar.StargazersCount,
		Forks:          ar.ForksCount,
		Private:        ar.Private,
		PushedAt:       ar.PushedAt,
		UpdatedAt:      ar.UpdatedAt,
	}, nil
}

// contentsItem covers both directory entries (when fetching a dir) and
// single-file responses (when fetching a file). GitHub's /repos/:o/:r/contents/:p
// returns an array for dirs, an object for files; we decode into one shape.
type contentsItem struct {
	Type        string `json:"type"`     // "file" | "dir" | "symlink" | "submodule"
	Name        string `json:"name"`
	Path        string `json:"path"`
	SHA         string `json:"sha"`
	Size        int64  `json:"size"`
	HTMLURL     string `json:"html_url"`
	Encoding    string `json:"encoding"` // "base64" for files
	Content     string `json:"content"`  // base64-encoded for files
}

func (c *Client) GetTree(ctx context.Context, owner, repo, ref, path string) ([]gitprovider.TreeEntry, error) {
	if err := gitprovider.ValidateOwnerRepo(owner, repo); err != nil {
		return nil, err
	}
	q := url.Values{}
	if ref != "" {
		q.Set("ref", ref)
	}
	// Strip leading slash on path so GitHub's contents endpoint accepts it.
	cleanPath := strings.TrimPrefix(path, "/")
	endpoint := "/repos/" + owner + "/" + repo + "/contents/" + cleanPath

	// GitHub returns either a JSON object (file) or an array (dir). Try array
	// first; fall back to single-object decode.
	body, err := c.raw(ctx, endpoint, q)
	if err != nil {
		return nil, err
	}
	var items []contentsItem
	if err := json.Unmarshal(body, &items); err != nil {
		// Not an array — single-file response means caller asked for a file path.
		return nil, fmt.Errorf("%w: path %q is a file, not a directory", gitprovider.ErrInvalidArg, path)
	}
	out := make([]gitprovider.TreeEntry, 0, len(items))
	for _, it := range items {
		out = append(out, gitprovider.TreeEntry{
			Path: it.Path,
			Name: it.Name,
			Type: it.Type,
			Size: it.Size,
			SHA:  it.SHA,
		})
	}
	return out, nil
}

func (c *Client) GetBlob(ctx context.Context, owner, repo, ref, path string) (*gitprovider.Blob, error) {
	if err := gitprovider.ValidateOwnerRepo(owner, repo); err != nil {
		return nil, err
	}
	q := url.Values{}
	if ref != "" {
		q.Set("ref", ref)
	}
	cleanPath := strings.TrimPrefix(path, "/")
	endpoint := "/repos/" + owner + "/" + repo + "/contents/" + cleanPath

	body, err := c.raw(ctx, endpoint, q)
	if err != nil {
		return nil, err
	}
	var item contentsItem
	if err := json.Unmarshal(body, &item); err != nil {
		// If we got an array, the path is a directory.
		return nil, fmt.Errorf("%w: path %q is a directory", gitprovider.ErrInvalidArg, path)
	}
	if item.Type != "file" {
		return nil, fmt.Errorf("%w: path %q is %s, not a file", gitprovider.ErrInvalidArg, path, item.Type)
	}

	blob := &gitprovider.Blob{
		Path:    item.Path,
		SHA:     item.SHA,
		Size:    item.Size,
		HTMLURL: item.HTMLURL,
	}

	// Files larger than the cap: return metadata only.
	if item.Size > gitprovider.MaxBlobBytes {
		blob.Truncated = true
		blob.ContentType = "text" // unknown — frontend shows "too large" banner regardless
		return blob, nil
	}

	// GitHub may still return empty content for large files even under the cap;
	// fall back to the raw blob endpoint by SHA in that case.
	var raw []byte
	if item.Encoding == "base64" && item.Content != "" {
		decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(item.Content, "\n", ""))
		if err != nil {
			return nil, fmt.Errorf("decode blob: %w", err)
		}
		raw = decoded
	} else if item.SHA != "" {
		// Files >1MB at the contents endpoint are returned without content;
		// fetch via /git/blobs/{sha}.
		var blobResp struct {
			Content  string `json:"content"`
			Encoding string `json:"encoding"`
		}
		if err := c.do(ctx, "/repos/"+owner+"/"+repo+"/git/blobs/"+item.SHA, nil, &blobResp); err != nil {
			return nil, err
		}
		if blobResp.Encoding == "base64" {
			decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(blobResp.Content, "\n", ""))
			if err != nil {
				return nil, fmt.Errorf("decode blob: %w", err)
			}
			raw = decoded
		}
	}

	if isBinary(raw) {
		blob.ContentType = "binary"
		return blob, nil
	}
	blob.ContentType = "text"
	blob.Content = raw
	return blob, nil
}

type apiRelease struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	Body        string    `json:"body"`
	HTMLURL     string    `json:"html_url"`
	PublishedAt time.Time `json:"published_at"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
}

func (c *Client) ListReleases(ctx context.Context, owner, repo string, limit int) ([]gitprovider.Release, error) {
	if err := gitprovider.ValidateOwnerRepo(owner, repo); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 30 {
		limit = 5
	}
	q := url.Values{}
	q.Set("per_page", fmt.Sprintf("%d", limit))
	var arr []apiRelease
	if err := c.do(ctx, "/repos/"+owner+"/"+repo+"/releases", q, &arr); err != nil {
		return nil, err
	}
	out := make([]gitprovider.Release, 0, len(arr))
	for _, r := range arr {
		if r.Draft {
			continue // never expose drafts via parley embeds
		}
		out = append(out, gitprovider.Release{
			TagName:     r.TagName,
			Name:        r.Name,
			Body:        r.Body,
			HTMLURL:     r.HTMLURL,
			PublishedAt: r.PublishedAt,
		})
	}
	return out, nil
}

// raw executes a GET and returns the raw response body. Used for endpoints
// whose shape is polymorphic (contents = file-or-dir).
func (c *Client) raw(ctx context.Context, path string, query url.Values) ([]byte, error) {
	tok, err := c.installationToken(ctx)
	if err != nil {
		return nil, err
	}
	full := apiBaseURL + path
	if len(query) > 0 {
		full += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, full, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if tok != "" {
		req.Header.Set("Authorization", "token "+tok)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		// Cap raw bodies at 8 MiB — GitHub responses for our use case stay well
		// below this and a runaway response shouldn't OOM the process.
		return io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	case http.StatusNotFound:
		return nil, gitprovider.ErrNotFound
	case http.StatusForbidden, http.StatusTooManyRequests:
		return nil, gitprovider.ErrRateLimited
	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("github %s: %s: %s", path, resp.Status, string(body))
	}
}

// isBinary returns true if b looks like a binary blob (contains NUL byte or
// non-UTF-8 sequences). Empty input is treated as text.
func isBinary(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	probe := b
	if len(probe) > 8192 {
		probe = probe[:8192]
	}
	for _, x := range probe {
		if x == 0 {
			return true
		}
	}
	return !utf8.Valid(probe)
}

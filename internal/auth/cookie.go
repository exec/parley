package auth

import (
	"net/http"
)

// SessionCookieName is the cookie carrying the JWT for browser sessions.
//
// We accept either the Authorization: Bearer header (legacy + bot API keys)
// or this cookie. The cookie is HttpOnly + Secure + SameSite=Lax so
// page-level XSS can't steal it and cross-site requests don't leak it.
//
// Lax (rather than Strict) so top-level navigations from external links —
// e.g. invite URLs the user clicks — still send the cookie and keep them
// signed in. SameSite=Strict would force a re-login on every external
// landing.
const SessionCookieName = "parley_session"

// SetSessionCookie issues the session cookie on the response. maxAgeSeconds
// matches the JWT's TTL so the browser drops the cookie at the same time
// the token would have expired.
func SetSessionCookie(w http.ResponseWriter, token string, maxAgeSeconds int) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   maxAgeSeconds,
	})
}

// ClearSessionCookie wipes the session cookie. Used by /auth/logout.
func ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// tokenFromCookie returns the session JWT from the cookie, or "" if absent.
func tokenFromCookie(r *http.Request) string {
	c, err := r.Cookie(SessionCookieName)
	if err != nil || c == nil {
		return ""
	}
	return c.Value
}

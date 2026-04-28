package email

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
)

// Client sends transactional emails via the Brevo HTTP API.
type Client struct {
	apiKey    string
	fromEmail string
	fromName  string
}

// NewClient creates a new Brevo email client.
func NewClient(apiKey, fromEmail, fromName string) *Client {
	return &Client{
		apiKey:    apiKey,
		fromEmail: fromEmail,
		fromName:  fromName,
	}
}

type sendEmailRequest struct {
	Sender      emailAddr   `json:"sender"`
	To          []emailAddr `json:"to"`
	Subject     string      `json:"subject"`
	HtmlContent string      `json:"htmlContent"`
}

type emailAddr struct {
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
}

// SendVerificationEmail sends an account verification email to the given address.
func (c *Client) SendVerificationEmail(ctx context.Context, toEmail, toName, token, siteURL string) error {
	// link components are server-minted (siteURL from config, token from CSPRNG)
	// but escape anyway as defense-in-depth — the href value lands in HTML so
	// any future drift toward user influence over either component must not
	// allow attribute breakout.
	link := fmt.Sprintf("%s/verify-email?token=%s", siteURL, token)
	htmlContent := fmt.Sprintf(`<p>Welcome to Parley!</p>
<p>Click the link below to verify your email address:</p>
<p><a href="%s">Verify Email</a></p>
<p>If you didn't create a Parley account, you can safely ignore this email.</p>`, html.EscapeString(link))

	reqBody := sendEmailRequest{
		Sender:      emailAddr{Email: c.fromEmail, Name: c.fromName},
		To:          []emailAddr{{Email: toEmail, Name: toName}},
		Subject:     "Verify your Parley email address",
		HtmlContent: htmlContent,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("email: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.brevo.com/v3/smtp/email", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("email: new request: %w", err)
	}
	req.Header.Set("accept", "application/json")
	req.Header.Set("api-key", c.apiKey)
	req.Header.Set("content-type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("email: do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("email: brevo returned status %d", resp.StatusCode)
	}

	return nil
}

// SendPasswordResetEmail sends a password reset link to the given address.
func (c *Client) SendPasswordResetEmail(ctx context.Context, toEmail, toName, token, siteURL string) error {
	// toName is user-controlled (display name / username) and lands in raw
	// HTML — html.EscapeString prevents tag injection / link spoofing in the
	// rendered email. link is server-minted, escaped as defense-in-depth.
	link := fmt.Sprintf("%s/reset-password?token=%s", siteURL, token)
	htmlContent := fmt.Sprintf(`<p>Hi %s,</p>
<p>You requested a password reset for your Parley account.</p>
<p>Click the link below to set a new password (valid for 24 hours):</p>
<p><a href="%s">Reset Password</a></p>
<p>If you didn't request this, you can safely ignore this email.</p>`, html.EscapeString(toName), html.EscapeString(link))

	reqBody := sendEmailRequest{
		Sender:      emailAddr{Email: c.fromEmail, Name: c.fromName},
		To:          []emailAddr{{Email: toEmail, Name: toName}},
		Subject:     "Reset your Parley password",
		HtmlContent: htmlContent,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("email: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.brevo.com/v3/smtp/email", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("email: new request: %w", err)
	}
	req.Header.Set("accept", "application/json")
	req.Header.Set("api-key", c.apiKey)
	req.Header.Set("content-type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("email: do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("email: brevo returned status %d", resp.StatusCode)
	}
	return nil
}

// SendVerificationSMS sends a 6-digit OTP via Brevo transactional SMS.
func (c *Client) SendVerificationSMS(ctx context.Context, toPhone, code string) error {
	payload := map[string]string{
		"sender":    "Parley",
		"recipient": toPhone,
		"content":   fmt.Sprintf("Your Parley verification code is: %s", code),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal SMS payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.brevo.com/v3/transactionalSMS/sms", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create SMS request: %w", err)
	}
	req.Header.Set("api-key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("SMS request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Brevo SMS API returned %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

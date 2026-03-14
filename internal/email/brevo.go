package email

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
	link := fmt.Sprintf("%s/verify-email?token=%s", siteURL, token)
	htmlContent := fmt.Sprintf(`<p>Welcome to Parley!</p>
<p>Click the link below to verify your email address:</p>
<p><a href="%s">Verify Email</a></p>
<p>If you didn't create a Parley account, you can safely ignore this email.</p>`, link)

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

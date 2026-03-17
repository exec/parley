package ai

// AIGenJob represents a single queued AI generation request.
type AIGenJob struct {
	JobID       string `json:"job_id"`
	UserMessage string `json:"user_message"`
}

// AIGenResult holds the result of a completed AI generation job.
type AIGenResult struct {
	CSS   string `json:"css,omitempty"`
	Error string `json:"error,omitempty"`
}

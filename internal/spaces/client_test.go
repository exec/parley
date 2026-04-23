package spaces

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestClient stands up an httptest server that accepts any PutObject and
// records the ACL header, so we can assert the public/private ACL contract
// without a real S3 backend.
func newTestClient(t *testing.T, bucket, cdnURL string) (*Client, *capturedACL) {
	t.Helper()
	cap := &capturedACL{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cap.method = r.Method
		cap.path = r.URL.Path
		cap.acl = r.Header.Get("X-Amz-Acl")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	c, err := NewClient("AKIA_TEST", "SECRET_TEST", bucket, "us-east-1", srv.URL, cdnURL)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c, cap
}

type capturedACL struct {
	method string
	path   string
	acl    string
}

func TestUpload_PublicReadACLAndCDNURL(t *testing.T) {
	c, cap := newTestClient(t, "parley", "https://cdn.example/parley")

	url, err := c.Upload(context.Background(), "avatars/abc.jpg",
		strings.NewReader("data"), 4)
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}

	if cap.method != http.MethodPut {
		t.Errorf("method = %q, want PUT", cap.method)
	}
	if cap.acl != "public-read" {
		t.Errorf("X-Amz-Acl = %q, want public-read", cap.acl)
	}
	if want := "https://cdn.example/parley/avatars/abc.jpg"; url != want {
		t.Errorf("returned URL = %q, want %q", url, want)
	}
}

func TestUploadPrivate_PrivateACLAndKeyReturned(t *testing.T) {
	c, cap := newTestClient(t, "parley-backups", "https://cdn.example/parley")

	got, err := c.UploadPrivate(context.Background(), "backups/parley-20260423.dump",
		strings.NewReader("dump-bytes"), 10)
	if err != nil {
		t.Fatalf("UploadPrivate: %v", err)
	}

	if cap.acl != "private" {
		t.Errorf("X-Amz-Acl = %q, want private", cap.acl)
	}
	// Private uploads must NOT return a CDN URL — callers need to route
	// through an authenticated fetch path.
	if strings.Contains(got, "cdn.example") {
		t.Errorf("UploadPrivate returned CDN URL %q; private objects must return the bucket key only", got)
	}
	if got != "backups/parley-20260423.dump" {
		t.Errorf("returned key = %q, want the raw object key", got)
	}
}

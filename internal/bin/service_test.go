package bin

import (
	"errors"
	"testing"
)

// NOTE: This package imports internal/auth (via handler.go), which calls
// log.Fatal at package init if JWT_SECRET is unset. Run tests with:
//   JWT_SECRET=test go test ./internal/bin/...

// TestSentinelErrorsAreDistinct verifies that all sentinel errors in the bin
// package are unique and can be distinguished with errors.Is.
func TestSentinelErrorsAreDistinct(t *testing.T) {
	sentinels := []struct {
		name string
		err  error
	}{
		{"ErrPostNotFound", ErrPostNotFound},
		{"ErrForbidden", ErrForbidden},
		{"ErrChannelNotFound", ErrChannelNotFound},
		{"ErrNotBinChannel", ErrNotBinChannel},
		{"ErrVersionNotFound", ErrVersionNotFound},
		{"ErrCommentNotFound", ErrCommentNotFound},
		{"ErrTagNotFound", ErrTagNotFound},
	}

	for i, a := range sentinels {
		if a.err == nil {
			t.Errorf("%s is nil", a.name)
			continue
		}
		for j, b := range sentinels {
			if i == j {
				continue
			}
			if errors.Is(a.err, b.err) {
				t.Errorf("%s and %s should be distinct but errors.Is returns true", a.name, b.name)
			}
		}
	}
}

// TestSentinelErrorMessages verifies that each sentinel error has a non-empty,
// human-readable message.
func TestSentinelErrorMessages(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"ErrPostNotFound", ErrPostNotFound, "post not found"},
		{"ErrForbidden", ErrForbidden, "forbidden"},
		{"ErrChannelNotFound", ErrChannelNotFound, "channel not found"},
		{"ErrNotBinChannel", ErrNotBinChannel, "channel is not a bin channel"},
		{"ErrVersionNotFound", ErrVersionNotFound, "version not found"},
		{"ErrCommentNotFound", ErrCommentNotFound, "comment not found"},
		{"ErrTagNotFound", ErrTagNotFound, "tag not found"},
	}
	for _, tc := range cases {
		if tc.err.Error() != tc.want {
			t.Errorf("%s.Error() = %q, want %q", tc.name, tc.err.Error(), tc.want)
		}
	}
}

// TestNewServiceNotNil verifies that NewService returns a non-nil *Service.
func TestNewServiceNotNil(t *testing.T) {
	svc := NewService(nil)
	if svc == nil {
		t.Fatal("NewService(nil) returned nil")
	}
}

// TestSetHubDoesNotPanic verifies that SetHub can be called (even with nil).
func TestSetHubDoesNotPanic(t *testing.T) {
	svc := NewService(nil)
	svc.SetHub(nil) // should not panic
}

// TestCreatePostValidation tests the validation logic in CreatePost that runs
// before any DB calls.  Because we pass a nil repo, these checks must fail
// before any repo method is called (otherwise we'd get a nil-pointer panic).
func TestCreatePostValidation(t *testing.T) {
	svc := NewService(nil)

	tests := []struct {
		name      string
		channelID string
		userID    string
		title     string
		wantErr   string
	}{
		{"empty title", "1", "1", "", "title is required"},
		{"invalid channel ID", "abc", "1", "t", "invalid channel ID"},
		{"invalid user ID", "1", "abc", "t", "invalid user ID"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.CreatePost(t.Context(), tc.channelID, tc.userID, tc.title, "", nil, nil)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if err.Error() != tc.wantErr {
				t.Errorf("error = %q, want %q", err.Error(), tc.wantErr)
			}
		})
	}
}

// TestGetPostValidation tests validation in GetPost.
func TestGetPostValidation(t *testing.T) {
	svc := NewService(nil)

	_, err := svc.GetPost(t.Context(), "not-a-number", "1")
	if err == nil || err.Error() != "invalid post ID" {
		t.Errorf("expected 'invalid post ID', got %v", err)
	}
}

// TestListPostsValidation tests validation in ListPosts.
func TestListPostsValidation(t *testing.T) {
	svc := NewService(nil)

	_, err := svc.ListPosts(t.Context(), "abc", "1", "", "", "", "", 25, 0)
	if err == nil || err.Error() != "invalid channel ID" {
		t.Errorf("expected 'invalid channel ID', got %v", err)
	}
}

// TestEditPostValidation tests validation in EditPost.
func TestEditPostValidation(t *testing.T) {
	svc := NewService(nil)

	tests := []struct {
		name    string
		postID  string
		userID  string
		wantErr string
	}{
		{"invalid post ID", "abc", "1", "invalid post ID"},
		{"invalid user ID", "1", "abc", "invalid user ID"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.EditPost(t.Context(), tc.postID, tc.userID, "t", "", nil, nil)
			if err == nil || err.Error() != tc.wantErr {
				t.Errorf("error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

// TestDeletePostValidation tests validation in DeletePost.
func TestDeletePostValidation(t *testing.T) {
	svc := NewService(nil)

	tests := []struct {
		name    string
		postID  string
		userID  string
		wantErr string
	}{
		{"invalid post ID", "abc", "1", "invalid post ID"},
		{"invalid user ID", "1", "abc", "invalid user ID"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := svc.DeletePost(t.Context(), tc.postID, tc.userID)
			if err == nil || err.Error() != tc.wantErr {
				t.Errorf("error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

// TestGetVersionsValidation tests validation in GetVersions.
func TestGetVersionsValidation(t *testing.T) {
	svc := NewService(nil)

	_, err := svc.GetVersions(t.Context(), "abc", "1")
	if err == nil || err.Error() != "invalid post ID" {
		t.Errorf("expected 'invalid post ID', got %v", err)
	}
}

// TestGetVersionValidation tests validation in GetVersion.
func TestGetVersionValidation(t *testing.T) {
	svc := NewService(nil)

	_, err := svc.GetVersion(t.Context(), "abc", "1")
	if err == nil || err.Error() != "invalid version ID" {
		t.Errorf("expected 'invalid version ID', got %v", err)
	}
}

// TestCreateLineCommentValidation tests validation in CreateLineComment.
func TestCreateLineCommentValidation(t *testing.T) {
	svc := NewService(nil)

	tests := []struct {
		name      string
		postID    string
		userID    string
		versionID string
		fileID    string
		content   string
		parentID  string
		wantErr   string
	}{
		{"empty content", "1", "1", "1", "1", "", "", "content is required"},
		{"invalid post ID", "abc", "1", "1", "1", "c", "", "invalid post ID"},
		{"invalid user ID", "1", "abc", "1", "1", "c", "", "invalid user ID"},
		{"invalid version ID", "1", "1", "abc", "1", "c", "", "invalid version ID"},
		{"invalid file ID", "1", "1", "1", "abc", "c", "", "invalid file ID"},
		{"invalid parent ID", "1", "1", "1", "1", "c", "abc", "invalid parent ID"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.CreateLineComment(t.Context(), tc.postID, tc.userID, tc.versionID, tc.fileID, 1, tc.content, tc.parentID)
			if err == nil || err.Error() != tc.wantErr {
				t.Errorf("error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

// TestGetLineCommentsValidation tests validation in GetLineComments.
func TestGetLineCommentsValidation(t *testing.T) {
	svc := NewService(nil)

	_, err := svc.GetLineComments(t.Context(), "abc", "", nil, nil)
	if err == nil || err.Error() != "invalid post ID" {
		t.Errorf("expected 'invalid post ID', got %v", err)
	}

	badVersion := "abc"
	_, err = svc.GetLineComments(t.Context(), "1", "abc", &badVersion, nil)
	if err == nil || err.Error() != "invalid user ID" {
		t.Errorf("expected 'invalid user ID', got %v", err)
	}
}

// TestUpdateLineCommentValidation tests validation in UpdateLineComment.
func TestUpdateLineCommentValidation(t *testing.T) {
	svc := NewService(nil)

	tests := []struct {
		name      string
		commentID string
		userID    string
		content   string
		wantErr   string
	}{
		{"empty content", "1", "1", "", "content is required"},
		{"invalid comment ID", "abc", "1", "c", "invalid comment ID"},
		{"invalid user ID", "1", "abc", "c", "invalid user ID"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.UpdateLineComment(t.Context(), tc.commentID, tc.userID, tc.content)
			if err == nil || err.Error() != tc.wantErr {
				t.Errorf("error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

// TestDeleteLineCommentValidation tests validation in DeleteLineComment.
func TestDeleteLineCommentValidation(t *testing.T) {
	svc := NewService(nil)

	tests := []struct {
		name      string
		commentID string
		userID    string
		wantErr   string
	}{
		{"invalid comment ID", "abc", "1", "invalid comment ID"},
		{"invalid user ID", "1", "abc", "invalid user ID"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := svc.DeleteLineComment(t.Context(), tc.commentID, tc.userID)
			if err == nil || err.Error() != tc.wantErr {
				t.Errorf("error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

// TestCreateTagValidation tests validation in CreateTag.
func TestCreateTagValidation(t *testing.T) {
	svc := NewService(nil)

	tests := []struct {
		name      string
		channelID string
		userID    string
		tagName   string
		wantErr   string
	}{
		{"empty name", "1", "", "", "name is required"},
		{"invalid channel ID", "abc", "", "t", "invalid channel ID"},
		{"invalid user ID", "1", "abc", "t", "invalid user ID"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.CreateTag(t.Context(), tc.channelID, tc.userID, tc.tagName, "")
			if err == nil || err.Error() != tc.wantErr {
				t.Errorf("error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

// TestGetTagsValidation tests validation in GetTags.
func TestGetTagsValidation(t *testing.T) {
	svc := NewService(nil)

	_, err := svc.GetTags(t.Context(), "abc")
	if err == nil || err.Error() != "invalid channel ID" {
		t.Errorf("expected 'invalid channel ID', got %v", err)
	}
}

// TestDeleteTagValidation tests validation in DeleteTag.
func TestDeleteTagValidation(t *testing.T) {
	svc := NewService(nil)

	err := svc.DeleteTag(t.Context(), "abc", "1")
	if err == nil || err.Error() != "invalid tag ID" {
		t.Errorf("expected 'invalid tag ID', got %v", err)
	}

	err = svc.DeleteTag(t.Context(), "1", "abc")
	if err == nil || err.Error() != "invalid user ID" {
		t.Errorf("expected 'invalid user ID', got %v", err)
	}
}

package projects

import (
	"errors"
	"testing"
)

// NOTE: this package imports internal/auth (via handler.go), which calls
// log.Fatal at package init if JWT_SECRET is unset or under 32 bytes. Run with:
//   JWT_SECRET=test-secret-at-least-32-bytes-long-1234 go test ./internal/projects/...

// TestSentinelErrorsAreDistinct verifies that errors.Is can distinguish each
// sentinel from every other one.
func TestSentinelErrorsAreDistinct(t *testing.T) {
	sentinels := []struct {
		name string
		err  error
	}{
		{"ErrProjectNotFound", ErrProjectNotFound},
		{"ErrServerNotFound", ErrServerNotFound},
		{"ErrNotMember", ErrNotMember},
		{"ErrForbidden", ErrForbidden},
		{"ErrInvalidInput", ErrInvalidInput},
		{"ErrPresetNotFound", ErrPresetNotFound},
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

// TestSentinelErrorMessages verifies each sentinel has a stable message.
func TestSentinelErrorMessages(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"ErrProjectNotFound", ErrProjectNotFound, "project not found"},
		{"ErrServerNotFound", ErrServerNotFound, "server not found"},
		{"ErrNotMember", ErrNotMember, "not a server member"},
		{"ErrForbidden", ErrForbidden, "forbidden"},
		{"ErrInvalidInput", ErrInvalidInput, "invalid input"},
		{"ErrPresetNotFound", ErrPresetNotFound, "preset not found"},
	}
	for _, tc := range cases {
		if tc.err.Error() != tc.want {
			t.Errorf("%s.Error() = %q, want %q", tc.name, tc.err.Error(), tc.want)
		}
	}
}

// TestNewServiceNotNil verifies the constructor.
func TestNewServiceNotNil(t *testing.T) {
	if svc := NewService(nil); svc == nil {
		t.Fatal("NewService(nil) returned nil")
	}
}

// TestSetHubDoesNotPanic verifies SetHub is safe with nil.
func TestSetHubDoesNotPanic(t *testing.T) {
	svc := NewService(nil)
	svc.SetHub(nil)
}

// TestValidSkillLevels checks the allowlist matches the DB CHECK constraint.
// If this test fails, Migration #72's CHECK and validSkillLevels have drifted.
func TestValidSkillLevels(t *testing.T) {
	want := []string{"beginner", "intermediate", "expert", "auto", "custom"}
	if len(validSkillLevels) != len(want) {
		t.Fatalf("validSkillLevels has %d entries, want %d", len(validSkillLevels), len(want))
	}
	for _, s := range want {
		if !validSkillLevels[s] {
			t.Errorf("validSkillLevels missing %q", s)
		}
	}
}

// TestCreateProjectValidation exercises the input checks that run before any
// DB call so a nil repo is safe.
func TestCreateProjectValidation(t *testing.T) {
	svc := NewService(nil)

	tests := []struct {
		name    string
		userID  string
		input   CreateInput
		wantErr error
	}{
		{
			name:    "invalid user ID",
			userID:  "not-a-number",
			input:   CreateInput{ServerID: 1, Name: "x", SkillLevel: "auto"},
			wantErr: ErrInvalidInput,
		},
		{
			name:    "empty name",
			userID:  "1",
			input:   CreateInput{ServerID: 1, Name: "", SkillLevel: "auto"},
			wantErr: ErrInvalidInput,
		},
		{
			name:    "invalid skill level",
			userID:  "1",
			input:   CreateInput{ServerID: 1, Name: "x", SkillLevel: "wizard"},
			wantErr: ErrInvalidInput,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.CreateProject(t.Context(), tc.userID, tc.input)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("error = %v, want errors.Is(_, %v) == true", err, tc.wantErr)
			}
		})
	}
}

// TestGetProjectValidation covers ID-parse failures.
func TestGetProjectValidation(t *testing.T) {
	svc := NewService(nil)
	cases := []struct{ projectID, userID string }{
		{"not-a-number", "1"},
		{"1", "not-a-number"},
	}
	for _, c := range cases {
		_, err := svc.GetProject(t.Context(), c.projectID, c.userID)
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("GetProject(%q,%q) = %v, want ErrInvalidInput", c.projectID, c.userID, err)
		}
	}
}

// TestListServerProjectsValidation covers ID-parse failures.
func TestListServerProjectsValidation(t *testing.T) {
	svc := NewService(nil)
	cases := []struct{ serverID, userID string }{
		{"not-a-number", "1"},
		{"1", "not-a-number"},
	}
	for _, c := range cases {
		_, err := svc.ListServerProjects(t.Context(), c.serverID, c.userID)
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("ListServerProjects(%q,%q) = %v, want ErrInvalidInput", c.serverID, c.userID, err)
		}
	}
}

// TestUpdateProjectValidation covers ID parse + skill_level validation.
// (Skill validation runs after the ownership lookup, so we only test the
// ID-parse paths that happen before any DB call.)
func TestUpdateProjectValidation(t *testing.T) {
	svc := NewService(nil)
	cases := []struct{ projectID, userID string }{
		{"not-a-number", "1"},
		{"1", "not-a-number"},
	}
	for _, c := range cases {
		_, err := svc.UpdateProject(t.Context(), c.projectID, c.userID, UpdateInput{})
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("UpdateProject(%q,%q) = %v, want ErrInvalidInput", c.projectID, c.userID, err)
		}
	}
}

// TestUpdateClaudeMDValidation covers ID parse failures.
func TestUpdateClaudeMDValidation(t *testing.T) {
	svc := NewService(nil)
	cases := []struct{ projectID, userID string }{
		{"not-a-number", "1"},
		{"1", "not-a-number"},
	}
	for _, c := range cases {
		_, _, err := svc.UpdateClaudeMD(t.Context(), c.projectID, c.userID, "x")
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("UpdateClaudeMD(%q,%q) = %v, want ErrInvalidInput", c.projectID, c.userID, err)
		}
	}
}

// TestDeleteProjectValidation covers ID parse failures.
func TestDeleteProjectValidation(t *testing.T) {
	svc := NewService(nil)
	cases := []struct{ projectID, userID string }{
		{"not-a-number", "1"},
		{"1", "not-a-number"},
	}
	for _, c := range cases {
		err := svc.DeleteProject(t.Context(), c.projectID, c.userID)
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("DeleteProject(%q,%q) = %v, want ErrInvalidInput", c.projectID, c.userID, err)
		}
	}
}

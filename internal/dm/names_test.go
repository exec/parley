package dm

import (
	"strings"
	"testing"
)

func TestPickGroupName_Size3(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		name := PickGroupName(3)
		if !contains(namesSize3, name) {
			t.Fatalf("PickGroupName(3) returned %q, not in size-3 bucket", name)
		}
		seen[name] = true
	}
	if len(seen) < 3 {
		t.Fatalf("expected variety across 50 picks, got %d unique", len(seen))
	}
}

func TestPickGroupName_SizeBuckets(t *testing.T) {
	cases := []struct {
		size   int
		bucket []string
	}{
		{3, namesSize3},
		{4, namesSizeSmall},
		{7, namesSizeSmall},
		{10, namesSizeSmall},
		{11, namesSizeLarge},
		{25, namesSizeLarge},
		{99, namesSizeLarge},
	}
	for _, c := range cases {
		name := PickGroupName(c.size)
		if !contains(c.bucket, name) {
			t.Errorf("PickGroupName(%d) = %q, not in expected bucket", c.size, name)
		}
	}
}

func TestPickGroupName_BelowMinFallsBackToSize3(t *testing.T) {
	name := PickGroupName(1)
	if !contains(namesSize3, name) {
		t.Errorf("PickGroupName(1) should fall back to size-3 bucket, got %q", name)
	}
}

func contains(bucket []string, name string) bool {
	for _, n := range bucket {
		if strings.EqualFold(n, name) {
			return true
		}
	}
	return false
}

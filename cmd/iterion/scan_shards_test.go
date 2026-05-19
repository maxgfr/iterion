package main

import (
	"strings"
	"testing"
)

// TestPlanShards_DeterministicIDs exercises planShards' contract:
// given the same parent run id, file list, and shard size, the plan
// must be byte-identical across calls (deterministic run ids let
// `iterion __scan-shards` survive interruptions: a re-invocation
// reattaches to the previously-spawned child runs instead of
// orphaning them and creating a fresh set).
func TestPlanShards_DeterministicIDs(t *testing.T) {
	files := []string{"a.go", "b.go", "c.go", "d.go", "e.go"}
	parent := "parent-abc"
	p1 := planShards(files, 2, parent)
	p2 := planShards(files, 2, parent)
	if len(p1) != 3 {
		t.Fatalf("expected 3 shards for 5 files at size 2, got %d", len(p1))
	}
	for i := range p1 {
		if p1[i].RunID != p2[i].RunID {
			t.Errorf("shard %d id drift: %s vs %s", i, p1[i].RunID, p2[i].RunID)
		}
		if p1[i].Index != p2[i].Index {
			t.Errorf("shard %d index drift: %d vs %d", i, p1[i].Index, p2[i].Index)
		}
	}
}

// Different parent ids must produce disjoint run id namespaces so two
// independent parent runs don't collide on the same child id.
func TestPlanShards_DifferentParentsDisjoint(t *testing.T) {
	files := []string{"a.go", "b.go", "c.go"}
	a := planShards(files, 1, "parent-a")
	b := planShards(files, 1, "parent-b")
	seen := map[string]bool{}
	for _, s := range a {
		seen[s.RunID] = true
	}
	for _, s := range b {
		if seen[s.RunID] {
			t.Errorf("run id %s collision between parent-a and parent-b", s.RunID)
		}
	}
}

// Last shard correctly carries the remainder when len(files) is not a
// multiple of shard_size. A bug here would silently drop files.
func TestPlanShards_LastShardRemainder(t *testing.T) {
	files := []string{"a", "b", "c", "d", "e", "f", "g"}
	plan := planShards(files, 3, "p")
	if len(plan) != 3 {
		t.Fatalf("expected 3 shards (3+3+1), got %d", len(plan))
	}
	wantLens := []int{3, 3, 1}
	for i, w := range wantLens {
		if len(plan[i].Files) != w {
			t.Errorf("shard %d: expected %d files, got %d", i, w, len(plan[i].Files))
		}
	}
	totalFiles := 0
	for _, s := range plan {
		totalFiles += len(s.Files)
	}
	if totalFiles != len(files) {
		t.Errorf("file count drift: planned %d, input %d (files dropped)", totalFiles, len(files))
	}
}

// Run ids must be in the documented format "shard-<8 bytes hex>" so
// they can be filtered / globbed in the run store.
func TestPlanShards_RunIDFormat(t *testing.T) {
	plan := planShards([]string{"a"}, 1, "p")
	if len(plan) != 1 {
		t.Fatalf("expected 1 shard, got %d", len(plan))
	}
	id := plan[0].RunID
	if !strings.HasPrefix(id, "shard-") {
		t.Errorf("run id missing 'shard-' prefix: %s", id)
	}
	if len(id) != len("shard-")+16 { // 8 bytes hex = 16 chars
		t.Errorf("run id wrong length: got %d, want %d (shard- + 16 hex chars)", len(id), len("shard-")+16)
	}
}

// Empty file list produces zero shards. Callers depend on this to
// short-circuit "no files to scan" without an extra branch.
func TestPlanShards_EmptyInput(t *testing.T) {
	if got := planShards(nil, 10, "p"); len(got) != 0 {
		t.Errorf("expected 0 shards for nil input, got %d", len(got))
	}
	if got := planShards([]string{}, 10, "p"); len(got) != 0 {
		t.Errorf("expected 0 shards for empty slice, got %d", len(got))
	}
}

package store

import "testing"

// The snapshot ref namespaces are load-bearing: the runtime writes them
// and the Fork API checks them out by exact name. A reordered or dropped
// format arg would silently desync writer and reader.
func TestNodeSnapshotRef(t *testing.T) {
	tests := []struct {
		runID, nodeID string
		loopIter      int
		want          string
	}{
		{"r1", "n1", 3, "refs/iterion/runs/r1/nodes/n1/3"},
		{"run-x", "node-y", 0, "refs/iterion/runs/run-x/nodes/node-y/0"},
	}
	for _, tt := range tests {
		if got := NodeSnapshotRef(tt.runID, tt.nodeID, tt.loopIter); got != tt.want {
			t.Errorf("NodeSnapshotRef(%q, %q, %d) = %q, want %q", tt.runID, tt.nodeID, tt.loopIter, got, tt.want)
		}
	}
}

func TestTurnSnapshotRef(t *testing.T) {
	tests := []struct {
		runID, nodeID  string
		loopIter, turn int
		want           string
	}{
		{"r1", "n1", 3, 7, "refs/iterion/runs/r1/turns/n1/3/7"},
		{"run-x", "node-y", 0, 0, "refs/iterion/runs/run-x/turns/node-y/0/0"},
	}
	for _, tt := range tests {
		if got := TurnSnapshotRef(tt.runID, tt.nodeID, tt.loopIter, tt.turn); got != tt.want {
			t.Errorf("TurnSnapshotRef(%q, %q, %d, %d) = %q, want %q", tt.runID, tt.nodeID, tt.loopIter, tt.turn, got, tt.want)
		}
	}
}

package blob

import (
	"fmt"

	"github.com/SocialGouv/iterion/pkg/store"
)

// artifactKey builds the canonical S3 key for an artifact body.
// Format: artifacts/<run_id>/<node_id>/<version>.json.
//
// Returns a descriptive error when any component fails sanitisation
// (path separator, traversal, control chars) or when version is
// negative. Callers in the blob package are all already in a fallible
// context (PutArtifact / GetArtifact / migration tooling), so
// surfacing the error is cheaper than the previous panic + recover
// dance and avoids killing the whole request-path goroutine when a
// malformed runID slips through the upstream sanitiser.
func artifactKey(runID, nodeID string, version int) (string, error) {
	if err := store.SanitizePathComponent("run_id", runID); err != nil {
		return "", fmt.Errorf("blob: invalid run_id: %w", err)
	}
	if err := store.SanitizePathComponent("node_id", nodeID); err != nil {
		return "", fmt.Errorf("blob: invalid node_id: %w", err)
	}
	if version < 0 {
		return "", fmt.Errorf("blob: negative artifact version %d", version)
	}
	return fmt.Sprintf("artifacts/%s/%s/%d.json", runID, nodeID, version), nil
}

// attachmentKey builds the canonical S3 key for an attachment body.
// Format: attachments/<run_id>/<name>/<filename>.
//
// All three components are sanitised against the same "no separators,
// no control chars, no traversal" invariant as artifactKey — the
// FS-mirror path uses filepath.Base for the filename so keeping the
// S3 key shape consistent prevents `migrate to-cloud` from producing
// keys the FS layer would have flattened (F-ST-10).
func attachmentKey(runID, name, filename string) (string, error) {
	if err := store.SanitizePathComponent("run_id", runID); err != nil {
		return "", fmt.Errorf("blob: invalid run_id: %w", err)
	}
	if err := store.SanitizePathComponent("attachment_name", name); err != nil {
		return "", fmt.Errorf("blob: invalid attachment_name: %w", err)
	}
	if err := store.SanitizePathComponent("attachment_filename", filename); err != nil {
		return "", fmt.Errorf("blob: invalid attachment_filename: %w", err)
	}
	return fmt.Sprintf("attachments/%s/%s/%s", runID, name, filename), nil
}

// attachmentRunPrefix is the S3 key prefix containing every
// attachment for a run. Trailing slash is included so a delete-by-
// prefix doesn't accidentally match `attachments/<runID>-other/`.
func attachmentRunPrefix(runID string) (string, error) {
	if err := store.SanitizePathComponent("run_id", runID); err != nil {
		return "", fmt.Errorf("blob: invalid run_id: %w", err)
	}
	return fmt.Sprintf("attachments/%s/", runID), nil
}

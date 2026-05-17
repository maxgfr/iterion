package blob

import (
	"fmt"

	"github.com/SocialGouv/iterion/pkg/store"
)

// artifactKey builds the canonical S3 key for an artifact body.
// Format: artifacts/<run_id>/<node_id>/<version>.json.
//
// IDs are sanitised against the same rules the filesystem store
// enforces. Any failure here is a programmer bug — IDs reach this
// path already sanitised — so we panic rather than threading an
// error through every blob write site.
func artifactKey(runID, nodeID string, version int) string {
	if err := store.SanitizePathComponent("run_id", runID); err != nil {
		panic(err)
	}
	if err := store.SanitizePathComponent("node_id", nodeID); err != nil {
		panic(err)
	}
	if version < 0 {
		panic(fmt.Sprintf("blob: negative artifact version %d", version))
	}
	return fmt.Sprintf("artifacts/%s/%s/%d.json", runID, nodeID, version)
}

// attachmentKey builds the canonical S3 key for an attachment body.
// Format: attachments/<run_id>/<name>/<filename>.
//
// The filename is run through SanitizePathComponent so it satisfies
// the same "no separators, no control chars, no traversal" invariant
// as runID and name. The FS-mirror path uses filepath.Base — keeping
// the S3 key shape consistent prevents `migrate to-cloud` from
// producing keys the FS layer would have flattened, and avoids any
// future caller surfacing an unsanitised value as a probe vector
// (F-ST-10).
func attachmentKey(runID, name, filename string) string {
	if err := store.SanitizePathComponent("run_id", runID); err != nil {
		panic(err)
	}
	if err := store.SanitizePathComponent("attachment_name", name); err != nil {
		panic(err)
	}
	if err := store.SanitizePathComponent("attachment_filename", filename); err != nil {
		panic(err)
	}
	return fmt.Sprintf("attachments/%s/%s/%s", runID, name, filename)
}

// attachmentRunPrefix is the S3 key prefix containing every
// attachment for a run. Trailing slash is included so a delete-by-
// prefix doesn't accidentally match `attachments/<runID>-other/`.
func attachmentRunPrefix(runID string) string {
	if err := store.SanitizePathComponent("run_id", runID); err != nil {
		panic(err)
	}
	return fmt.Sprintf("attachments/%s/", runID)
}

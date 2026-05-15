package conductor

import "os"

// osHostname and osPid are package-level shims so tests can override
// the values used in the host claim marker (`<host>-<pid>`).
var (
	osHostname = os.Hostname
	osPid      = os.Getpid
)

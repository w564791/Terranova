package version

import "fmt"

// Set via -ldflags at build time
var (
	CommitHash = "dev"
	BuildTime  = "unknown"
)

func String() string {
	return fmt.Sprintf("commit=%s build=%s", CommitHash, BuildTime)
}

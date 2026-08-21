package version

import "fmt"

// These values are intended to be set at link time with -ldflags.
var (
	Version   = "0.1.0"
	Commit    = "unknown"
	BuildTime = "unknown"
)

func String() string {
	return fmt.Sprintf("RiskLens Agent %s (commit %s, built %s)", Version, Commit, BuildTime)
}

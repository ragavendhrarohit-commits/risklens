package browseractivity

import "strings"

// AllowSample is the foreground gate. Background Chrome with GitHub open
// while Excel is focused must emit nothing.
func AllowSample(fg Foreground, sample Sample) bool {
	if strings.TrimSpace(fg.SessionID) == "" {
		return false
	}
	if !IsBrowserExecutable(fg.ExecutableName) {
		return false
	}
	return NormalizeExecutable(fg.ExecutableName) == NormalizeExecutable(sample.Executable)
}

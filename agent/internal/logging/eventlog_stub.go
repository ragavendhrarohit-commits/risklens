//go:build !windows

package logging

func openEventLog(source string) eventLogger {
	return nil
}

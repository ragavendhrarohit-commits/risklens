package logging

const (
	eventServiceStarting     uint32 = 1000
	eventServiceStarted      uint32 = 1001
	eventServiceStopping     uint32 = 1002
	eventServiceStopped      uint32 = 1003
	eventConfigurationLoaded uint32 = 1004
	eventVersion             uint32 = 1005
	eventLogWriteFailed      uint32 = 1099
)

var lifecycleEventIDs = map[string]uint32{
	"Service starting":     eventServiceStarting,
	"Service started":      eventServiceStarted,
	"Service stopping":     eventServiceStopping,
	"Service stopped":      eventServiceStopped,
	"Configuration loaded": eventConfigurationLoaded,
	"Version":              eventVersion,
}

type eventLogger interface {
	Info(eid uint32, msg string)
	Warn(eid uint32, msg string)
	Error(eid uint32, msg string)
	Close() error
}

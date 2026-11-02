package version

import (
	"fmt"
	"runtime"
	"strings"
)

// Annotate ties a persisted execution result to the process that produced it.
func Annotate(message string) string {
	if strings.Contains(message, " | build=") {
		return message
	}
	return fmt.Sprintf("%s | build=%s commit=%s built_at=%s", message, Version, Commit, BuildTime)
}

var (
	// Version, Commit and BuildTime are populated by -ldflags during release builds.
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = "unknown"
)

type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"build_time"`
	GoVersion string `json:"go_version"`
}

func Get() Info {
	return Info{
		Version:   Version,
		Commit:    Commit,
		BuildTime: BuildTime,
		GoVersion: runtime.Version(),
	}
}

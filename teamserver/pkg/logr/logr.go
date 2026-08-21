package logr

import (
	"os"
	"path/filepath"
	"strings"

	"Havoc/pkg/logger"
)

type Logr struct {
	// Path to directory where everything is going to be logged (user chat, input/output from agent)
	Path         string
	ListenerPath string
	AgentPath    string
	ServerPath   string

	LogrSendText func(text string)
}

var LogrInstance *Logr

func NewLogr(Server, Path string) *Logr {
	var (
		logr = new(Logr)
		err  error
	)

	logr.ServerPath = Server
	logr.Path = Server + "/" + Path
	logr.ListenerPath = Path + "/listener"
	logr.AgentPath = Path + "/agents"

	if _, err = os.Stat(Path); os.IsNotExist(err) {
		if err = os.MkdirAll(Path, os.ModePerm); err != nil {
			logger.Error("Failed to create Logr folder: " + err.Error())
			return nil
		}
	}

	if _, err = os.Stat(logr.AgentPath); os.IsNotExist(err) {
		if err = os.MkdirAll(logr.AgentPath, os.ModePerm); err != nil {
			logger.Error("Failed to create Logr agent folder: " + err.Error())
			return nil
		}
	}

	if _, err = os.Stat(logr.ListenerPath); os.IsNotExist(err) {
		if err = os.MkdirAll(logr.ListenerPath, os.ModePerm); err != nil {
			logger.Error("Failed to create Logr listener folder: " + err.Error())
			return nil
		}
	}

	return logr
}

// PathWithin reports whether target is contained within base using
// filepath.Rel, so a sibling directory that merely shares a string
// prefix with base is rejected.
func PathWithin(base, target string) bool {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

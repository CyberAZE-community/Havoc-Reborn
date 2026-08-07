package events

import (
	"encoding/base64"
	"time"

	"Havoc/pkg/packager"
)

type loot int

var Loot loot

// SendFile delivers a looted file's content to the requesting client.
// Marked OneTime so it is neither stored in the event log nor replayed.
func (loot) SendFile(AgentID, FileName string, Content []byte) packager.Package {
	return packager.Package{
		Head: packager.Head{
			Event:   packager.Type.Loot.Type,
			Time:    time.Now().Format("02/01/2006 15:04:05"),
			OneTime: "true",
		},
		Body: packager.Body{
			SubEvent: packager.Type.Loot.SendFile,
			Info: map[string]any{
				"AgentID":  AgentID,
				"FileName": FileName,
				"Content":  base64.StdEncoding.EncodeToString(Content),
			},
		},
	}
}

func (loot) SendError(Message string) packager.Package {
	return packager.Package{
		Head: packager.Head{
			Event:   packager.Type.Loot.Type,
			Time:    time.Now().Format("02/01/2006 15:04:05"),
			OneTime: "true",
		},
		Body: packager.Body{
			SubEvent: packager.Type.Loot.Error,
			Info: map[string]any{
				"Error": Message,
			},
		},
	}
}

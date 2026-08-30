package server

import (
	"Havoc/pkg/logger"
	crand "crypto/rand"
	"encoding/json"
	"math/big"
	"fmt"
	"strconv"

	"Havoc/pkg/agent"
	"Havoc/pkg/events"
	"Havoc/pkg/packager"
)

func (t *Teamserver) AgentUpdate(agent *agent.Agent) {
	err := t.DB.AgentUpdate(agent)
	if err != nil {
		logger.Error("Could not update agent: " + err.Error())
	}
}

func (t *Teamserver) Died(Agent *agent.Agent) {
	Agent.Active = false
	t.UnlinkFromAll(Agent)
	t.EventAgentMark(Agent.NameID, "Dead")
	t.AgentUpdate(Agent)
}

func (t *Teamserver) UnlinkFromAll(Agent *agent.Agent) {
	// remove all links from agent. the loop reads a snapshot instead of the
	// live slice (unlocked Links[0] reads race concurrent PivotLinkAdd), and
	// removing by NameID keeps the remaining snapshot entries valid.
	for _, Link := range Agent.PivotsSnapshotLinks() {
		Agent.PivotLinkRemove(Link.NameID)
		t.LinkRemove(Agent, Link, false)
	}

	// remove agent from parent's link
	for _, ParentAgent := range t.Agents.Snapshot() {
		if ParentAgent.NameID == Agent.NameID {
			continue
		}

		for _, Link := range ParentAgent.PivotsSnapshotLinks() {
			if Link.NameID == Agent.NameID {
				t.LinkRemove(ParentAgent, Agent, false)
				ParentAgent.PivotLinkRemove(Agent.NameID)
				break
			}
		}
	}
}

func (t *Teamserver) ParentOf(Agent *agent.Agent) (int, error) {
	var AgentID, _ = strconv.ParseInt(Agent.NameID, 16, 64)

	ID, err := t.DB.ParentOf(int(AgentID))
	return ID, err
}

func (t *Teamserver) LinksOf(Agent *agent.Agent) []int {
	var AgentID, _ = strconv.ParseInt(Agent.NameID, 16, 64)

	return t.DB.LinksOf(int(AgentID))
}

func (t *Teamserver) LinkAdd(ParentAgent *agent.Agent, LinkAgent *agent.Agent) error {
	var ParentAgentID, _ = strconv.ParseInt(ParentAgent.NameID, 16, 64)
	var LinkAgentID,   _ = strconv.ParseInt(LinkAgent.NameID, 16, 64)

	err := t.DB.LinkAdd(int(ParentAgentID), int(LinkAgentID))
	if err != nil {
		logger.Error("Could not add link to database: " + err.Error())
	}

	return nil
}

func (t *Teamserver) LinkRemove(ParentAgent *agent.Agent, LinkAgent *agent.Agent, UpdateLinks bool) {
	var ParentAgentID, _ = strconv.ParseInt(ParentAgent.NameID, 16, 64)
	var LinkAgentID,   _ = strconv.ParseInt(LinkAgent.NameID, 16, 64)

	LinkAgent.Active = false
	LinkAgent.Reason = "Disconnected"

	if UpdateLinks {
		ParentAgent.PivotLinkRemove(LinkAgent.NameID)
	}

	err := t.DB.LinkRemove(int(ParentAgentID), int(LinkAgentID))
	if err != nil {
		logger.Error("Could not remove link to database: " + err.Error())
	}

	t.AgentUpdate(LinkAgent)
}

func (t *Teamserver) AgentHasDied(Agent *agent.Agent) bool {
	var AgentID, _ = strconv.ParseInt(Agent.NameID, 16, 64)

	return t.DB.AgentHasDied(int(AgentID))
}

func (t *Teamserver) AgentAdd(Agent *agent.Agent) []*agent.Agent {
	if Agent != nil {
		if t.WebHooks != nil {
			// snapshot the agent synchronously: ToMap takes the
			// per-agent lock while copying, so running it here keeps
			// the webhook goroutine from racing later mutations
			AgentMap := Agent.ToMap()

			// send the webhook asynchronously: a slow or dead endpoint
			// must not block agent registration
			go func() {
				if err := t.WebHooks.NewAgent(AgentMap); err != nil {
					logger.Error("Failed to send new-agent webhook: " + err.Error())
				}
			}()
		}
	}

	err := t.DB.AgentAdd(Agent)
	if err != nil {
		logger.Error("Could not add agent to database: " + err.Error())
	}

	return t.Agents.AgentsAppend(Agent)
}

func (t *Teamserver) AgentSendNotify(Agent *agent.Agent) {

	var pk packager.Package

	/* create a new agent package */
	pk = t.EventNewDemon(Agent)

	/* append the new agent event */
	t.EventAppend(pk)

	/* send it to every connected client */
	t.EventBroadcast("", pk)

}

func (t *Teamserver) AgentCallbackSize(DemonInstance *agent.Agent, i int) {
	var (
		Message = make(map[string]string)
		pk      packager.Package
	)

	Message["Type"] = "Good"
	Message["Message"] = fmt.Sprintf("Send Task to Agent [%v bytes]", i)

	OutputJson, _ := json.Marshal(Message)

	pk = events.Demons.DemonOutput(DemonInstance.NameID, agent.HAVOC_CONSOLE_MESSAGE, string(OutputJson))

	t.EventAppend(pk)
	t.EventBroadcast("", pk)
}

func (t *Teamserver) AgentInstance(AgentID int) *agent.Agent {
	for _, demon := range t.Agents.Snapshot() {
		var NameID, _ = strconv.ParseInt(demon.NameID, 16, 64)

		if AgentID == int(NameID) {
			return demon
		}
	}
	return nil
}

func (t *Teamserver) AgentLastTimeCalled(AgentID string, LastCallback string, Sleep int, Jitter int, KillDate int64, WorkingHours int32) {
	var (
		Output = map[string]string{
			"Last": LastCallback,
			"Sleep": fmt.Sprintf("%d", Sleep),
			"Jitter": fmt.Sprintf("%d", Jitter),
			"KillDate": fmt.Sprintf("%d", KillDate),
			"WorkingHours": fmt.Sprintf("%d", WorkingHours),
		}

		out, _ = json.Marshal(Output)
		pk     = events.Demons.DemonOutput(AgentID, agent.COMMAND_NOJOB, string(out))
	)

	t.EventBroadcast("", pk)
}

func (t *Teamserver) AgentExist(AgentID int) bool {
	for _, demon := range t.Agents.Snapshot() {
		var NameID, err = strconv.ParseInt(demon.NameID, 16, 64)
		if err != nil {
			logger.Debug("Failed to convert demon.NameID to int: " + err.Error())
			return false
		}

		if AgentID == int(NameID) {
			return true
		}
	}
	return false
}

func (t *Teamserver) AgentConsole(AgentID string, CommandID int, Output map[string]string) {
	var (
		out, _ = json.Marshal(Output)
		pk     = events.Demons.DemonOutput(AgentID, CommandID, string(out))
	)

	t.EventAppend(pk)
	t.EventBroadcast("", pk)
}

func (t *Teamserver) PythonModuleCallback(ClientID string, AgentID string, CommandID int, Output map[string]string) {
	var (
		out, _ = json.Marshal(Output)
		pk     = events.Demons.DemonOutput(AgentID, CommandID, string(out))
	)

	err := t.SendEvent(ClientID, pk)
	if err != nil {
		logger.Error("SendEvent error: ", err)
	}
}

func (t *Teamserver) AgentCallback(DemonID string, Time string) {
	var (
		Output = map[string]string{
			"Output": Time,
		}

		out, _ = json.Marshal(Output)
		pk     = events.Demons.DemonOutput(DemonID, agent.COMMAND_NOJOB, string(out))
	)

	t.EventBroadcast("", pk)
}

func (t *Teamserver) SendLogs() bool {
	return t.Flags.Server.SendLogs
}

func (t *Teamserver) GetDotNetPipeTemplate() string {
	// the profile may legitimately have no Demon block (service-only teamserver)
	PipeTemplate := ""
	if t.Profile.Config.Demon != nil {
		PipeTemplate = t.Profile.Config.Demon.DotNetNamePipe
	}

	// https://gist.github.com/realoriginal/d9178c9b071707fec2d6de89a63e4709

	PipeTemplates := []string{
		"Winsock2\\\\CatalogChangeListener-$#-0",
		"mojo.{pid}.{tid}.####################",
		"crashpad_{pid}_@@@@@@@@@@@@@@@@",
		"chrome.sync.{pid}.{tid}.########",
	}

	if PipeTemplate == "" {
		// crypto/rand instead of a globally-seeded math/rand: the picked
		// template feeds SMB pipe names for every assembly task
		if n, err := crand.Int(crand.Reader, big.NewInt(int64(len(PipeTemplates)))); err == nil {
			PipeTemplate = PipeTemplates[n.Int64()]
		} else {
			PipeTemplate = PipeTemplates[0]
		}
	}

	return PipeTemplate
}

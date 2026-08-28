package server

import (
	"Havoc/pkg/agent"
	"Havoc/pkg/logger"
	"fmt"
)

func (t *Teamserver) ServiceAgent(MagicValue int) agent.ServiceAgentInterface {
	for _, agentService := range t.Service.AgentsSnapshot() {
		if agentService.MagicValue == fmt.Sprintf("0x%x", MagicValue) {
			return agentService
		}
	}

	logger.Debug("Service agent not found")
	return nil
}

func (t *Teamserver) ServiceAgentExist(MagicValue int) bool {
	// t.Service is only initialized when the profile defines a Service
	// block; without this check any non-Demon magic value in a request
	// panics here
	if t.Service == nil {
		return false
	}

	for _, agentService := range t.Service.AgentsSnapshot() {
		if agentService.MagicValue == fmt.Sprintf("0x%x", MagicValue) {
			return true
		}
	}

	logger.Debug("Service agent not found")
	return false
}

package service

import (
	"Havoc/pkg/agent"
	"Havoc/pkg/packager"
	"Havoc/pkg/profile"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type ClientService struct {
	Conn      *websocket.Conn
	Mutex     sync.Mutex

	// Authenticated marks whether the login handshake completed; read by
	// the pre-auth watchdog, so only set/clear it while holding Mutex
	Authenticated bool

	// Responses maps request IDs to response channels; RespMtx guards it
	RespMtx   sync.Mutex
	Responses map[string]chan []byte
}

type Teamserver interface {
	AgentAdd(agent *agent.Agent) []*agent.Agent

	ListenerServiceExc2Add(Name, ExEndpoint string, client *ClientService) error
	ListenerStartNotify(Listener map[string]any)

	EventAppend(pk packager.Package) []packager.Package
	EventBroadcast(FromUser string, pk packager.Package)
	SendEvent(id string, pk packager.Package) error
}

type ConfigService struct {
	Endpoint string
	Name     string
	Password string
}

type Service struct {
	engine  *gin.Engine
	clients []*ClientService

	Config profile.ServiceConfig

	Teamserver Teamserver
	Agents     []*AgentService
	Listeners  []*ListenerService

	// stateMtx guards the clients, Agents and Listeners slices: every
	// service connection runs its own goroutine and the operator dispatch
	// path reads these too
	stateMtx sync.RWMutex

	// loginAttempts tracks failed service logins per source IP; without it
	// the auth handshake is one password guess per connection and
	// reconnecting gives an attacker unlimited tries
	loginAttempts map[string]*loginAttempt
	loginMtx      sync.Mutex

	// agentOwners binds service-registered agents (by NameID) to the
	// service client that registered them, so one service client cannot
	// task or inject output for another client's agent — or for
	// operator-registered (Demon) agents
	agentOwners    map[string]*ClientService
	agentOwnersMtx sync.Mutex

	Data struct {
		ServerAgents *agent.Agents
	}
}

type loginAttempt struct {
	Failures     int
	LockedUntil  time.Time
}

const (
	HeadRegister      = "Register"
	HeadRegisterAgent = "RegisterAgent"
	HeadAgent         = "Agent"
	HeadListener      = "Listener"

	BodyAgentRegister = "AgentRegister"
	BodyAgentTask     = "AgentTask"
	BodyAgentResponse = "AgentResponse"
	BodyAgentOutput   = "AgentOutput"
	BodyAgentBuild    = "AgentBuild"

	BodyListenerAdd      = "ListenerAdd"
	BodyListenerExC2     = "ListenerAddExC2"
	BodyListenerStart    = "ListenerStart"
	BodyListenerShutdown = "ListenerShutdown"
	BodyListenerTransmit = "ListenerTransmit"
)

func (c *ClientService) WriteJson(v any) error {
	var err error

	// bound each write so a stalled peer (dead NAT, half-open TCP) can't
	// pin the client mutex and queue every subsequent writer forever (M80)
	c.Mutex.Lock()
	_ = c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	err = c.Conn.WriteJSON(v)
	c.Mutex.Unlock()

	return err
}

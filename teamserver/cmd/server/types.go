package server

import (
	"Havoc/pkg/agent"
	"Havoc/pkg/db"
	"Havoc/pkg/packager"
	"Havoc/pkg/profile"
	"Havoc/pkg/service"
	"Havoc/pkg/webhook"
	"net"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type Listener struct {
	Name   string
	Type   int
	Config any
}

type Client struct {
	ClientID      string
	Username      string
	GlobalIP      string
	ClientVersion string
	Connection    *websocket.Conn
	Packager      *packager.Packager
	Authenticated bool
	SessionID     string
	Mutex         sync.Mutex
}

// LoginIP returns the host part of GlobalIP (RemoteAddr is "ip:port",
// and every connection gets a fresh source port, which would defeat
// per-IP login throttling). IPv6 sources are keyed on their /64 prefix:
// hosts routinely rotate through the whole prefix, which would otherwise
// defeat throttling the same way source ports do. IPv4 is unchanged.
func (c *Client) LoginIP() string {
	host := c.GlobalIP
	if h, _, err := net.SplitHostPort(c.GlobalIP); err == nil {
		host = h
	}

	if ip := net.ParseIP(host); ip != nil && ip.To4() == nil {
		// IPv6: mask everything past the first 8 bytes (the /64 prefix)
		return ip.Mask(net.CIDRMask(64, 128)).String()
	}

	// IPv4 or unparseable: use the raw host string
	return host
}

type Users struct {
	Name     string
	Password string
	Hashed   bool
	Online   bool
}

type serverFlags struct {
	Host string
	Port string

	Profile  string
	Verbose  bool
	Debug    bool
	DebugDev bool
	SendLogs bool
	Default  bool
}

type utilFlags struct {
	NoBanner bool
	Debug    bool
	Verbose  bool

	Test bool

	ListOperators bool
}

type TeamserverFlags struct {
	Server serverFlags
	Util   utilFlags
}

type Endpoint struct {
	Endpoint string
	Function func(ctx *gin.Context)
}

// defaultEventsListMax is the default cap for the in-memory event history
const defaultEventsListMax = 10000

const (
	// loginMaxFailures is the number of failed operator logins from one
	// source IP before it gets temporarily locked out
	loginMaxFailures = 5

	// loginLockout is how long a source IP is locked out after too many
	// failed operator logins
	loginLockout = 5 * time.Minute

	// loginAttemptsMax bounds the LoginAttempts map; past this size
	// expired entries are swept before new ones are recorded
	loginAttemptsMax = 10000
)

// LoginAttempt tracks failed operator logins from a single source IP
type LoginAttempt struct {
	Failures    int
	LockedUntil time.Time
}

type Teamserver struct {
	Flags      TeamserverFlags
	Profile    *profile.Profile
	Clients    sync.Map // map[string]*Client
	Users      []Users
	EventsList []packager.Package

	// EventsMutex guards EventsList append/remove/iterate
	EventsMutex sync.RWMutex

	// LoginMutex serializes the operator login flow so the
	// duplicate-username check and its assignment are atomic
	LoginMutex sync.Mutex

	// LoginAttempts tracks failed operator logins per source IP for
	// brute-force throttling
	LoginAttempts    map[string]*LoginAttempt
	LoginAttemptsMtx sync.Mutex

	Service    *service.Service
	WebHooks   *webhook.WebHook
	DB         *db.DB

	Server struct {
		Path   string
		Engine *gin.Engine
	}

	Agents    agent.Agents
	Listeners []*Listener

	// ListenersMutex guards Listeners add/remove/iterate
	ListenersMutex sync.RWMutex

	// EndpointsMutex guards Endpoints add/remove/iterate (the gin route
	// handler iterates while service agents can add/remove at runtime)
	EndpointsMutex sync.RWMutex

	Endpoints []*Endpoint

	Settings struct {
		Compiler64 string
		Compiler32 string
		Nasm       string
	}
}

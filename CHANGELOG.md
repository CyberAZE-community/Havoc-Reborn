# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Changed

- CI now skips runs when only non-code files change (`paths-ignore` for markdown, `LICENSE`, `.gitignore`, `assets/`, `profiles/`, `data/`).
- The per-agent SOCKS5 proxy now binds to `127.0.0.1` only instead of `0.0.0.0` (it was an unauthenticated open relay). Use port forwarding if remote access to the proxy port is needed. (#45)
- Service API: agents registered through the Service API are now bound to the service connection that registered them — tasking an agent or injecting console output for an agent owned by another connection (including operator-registered Demons) is rejected. Third-party agents that relied on cross-client tasking will need to register and task their own agents. (#52)
- The operator event history kept in memory (and replayed to every newly connected client) is now capped at 10000 entries; the oldest events are dropped beyond the cap. (#48)
- Shipped profiles no longer contain the well-known default credentials `password1234` / `service-password`; they use obvious `CHANGE-ME-*` placeholders instead, and the teamserver warns at startup about default/placeholder/weak-looking operator and Service API passwords. The SHA3-256 auth scheme itself is unchanged. (#57)
- `BehindRedir` HTTP listeners still trust `X-Forwarded-For` (that is the feature's trust model), but the value is now validated with `net.ParseIP` and the peer address is used as fallback for malformed values. (#58)

### Removed

- `RELEASE.md` (upstream historical changelog superseded by `CHANGELOG.md`) and `exception_mac.hpp` (a stray toml11 `exception.hpp` patch left at the repo root by an upstream merge; unreferenced by any build).

### Fixed

- Teamserver: post-auth RCE via the payload `Service Name` option — the value was interpolated unescaped into the `SERVICE_NAME` define of a `sh -c` compile command; it is now restricted to letters, digits, space, `.`, `-`, `_` and rejected otherwise. Also fixed the nasm path being used as a `fmt.Sprintf` format string. (#41)
- Teamserver: data races on `Agent` state — `JobQueue`, `Tasks` and `Downloads` are now guarded by a per-agent mutex (`AddRequest`/`RequestCompleted`/`IsKnownRequestID`, `AddJobToQueue`/`GetQueuedJobs`, the pivot job append, the `Download*` helpers, `task list`/`task clear`, and new `QueuedJobsLen`/`DownloadsLen` helpers). (#42)
- Teamserver: the Service API dispatch and `agent.RegisterInfoToInstance` ran unchecked type assertions on service-client-controlled JSON, panicking the handler on malformed messages; all such assertions are now comma-ok checked and rejected with a debug log. (#43)
- Teamserver: `log.Fatal` in `logr` (agent-influenced log-file open) and `db.ListenerCount` killed the whole teamserver; both now log/return errors instead. (#46)
- Teamserver: the payload compile directory under `/tmp` had a predictable time-seeded name; it is now created with `os.MkdirTemp` (unpredictable, 0700). (#47)
- Teamserver: the global agents slice was appended and iterated concurrently without synchronization; `AgentsAppend` now takes a write lock and all iterations use a new `Agents.Snapshot()` copy helper. (#49)
- Teamserver: Service API `SendResponse` blocked forever when a third-party agent never answered, and the `Responses` map was accessed without a lock; it now uses a mutex-guarded map, buffered channels, a 5-minute timeout, and non-blocking response delivery. (#50)
- Teamserver: the SOCKS reader goroutine busy-spun at 100% CPU while waiting for the agent to connect, `socks.Clients` was appended/iterated without a lock, the local socket leaked on EOF, and `SocksClient.Connected` was racy (now atomic). (#51)
- Teamserver: path-containment checks used bare `strings.HasPrefix` (bypassable by a sibling directory sharing the prefix); a `filepath.Rel`-based `logr.PathWithin` helper is now used for log/download/screenshot paths. (#53)
- Teamserver: the new-agent webhook was synchronous with no timeout (blocking agent registration), leaked the response body on success, and used the remote response as a `fmt.Errorf` format string; it is now asynchronous with a 10s timeout and a constant error format. (#59)
- Teamserver: `GetQueuedJobs` size accounting ignored the 12-byte per-job header, pre-built `job.Payload` blobs, and the terminating NUL of strings, letting queued jobs exceed `DEMON_MAX_RESPONSE_LENGTH`. (#62)
- Teamserver: `PortFwdGet` returned a pointer callers dereferenced after releasing the lock while `PortFwdClose` could nil/remove it concurrently; the port-forward operations now hold the lock across lookup and use (the blocking read grabs the connection under the lock and copies without it). (#68)
- Teamserver: `ToMap` temporarily mutated the shared `Pivots.Parent` during marshalling; concurrent calls could interleave, so `ToMap` is now serialized with a package-level mutex. (#69)
- Teamserver: `GenerateID`/`GenerateString` used time-seeded `math/rand` (predictable, colliding within the same nanosecond); both now use `crypto/rand` with unchanged output formats. (#70)
- Teamserver: the duplicate-login check raced the username assignment, letting two concurrent logins as the same operator through; the check, authentication and assignment now run under a login mutex. (#71)
- Teamserver: no operator login rate limiting existed; failed logins are now counted per source IP with a 5-minute lockout after 5 failures. (#73)
- Teamserver: the fake 404 page was read via a CWD-relative path; it is now embedded with `go:embed`. (#74)
- Teamserver: `pkg/db` prepared statements in `LinkExist`, `ParentOf` and `LinksOf` were never closed, leaking SQLite statement handles. (#92)
- Teamserver: `Parser.ParseBytes` converted the length prefix via `uint(ParseInt32())`, turning negative 32-bit lengths into ~4 GiB sizes; it now parses the length as signed and clamps to the remaining buffer. (#94)
- Teamserver: a panic in the operator websocket handler left a ghost (possibly authenticated) client whose username blocked reconnects; the recover path now closes the socket, emits the disconnect event and removes the client. (#103)
- Vendored `yaotl/hclsyntax` parser had unreachable code (`return nil, nil` after a switch whose branches all return), which failed `go vet ./...` and broke the new CI; the dead statement is removed.
- Demon: `ParserGetBytes` trusted the length prefix blindly, underflowing the parser length and returning out-of-bounds pointers; it now validates the length against the remaining buffer, and the command dispatcher stops on a malformed task buffer. `FS::Dir` no longer copies a fixed 520 bytes from a possibly NULL/short string, and the `MemFile` upload path validates `Size` (>4 GiB truncation) and chunk sizes against the allocated buffer. (#44)
- Demon: robustness bundle — `PackageTransmitNow` reused the advanced CTR IV for its restore-decrypt (corrupting the first-package retry after a failed connect), `DownloadGet` had an inverted loop condition (NULL deref), `SmbRecv`/`PivotPush` trusted the pipe package size for allocation (now capped at `PIPE_BUFFER_MAX` with allocation checks), `JobCheckList`/`JobKill` used job entries after `JobRemove` freed them, and `HwBpEngineRemove` corrupted the list head when removing the first breakpoint. (#60)
- Demon: working-hours handling compared hour fields directly, breaking overnight ranges (e.g. 22:00-06:00), and a sleep of 0 busy-spun the dispatcher without checking in; the comparison now uses minutes-of-day (overnight-safe), sleep 0 overrides working hours as documented, and the sleep-until-start computation uses correct millisecond units. (#61)
- Demon: `PROC_MODULES` read the target process's `FullDllName.Length` into fixed `MAX_PATH` stack buffers without clamping (stack overflow); the read and the wide-to-char conversion are now bounded to the destination capacity. (#81)
- Demon: `listDir` concatenated the search path and filename (each up to `MAX_PATH`) into a fixed `MAX_PATH+1` heap buffer without capacity accounting; over-long subdirectory paths are now skipped. (#82)
- Demon: the Coffee BOF vectored exception handler was process-wide and redirected the RIP of any faulting thread; it now only handles exceptions on the BOF thread with an address inside the BOF image, and is removed on every exit path. (#83)
- Demon: `TokenQueryOwner` with the default flag wrote the domain\user separator over the first character of the username (e.g. `DOMAIN\dmin`); it now replaces the domain's NUL terminator. (#84)
- Demon: `ListTokens`/`AddUserToken` could append past the `BUF_SIZE`-entry token table; entries beyond the capacity are now refused. (#85)
- Demon: Coffee symbol names from the untrusted BOF symbol table were copied into fixed 1024-byte stack buffers without length checks; copies are now bounded and over-long names rejected. (#86)
- Demon: Coffee relocations used `SymbolTableIndex`, `SectionNumber` and relocation offsets from the untrusted BOF without bounds checks; they are now validated against the symbol table, section list and target section. (#87)
- Demon: the sleep-obfuscation ROP chain is now checked for NULL `Rip` entries (e.g. after a jmp-gadget lookup miss) and aborts into the default `WaitForSingleObjectEx` sleep instead of crashing the timer thread. (#88)
- Demon: length-prefixed strings from the packet buffer were used as NUL-terminated C strings; `ParserGetString`/`ParserGetWString` now verify termination within the length prefix (the teamserver always terminates, so legitimate traffic is unaffected). (#89)
- Demon: the SMB pivot named pipe granted full access to the Everyone SID with only the 32-bit DemonId as authentication; the DACL is now restricted to the process owner (pivot children run as the same user, wire protocol unchanged). (#90)
- Client: the Python API `AllocMov` macro allocated `QString::size()+1` (UTF-16 code units) and then `strcpy`'d the UTF-8 text, overflowing the heap buffer for any non-ASCII value; it now allocates `strlen(src)+1`. `DemonClass_dealloc`/`AgentClass_dealloc` also `Py_XDECREF`'d plain `char*` members (type confusion); they are now `free`'d. (#39)

## [0.7.2] - 2026-08-07

### Added

- GitHub Actions CI (`.github/workflows/ci.yml`) on push/PR to `dev` and `main`: a teamserver job (`go build ./...` + `go vet ./...` on Go 1.21) and a client job (Qt5/websocket/python3-dev deps, submodule init, Release cmake build). (#111)
- `SECURITY.md` with a vulnerability-reporting policy (GitHub Security Advisories; only the latest release is supported). (#101)

### Fixed

- Repo hygiene: `.gitignore` now covers AI-tooling artifacts (`node_modules/`, `package.json`, `package-lock.json`, `.opencode/`) and whitelists `SECURITY.md` so `git add -A` no longer risks committing them. (#99)
- Docs drift: `teamserver/README.md` now requires Go 1.21+ and documents the real build/run paths (`make ts-build` from the repo root, `./havoc` binary); `WIKI.MD` no longer demands Python 3.10 (any recent Python 3 / `python3-dev` works, currently 3.12) and points clones and the Modules links at this fork; `CONTRIBUTING.MD` now matches the `dev`-branch workflow in `AGENTS.md` instead of telling contributors to PR into `main`. (#101)

- Makefile: version stamping was broken — `$(git rev-parse HEAD)` was expanded by make as an undefined variable (always empty) and the `-X cmd.VersionCommit` ldflags path matched no existing variable; the shell call is now `$(shell git rev-parse HEAD)`, the flag targets `Havoc/cmd.VersionCommit`, and the teamserver declares and prints that commit in its version output. (#100)
- Demon: `RandomNumber32()` re-seeded `RtlRandomEx` from `NtGetTickCount()` on every call and its result was stored byte-wise, so the transport AES key/IV were 32/16 copies of a single byte generated within one tick; it now uses the system CSPRNG `ntdll!RtlGenRandom` (falling back to `RtlRandomEx`) and the key/IV loops use the full 32-bit output across 4 bytes. (#38)
- Teamserver: inverted nil check in the SMB pivot output loop dereferenced a nil pivot agent and panicked when a pivot in the chain was no longer registered; the loop now breaks when the pivot instance cannot be found. (#75)
- Teamserver: `agent.ParseHeader` rejected buffers holding exactly one more 4-byte field (`Parser.Length() > 4`), truncating minimally-sized headers; the bounds checks are now `>= 4`. (#93)
- Teamserver: removed the leftover unauthenticated static file handler that served `./bin/static` at `/home` (plus the `/` → `home/` redirect pointing at it); nothing in the fork ships or references that directory. (#63)
- Teamserver: `logr.NewLogr` deleted the entire existing log directory tree (`os.RemoveAll`) whenever the path already existed; it now only creates the directory if missing and never removes prior logs. (#91)
- Teamserver: fixed all `go vet` findings in first-party code — self-assignments in `demons.go`, `fmt.Sprintf` calls with missing args, `fmt.Sprint` misused with `%d` format verbs in `pkg/socks`, and a literal `%s` passed to `logger.Debug` in `pkg/certs`. (#56)
- Teamserver: `getWindowsVersionString` indexed up to `OsVersion[4]` without a length check, panicking on third-party agents that report a short OS version array; it now returns "Unknown" for arrays shorter than 5 elements. (#64)
- Teamserver: removed the dead `Module` (0x6) and `Misc` (0x7) packager type constants, which duplicated the live `HostFile` (0x6) and `Session` (0x7) IDs; no code referenced them and no active IDs were renumbered. (#66)
- Teamserver: failed payload builds left world-writable (`0777`) temp directories behind in `/tmp`; the build directory is now created `0700` and removed on all build paths, success or failure. (#67)
- Teamserver: the HTTP listener derived the external IP with `strings.Split(RemoteAddr, ":")`, producing `"["` for IPv6 peers; it now uses `net.SplitHostPort` and strips any IPv6 zone suffix. (#72)
- Teamserver: a matching `HostHeader` (or `X-Forwarded-Host`) re-validated requests that had already failed a required profile header check; a host match can no longer override a header mismatch. (#107)
- Teamserver (Service API): closing a service client removed only its first registered agent and listener, leaking the rest; all agents and listeners owned by the client are now unregistered. (#109)
- Teamserver: restoring listeners from the database used unchecked type assertions on the stored JSON config, so one corrupted row crashed the teamserver at startup; malformed entries are now logged and skipped. (#108)
- Teamserver: `ServerFinished` was declared but never initialized, so a failed TLS startup deadlocked on the nil channel instead of shutting down; it is now created with `make(chan bool)`. (#104)
- Teamserver: the database was opened twice at startup (`NewTeamserver` and again in `Start`), leaking the first handle; `Start` now reuses the already-open database. (#105)
- Teamserver: `db.AgentExist` deferred `query.Close()` before the error check (nil `*sql.Rows` dereference on query failure), never closed its prepared statement, and only treated exactly-one-row as existing; defers now follow their error checks, the statement is closed, and any positive count means the agent exists. (#106)
- `teamserver/Install.sh` no longer downloads the musl.cc cross toolchains (the URLs are dead); it installs the distro `mingw-w64` packages instead and skips `sudo` when running as root. `profiles/havoc.yaotl` now points `Teamserver.Build` at the system compilers (`/usr/bin/x86_64-w64-mingw32-gcc`, `/usr/bin/i686-w64-mingw32-gcc`), so fresh setups no longer fail with "Compiler x64 path doesn't exist".
- Client: `Connector` inherited from `QTcpSocket` even though it only ever uses an internal `QWebSocket`; the dead socket base is replaced with `QObject`, and `ErrorString` is now value-initialized (`QString()`) instead of being assigned `nullptr`. (#98)
- Client: the teamserver logger's `QDialog` was created unparented and leaked for every teamserver tab; it is now parented to the tab session widget. `DispatchListener::Remove` also dereferenced `ListenerTableWidget` without a null check and now guards it. (#95)
- Client: named the `InitConnection` sub-event constants `InitInfo` (0x4) and `Profile` (0x5) to match the teamserver protocol, replacing the magic `case 0x5` literal in `DispatchInitConnection`; no values were renumbered and the teamserver side is unchanged. (#102)
- Client: `DispatchTeamserver`'s `Logger` case fell through into the `Profile` case for lack of a `break`, and `Util::gen_random` generated TaskIDs by shuffling a 16-character alphabet (no repeated characters, silently truncated above 16 chars); the `break` was added and TaskIDs are now drawn per-character from `QRandomGenerator`. (#63)

### Changed

- The client Modules are now cloned from the fork-owned [CyberAZE-community/Modules](https://github.com/CyberAZE-community/Modules) repository (`dev` branch) instead of upstream `HavocFramework/Modules`.

## [0.7.1] - 2026-08-07

### Added

- **Selected fixes from upstream `dev` branch** (changes reviewed and applied selectively):
  - HTTP listener `HostHeader` profile field + Host / `X-Forwarded-Host` request validation
  - Profile-declared listener proxy settings are now actually applied at teamserver startup (`Proxy.Type` field added)
  - Demon: use `WINHTTP_ACCESS_TYPE_AUTOMATIC_PROXY` for proxy autodetect (`TransportHttp.c`)
  - Demon: `RandomNumber32` refactor — removed redundant modulus/double-call (`RtlRandomEx` already bounds the value)
  - Client: fix new-session event timestamp (use the session's first call-in time)
  - Client: fix command argument parsing for quoted strings with spaces (`ConsoleInput.cc`)
  - Client: fix listener custom-header handling — headers are now delimited with CRLF instead of `, `, so commas inside header values work
  - Teamserver: fix elevated-permission parsing for third-party (service) agents in `RegisterInfoToInstance`
  - Teamserver: close leaked file handle in logr
  - Event Viewer: initial size / resizing fixes (POSIX)
  - "Extentions" → "Extensions" spelling fixes (Store widget, menus)
  - makefile: `SHELL := /bin/bash`, `mkdir -p`, recursive Modules clone, `client-build-mac` target (with `config_mac.toml` / `exception_mac.hpp`)
  - Not applied: upstream `go.mod` downgrade (ours is newer), README changes (ours is newer), `hash_func.py` credit URL (ours is newer), and the misnamed `Dockerfiledocke` (duplicate of `teamserver/Teamserver-Dockerfile`)

- **Loot file download to client** — downloaded files can now be pulled from the teamserver and saved to disk directly from the client. New `Loot` packager event type `0x11` (`GetFile` / `SendFile` / `Error`): the client requests a looted file, the teamserver locates it under `data/loot/<ts>/agents/<AgentID>/Download/` (with path-traversal protection) and returns the content base64-encoded to the requesting client only. The Loot widget's Downloads table now has a "Get file" context-menu action.
- `teamserver/pkg/events/loot.go` — `events.Loot.SendFile` / `events.Loot.SendError` constructors (marked OneTime, not replayed to newly connected clients).

### Fixed

- Loot widget: the screenshot context menu never appeared (signal connected to the wrong widget) and its "Download" action was never connected to a handler. Right-clicking a screenshot now shows a working "Get file" action that writes the BMP to a user-chosen path.
- `teamserver/Install.sh`: the guard checked the nonexistent `dir/x86_64-w64-mingw32-cross`, so the toolchain install/extract block re-ran every time; it now checks `data/x86_64-w64-mingw32-cross` where the tarball actually extracts.
- `teamserver/Teamserver-Dockerfile`: no longer clones the abandoned upstream `HavocFramework/Havoc` repo; it now `COPY`s this fork's local source (build context = repository root) and builds the teamserver from it, with Go bumped to 1.21 per `teamserver/go.mod`.
- Root `makefile`: `ts-build` no longer runs `sudo setcap` unconditionally (only when invoked as `make ts-build SETCAP=1`); `client-build` now pins the Modules clone to the `dev` branch explicitly instead of using the current local branch name.
- Teamserver hardening and internal bug fixes (no protocol/DB/profile changes):
  - Unauthenticated panic/DoS: per-connection goroutines in `handleRequest` (operator websocket) and service `handleConnection` are now wrapped in `defer`/`recover`; the login/auth path no longer does unchecked `.(string)` assertions on attacker-controlled JSON (comma-ok checks reject malformed packages), and `ClientAuthenticate` handles a missing/non-string `Password` gracefully.
  - Duplicate-operator check compared the not-yet-set local `client.Username` (always empty); it now compares each connected client's stored username.
  - `SendEvent` returned on write error without unlocking the client write mutex, permanently deadlocking later sends to that client (the old "seems to crash the server" workaround); the mutex is now released via `defer` on all paths.
  - `EventAppend` appended the event twice to the returned slice and `EventRemove` recomputed the removal on the already-truncated slice (dropping an extra event); both now return the actual list.
  - `Teamserver.Listeners` add/remove/iterate paths are now guarded by a `sync.RWMutex`.
  - `parser.ParseInt32`/`ParseInt64`/`ParseBool` read `Length-width` bytes instead of exactly the integer width when more data remained, corrupting every multi-field parse.
  - HTTP and external listener request bodies are now capped at 16 MiB (`http.MaxBytesReader`).
  - Removed the fingerprintable `X-Havoc: true` header from the fake-404 response.
  - Teamserver and listener TLS key/cert files are now written with `0600` instead of `0644`.
- Client: fix 1-byte heap overflow in the duplicated `AllocMov` macro (all 7 copies: `PyDemonClass.cc`, `PyAgentClass.hpp`, `Event.cc`, `PyWidgetClass.cc`, `PyDialogClass.cc`, `PyTreeClass.cc`, `PyLoggerClass.cc`) — it allocated `size` bytes and then `strcpy`'d the trailing NUL past the buffer; now allocates `size + 1`.
- Client Python API: fix off-by-one in `GetAgents()` that skipped the first service agent and left the last list slot uninitialized, crashing Python consumers.
- Client packager: `case 0x5` (teamserver IPs / Demon config) in `DispatchInitConnection` fell through to `default: return false`; the connection-error message box also had inverted logic (showed the empty message and hid the real one).
- Client connector: every decoded `Package` from `DecodePackage` was leaked; it is now freed after `DispatchPackage` returns.
- Client Python API: `RegisterCommand` / `RegisterModule` iterated a copy of the session vector, so `AutoCompleteAdd` mutations never reached the real sessions; now iterate a reference.
- Client Python console widget: the error path in `RunCode` returned before `emb::reset_stdout()`, leaving Python stdout permanently redirected (and capturing into a dangling reference); `reset_stdout()` now runs on all paths.
- Client demon console: agent-controlled command output/messages were inserted as unescaped HTML (`CommandOutput.cc` → `AppendRaw`), allowing HTML injection into the operator console; agent data is now escaped with `toHtmlEscaped()` at the source while intentional client-side formatting (colors) is preserved.
- Teamserver Loot `GetFile` dispatch no longer does bare `.(string)` assertions on `AgentID`/`FileName` (malformed packages are rejected gracefully), the loot path-traversal prefix check is now path-boundary-aware, and loot files over 64 MiB are refused instead of being read fully into memory.
- `agent.go` register parsing: `Process Elevated` no longer panics on string-typed values (accepts `1`/`true` strings as well as numbers).
- HTTP listener `HostHeader` validation now compares against the request host with any port stripped, so a port-less profile value no longer drops all agent requests.
- Client module-command dispatch: `JoinAtIndexPreserveQuotes` no longer joins the empty sentinel element (trailing space in the final argument), and the file-parameter check uses `QByteArray::isNull()` instead of an always-true `!= nullptr` comparison so missing files are properly reported.
- `ListenerStart` no longer performs config type assertions while holding `ListenersMutex`.

### Changed

- **README rewritten for the fork** — identifies this as the community fork, points all documentation/wiki/issue links to `CyberAZE-community/Havoc` (wiki: https://github.com/CyberAZE-community/Havoc/wiki), adds a fork-changes summary and corrected quick-start (Go 1.21+, any Python 3), and drops the outdated Python 3.10 requirement. `WIKI.MD` now opens with a pointer to the maintained GitHub wiki. The repo homepage was set to the wiki.
- **Client Python support modernized** — the client now builds against any Python 3 (verified with 3.12). `client/CMakeLists.txt` uses modern `find_package(Python 3 COMPONENTS Interpreter Development)` instead of the deprecated `FindPythonLibs` module that effectively pinned older Python versions. The "Python 3.10 required" documentation note was outdated.

# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Fixed

- Teamserver: inverted nil check in the SMB pivot output loop dereferenced a nil pivot agent and panicked when a pivot in the chain was no longer registered; the loop now breaks when the pivot instance cannot be found. (#75)
- Teamserver: `agent.ParseHeader` rejected buffers holding exactly one more 4-byte field (`Parser.Length() > 4`), truncating minimally-sized headers; the bounds checks are now `>= 4`. (#93)
- Teamserver: the database was opened twice at startup (`NewTeamserver` and again in `Start`), leaking the first handle; `Start` now reuses the already-open database. (#105)
- Teamserver: `db.AgentExist` deferred `query.Close()` before the error check (nil `*sql.Rows` dereference on query failure), never closed its prepared statement, and only treated exactly-one-row as existing; defers now follow their error checks, the statement is closed, and any positive count means the agent exists. (#106)
- `teamserver/Install.sh` no longer downloads the musl.cc cross toolchains (the URLs are dead); it installs the distro `mingw-w64` packages instead and skips `sudo` when running as root. `profiles/havoc.yaotl` now points `Teamserver.Build` at the system compilers (`/usr/bin/x86_64-w64-mingw32-gcc`, `/usr/bin/i686-w64-mingw32-gcc`), so fresh setups no longer fail with "Compiler x64 path doesn't exist".

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

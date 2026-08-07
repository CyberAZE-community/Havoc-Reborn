# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

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

### Changed

- **Client Python support modernized** — the client now builds against any Python 3 (verified with 3.12). `client/CMakeLists.txt` uses modern `find_package(Python 3 COMPONENTS Interpreter Development)` instead of the deprecated `FindPythonLibs` module that effectively pinned older Python versions. The "Python 3.10 required" documentation note was outdated.

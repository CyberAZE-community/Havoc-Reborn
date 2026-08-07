# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added

- **Loot file download to client** — downloaded files can now be pulled from the teamserver and saved to disk directly from the client. New `Loot` packager event type `0x11` (`GetFile` / `SendFile` / `Error`): the client requests a looted file, the teamserver locates it under `data/loot/<ts>/agents/<AgentID>/Download/` (with path-traversal protection) and returns the content base64-encoded to the requesting client only. The Loot widget's Downloads table now has a "Get file" context-menu action.
- `teamserver/pkg/events/loot.go` — `events.Loot.SendFile` / `events.Loot.SendError` constructors (marked OneTime, not replayed to newly connected clients).

### Fixed

- Loot widget: the screenshot context menu never appeared (signal connected to the wrong widget) and its "Download" action was never connected to a handler. Right-clicking a screenshot now shows a working "Get file" action that writes the BMP to a user-chosen path.

### Changed

- **Client Python support modernized** — the client now builds against any Python 3 (verified with 3.12). `client/CMakeLists.txt` uses modern `find_package(Python 3 COMPONENTS Interpreter Development)` instead of the deprecated `FindPythonLibs` module that effectively pinned older Python versions. The "Python 3.10 required" documentation note was outdated.

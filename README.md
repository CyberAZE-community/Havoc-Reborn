<div align="center">
  <img width="125px" src="assets/Havoc.png" />
  <h1>Havoc (community fork)</h1>
  <br/>

  <p><i>Havoc is a modern and malleable post-exploitation command and control framework, originally created by <a href="https://twitter.com/C5pider">@C5pider</a>.</i></p>
  <p><b>This is a community-maintained fork of <a href="https://github.com/HavocFramework/Havoc">HavocFramework/Havoc</a>, which is no longer actively developed upstream. This fork continues maintenance: bug fixes, security hardening, and select features. See <a href="CHANGELOG.md">CHANGELOG.md</a> for what has changed.</b></p>
  <br />

  <img src="assets/Screenshots/FullSessionGraph.jpeg" width="90%" /><br />
  <img src="assets/Screenshots/MultiUserAgentControl.png" width="90%" /><br />

</div>

Havoc is a post-exploitation command and control framework: a Go **teamserver** that runs listeners and builds payloads, a Qt **client** for operators (multiplayer, scriptable in Python), and **Demon**, a Windows implant written in C/ASM with sleep obfuscation, indirect syscalls, token manipulation, SMB pivots, and BOF/.NET support.

This fork picks up where upstream left off. It stays protocol-compatible with upstream 0.7 while landing security fixes, stability work, and select features — everything we change is recorded in [CHANGELOG.md](CHANGELOG.md), and planned work lives in the [issue tracker](https://github.com/CyberAZE-community/Havoc/issues).

To get running, see the **[build instructions in the wiki](https://github.com/CyberAZE-community/Havoc/wiki/Building-From-Source)** (or the quick start below). The wiki also documents the internals: teamserver/client/Demon architecture, the client↔teamserver protocol, the Service API for writing third-party agents, and the full profile reference.

### Quick Start

Havoc works well on Debian 11/12, Ubuntu 20.04/22.04/24.04 and Kali Linux. You'll need Go 1.21+, Qt5 dev packages, and any Python 3 (the client auto-detects the newest installed Python — the old "Python 3.10 required" note no longer applies).

```bash
# teamserver (installs deps incl. MinGW cross toolchains, builds ./havoc)
make ts-build

# client (initializes submodules, CMake build, fetches the Modules repo)
make client-build

# run
./havoc server --profile profiles/havoc.yaotl
./havoc client
```

Full details, dependency lists, Docker build, and known build issues: **[Building From Source](https://github.com/CyberAZE-community/Havoc/wiki/Building-From-Source)**.

If you run into problems, check the [Issues](https://github.com/CyberAZE-community/Havoc/issues) list for this fork (upstream issues were triaged and ported where still relevant). Please report bugs against **this fork**, not upstream.

---

### Fork changes (highlights)

- Loot file download from the client (new packager event type `0x11`)
- Teamserver hardening: panic-resistant request handling, body size limits, listener mutex, TLS key permissions, removal of the fingerprintable `X-Havoc` response header
- Client fixes: memory-safety fixes in the Python API, connection error handling, console HTML-injection fix
- Modernized Python detection (any Python 3), Dockerfile builds this fork's source
- Cherry-picked upstream `dev` fixes: HostHeader validation, proxy config application, and more

See [CHANGELOG.md](CHANGELOG.md) for the full list. Roadmap and known issues are tracked in [Issues](https://github.com/CyberAZE-community/Havoc/issues).

### Features

#### Client

> Cross-platform UI written in C++ and Qt

- Modern, dark theme based on [Dracula](https://draculatheme.com/)


#### Teamserver

> Written in Golang

- Multiplayer
- Payload generation (exe/shellcode/dll)
- HTTP/HTTPS listeners
- Customizable C2 profiles
- External C2

#### Demon

> Havoc's flagship agent written in C and ASM

- Sleep Obfuscation via [Ekko](https://github.com/Cracked5pider/Ekko), Ziliean or [FOLIAGE](https://github.com/SecIdiot/FOLIAGE)
- x64 return address spoofing
- Indirect Syscalls for Nt* APIs
- SMB support
- Token vault
- Variety of built-in post-exploitation commands
- Patching Amsi/Etw via Hardware breakpoints
- Proxy library loading
- Stack duplication during sleep.

<div align="center">
  <img src="assets/Screenshots/SessionConsoleHelp.png" width="90%" /><br />
</div>

#### Extensibility

- [External C2](https://github.com/CyberAZE-community/Havoc/wiki/Listeners#external-c2)
- Custom Agent Support — see [Building a Third-Party Agent](https://github.com/CyberAZE-community/Havoc/wiki/Building-a-Third-Party-Agent)
  - [Talon](https://github.com/HavocFramework/Talon) (upstream reference agent)
- [Python API](https://github.com/HavocFramework/havoc-py)
- [Modules](https://github.com/CyberAZE-community/Modules) (forked)

---

### Contributing

See [CONTRIBUTING.MD](CONTRIBUTING.MD) and [AGENTS.md](AGENTS.md) (the latter documents the build/verify rules, code conventions, and which wiki pages must be kept in sync with which changes).

### Credits

Original framework by [@C5pider](https://twitter.com/C5pider) and the upstream contributors — see [CREDITS.md](CREDITS.md). This fork is community-maintained; the upstream [Havoc Discord](https://discord.gg/z3PF3NRDE5) remains the general community chat.

### Note

Please do not open any issues regarding detection.

The Havoc Framework hasn't been developed to be evasive. Rather it has been designed to be as malleable & modular as possible. Giving the operator the capability to add custom features or modules that evades their targets detection system.

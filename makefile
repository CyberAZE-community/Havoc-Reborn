SHELL := /bin/bash
ifndef VERBOSE
.SILENT:
endif

# main build target. compiles the teamserver and client
all: ts-build client-build

# teamserver building target
ts-build:
	@ echo "[*] building teamserver"
	@ ./teamserver/Install.sh
	@ cd teamserver; GO111MODULE="on" go build -ldflags="-s -w -X cmd.VersionCommit=$(git rev-parse HEAD)" -o ../havoc main.go
	@ # optional: run with SETCAP=1 (e.g. `make ts-build SETCAP=1`) to allow running the server as a regular user on privileged ports
	@ if [ "$(SETCAP)" = "1" ]; then sudo setcap 'cap_net_bind_service=+ep' havoc; fi

dev-ts-compile:
	@ echo "[*] compile teamserver"
	@ cd teamserver; GO111MODULE="on" go build -ldflags="-s -w -X cmd.VersionCommit=$(git rev-parse HEAD)" -o ../havoc main.go 

ts-cleanup: 
	@ echo "[*] teamserver cleanup"
	@ rm -rf ./teamserver/bin
	@ rm -rf ./data/loot
	@ rm -rf ./data/x86_64-w64-mingw32-cross 
	@ rm -rf ./data/havoc.db
	@ rm -rf ./data/server.*
	@ rm -rf ./teamserver/.idea
	@ rm -rf ./havoc

# client building and cleanup targets 
client-build:
	@ echo "[*] building client"
	@ git submodule update --init --recursive
	@ mkdir -p client/Build; cd client/Build; cmake ..
	@ if [ -d "client/Modules" ]; then echo "Modules installed"; else git clone --recurse-submodules https://github.com/CyberAZE-community/Modules client/Modules --single-branch --branch dev; fi # fork-owned Modules repo, dev branch
	@ cmake --build client/Build -- -j 4

client-build-mac:
	@ echo "[*] building client"
	@ git submodule update --init --recursive
	@ mkdir -p client/Build; cd client/Build; cmake ..
	@ if [ -d "client/Modules" ]; then echo "Modules installed"; else git clone --recurse-submodules https://github.com/CyberAZE-community/Modules client/Modules --single-branch --branch dev; fi # fork-owned Modules repo, dev branch
	@ rm client/external/toml/toml/exception.hpp ; cp exception_mac.hpp client/external/toml/toml/exception.hpp
	@ cmake --build client/Build -- -j 4

client-cleanup:
	@ echo "[*] client cleanup"
	@ rm -rf ./client/Build
	@ rm -rf ./client/Bin/*
	@ rm -rf ./client/Data/database.db
	@ rm -rf ./client/.idea
	@ rm -rf ./client/cmake-build-debug
	@ rm -rf ./client/Havoc
	@ rm -rf ./client/Modules


# cleanup target 
clean: ts-cleanup client-cleanup
	@ rm -rf ./data/*.db
	@ rm -rf payloads/Demon/.idea

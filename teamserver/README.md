# Havoc Teamserver

Source code of Havoc teamserver. Written in Golang.


### Build the Teamserver
- **Pre-requisites**
	1. Go 1.21+ (see `go.mod`)
	2. The mingw-w64 cross compilers and nasm for building agents (installed by `Install.sh`, which the root makefile runs automatically)
- **Native**
	- From the repository root (`Havoc/`), run:
		1. `make ts-build`
	- That's it! If it ran successfully to completion, you should now have a compiled `havoc` binary in the repository root.
	- Alternatively, from this folder (`Havoc/teamserver/`): `go build -o ../havoc main.go`
	- Example use with a prewritten profile (from the repository root): `./havoc server --profile profiles/havoc.yaotl --verbose`
	- Example use with default profile: `./havoc server --default --verbose`
- **Docker**
	- To build the Teamserver using a local Docker container, run the following commands from the repository root (assuming you have Docker installed):
		1. Build the Dockerfile:
			* `sudo docker build -t havoc-teamserver -f teamserver/Teamserver-Dockerfile .`
		2. (Optional) Create a persistent data volume for the container:
			* `sudo docker volume create havoc-c2-data`
		3. Run the container:
			* `sudo docker run -it -d -v havoc-c2-data:/data havoc-teamserver`


### Run the Teamserver
- **Base:**
	- The teamserver binary is `./havoc` in the repository root:
		* `./havoc -h`
		* `./havoc server --profile profiles/havoc.yaotl -v`
		* `./havoc server --default -v`
- **Docker**
	- We can run the teamserver completely from within a container!
	1. Build the container:
		* `sudo docker build -t havoc-teamserver -f teamserver/Teamserver-Dockerfile .` (from the repository root)
	2. Launch the container (be sure to change the port mapping to match your environment):
		* `sudo docker run -p40056:40056 -p 443:443 -it -d -v havoc-c2-data:/data havoc-teamserver`
	3. Access the teamserver at `localhost:40056` using your Havoc client.

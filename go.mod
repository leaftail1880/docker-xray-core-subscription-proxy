module xray-balancer

go 1.26

require github.com/xtls/xray-core v1.260327.0

require (
	github.com/pires/go-proxyproto v0.11.0 // indirect
	github.com/sagernet/sing v0.5.1 // indirect
	golang.org/x/net v0.52.0
	golang.org/x/sys v0.42.0 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
	pira/x2j v0.0.5
)

replace pira/x2j => ./x2j

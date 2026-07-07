module go.f0o.dev/version-intercept/autopin/connect

go 1.26

require (
	connectrpc.com/connect v1.20.0
	go.f0o.dev/version-intercept v0.0.0
	google.golang.org/protobuf v1.36.11
)

replace go.f0o.dev/version-intercept => ../../

module go.f0o.dev/version-intercept/autopin/connect

go 1.26

require (
	connectrpc.com/connect v1.18.1
	go.f0o.dev/version-intercept v0.0.0
	google.golang.org/protobuf v1.36.8
)

replace go.f0o.dev/version-intercept => ../../

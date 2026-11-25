module go.f0o.dev/version-intercept/autopin/connect

go 1.26.0

require (
	connectrpc.com/connect/v2 v2.0.2
	go.f0o.dev/version-intercept v0.0.0
	google.golang.org/protobuf v1.36.11
)

replace go.f0o.dev/version-intercept => ../../

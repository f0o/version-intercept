module go.f0o.dev/version-intercept/autopin/grpc

go 1.26

require (
	go.f0o.dev/version-intercept v0.0.0
	google.golang.org/grpc v1.68.1
	google.golang.org/protobuf v1.36.8
)

require (
	golang.org/x/net v0.55.0 // indirect
	golang.org/x/sys v0.45.0 // indirect
	golang.org/x/text v0.37.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240903143218-8af14fe29dc1 // indirect
)

replace go.f0o.dev/version-intercept => ../../

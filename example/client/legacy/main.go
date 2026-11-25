// Command legacy is the T1 client: it was code-generated at T0 from the frozen
// 1.0 proto snapshot (example/proto/legacy). Its generated descriptor carries
// service_versioning{max:"1.0"}, so it auto-pins to microversion 1.0 purely by
// reading its OWN compiled-in annotation — there is no hardcoded version string
// anywhere in this program. Its types also physically lack the 2.0
// title/language/served_by fields and the Farewell method, so it could not use
// newer features even if it wanted to. It nevertheless interoperates with the
// same modern server on the same path.
package main

import (
	"context"
	"fmt"
	"os"

	"connectrpc.com/connect/v2"

	greeterv1 "go.f0o.dev/version-intercept/example/gen-legacy/greeter/v1"
	"go.f0o.dev/version-intercept/example/gen-legacy/greeter/v1/greeterv1connect"
	"go.f0o.dev/version-intercept/example/internal/exampleclient"
)

// serviceDescriptor is this client's OWN frozen service descriptor, from which
// the auto-pinned microversion is read.
var serviceDescriptor = greeterv1.File_greeter_v1_greeter_proto.Services().ByName("GreeterService")

func main() {
	autoPin, pinned := exampleclient.AutoPinInterceptor(serviceDescriptor)
	client := greeterv1connect.NewGreeterServiceClient(exampleclient.NewConnectClient(autoPin))

	fmt.Println("legacy client: T1, compiled from the frozen 1.0 proto snapshot.")
	fmt.Printf("auto-pinned microversion (read from its own descriptor) = %s\n", pinned)
	fmt.Println("its generated types have no title/language/served_by/Farewell at all.")

	ctx, callInfo := connect.NewClientContext(context.Background())
	resp, err := client.Hello(ctx, &greeterv1.HelloRequest{
		Name: "Ada",
	})
	if err != nil {
		fmt.Printf("\n  \u2717 Hello failed: %v\n", err)
		fmt.Println("\nRESULT: FAIL \u2014 the T1 client could not reach the modern server.")
		os.Exit(1)
	}

	fmt.Printf("\n  \u2713 Hello succeeded: message=%q (server served %s)\n", resp.GetMessage(), callInfo.ResponseHeader().Get(exampleclient.RequestHeader))
	fmt.Println("\nRESULT: PASS \u2014 the T1 client auto-negotiated 1.0 and interoperated.")
}

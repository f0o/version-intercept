// Command grpc is the raw-gRPC (google.golang.org/grpc) example client. It dials
// the gRPC example server (grpcserver, default :8081) using the autopingrpc
// client interceptors, which stamp the Api-Version metadata: auto-pinned from the
// modern service descriptor (2.1) by default, or an explicit override. It shows
// microversion negotiation, method-lifecycle rejection, field gating, and
// server-streaming — all over raw gRPC rather than ConnectRPC.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	autopingrpc "github.com/f0o/version-intercept/autopin/grpc"
	greeterv1 "github.com/f0o/version-intercept/example/gen/greeter/v1"
)

// grpcAddr is the raw-gRPC example server address.
const grpcAddr = "localhost:8081"

// serviceToken is the OpenStack-style service token for the greeter service.
const serviceToken = "greeter"

// serviceDescriptor is this client's OWN modern service descriptor (max 2.1),
// from which the auto-pinned microversion is read.
var serviceDescriptor = greeterv1.File_greeter_v1_greeter_proto.Services().ByName("GreeterService")

// failures counts scenarios whose outcome did not match expectations.
var failures int

// ok reports a scenario whose outcome matched expectations.
func ok(what, detail string) { fmt.Printf("  \u2713 %-46s %s\n", what, detail) }

// bad reports a scenario whose outcome did NOT match expectations.
func bad(what, detail string) {
	failures++
	fmt.Printf("  \u2717 %-46s %s\n", what, detail)
}

func main() {
	fmt.Println("grpc client: raw gRPC, auto-pinning via autopingrpc from the 2.1 descriptor.")
	fmt.Println()

	demoAutoPinned()
	demoExplicitOverride()

	fmt.Println()
	if failures == 0 {
		fmt.Println("RESULT: PASS \u2014 the raw-gRPC client negotiated microversions and every scenario matched.")
		return
	}
	fmt.Printf("RESULT: FAIL \u2014 %d scenario(s) did not behave as expected.\n", failures)
	os.Exit(1)
}

// dial connects to the gRPC server with the given autopingrpc options applied to
// both the unary and stream client interceptors.
//
// Arguments:
//   - opts: autopingrpc options (service token, optional version override).
//
// Returns the client connection or exits on dial failure.
func dial(opts ...autopingrpc.Option) *grpc.ClientConn {
	conn, err := grpc.NewClient(
		grpcAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(autopingrpc.UnaryClientInterceptor(serviceDescriptor, opts...)),
		grpc.WithStreamInterceptor(autopingrpc.StreamClientInterceptor(serviceDescriptor, opts...)),
	)
	if err != nil {
		fmt.Printf("dial error: %v\n", err)
		os.Exit(1)
	}
	return conn
}

// demoAutoPinned exercises the modern surface at the auto-pinned 2.1 version over
// raw gRPC: Hello (2.0 fields honored), Farewell (2.0), and HelloStream (2.1).
func demoAutoPinned() {
	fmt.Println("=== auto-pinned 2.1 behavior (raw gRPC) ===")
	conn := dial(autopingrpc.WithServiceToken(serviceToken))
	defer conn.Close()
	client := greeterv1.NewGreeterServiceClient(conn)

	if resp, err := client.Hello(context.Background(), &greeterv1.HelloRequest{
		Name:     "Turing",
		Title:    "Dr.",
		Language: "de",
	}); err != nil {
		bad("Hello (title/language honored)", "unexpected error: "+err.Error())
	} else if resp.GetServedBy() != "" && resp.GetMessage() == "Hallo, Dr. Turing!" {
		ok("Hello honors title/language, sets served_by", fmt.Sprintf("message=%q served_by=%q", resp.GetMessage(), resp.GetServedBy()))
	} else {
		bad("Hello should honor 2.0 fields", fmt.Sprintf("message=%q served_by=%q", resp.GetMessage(), resp.GetServedBy()))
	}

	if resp, err := client.Farewell(context.Background(), &greeterv1.FarewellRequest{Name: "Hopper"}); err != nil {
		bad("Farewell (introduced at 2.0)", "unexpected error: "+err.Error())
	} else {
		ok("Farewell allowed at 2.1", fmt.Sprintf("message=%q", resp.GetMessage()))
	}

	demoHelloStream(client)
}

// demoHelloStream exercises the server-streaming HelloStream RPC (2.1) over raw
// gRPC, verifying the expected number of messages and served_by presence.
func demoHelloStream(client greeterv1.GreeterServiceClient) {
	stream, err := client.HelloStream(context.Background(), &greeterv1.HelloStreamRequest{
		Name:     "Lovelace",
		Language: "fr",
		Repeat:   3,
	})
	if err != nil {
		bad("HelloStream (introduced at 2.1)", "unexpected error: "+err.Error())
		return
	}

	var count int
	var last string
	var servedBySeen bool
	for {
		msg, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			bad("HelloStream receive", "stream error: "+err.Error())
			return
		}
		count++
		last = msg.GetMessage()
		if msg.GetServedBy() != "" {
			servedBySeen = true
		}
	}

	if count == 3 && servedBySeen {
		ok("HelloStream streams 3 greetings at 2.1", fmt.Sprintf("count=%d last=%q", count, last))
	} else {
		bad("HelloStream should stream 3 greetings with served_by", fmt.Sprintf("count=%d served_by_seen=%v", count, servedBySeen))
	}
}

// demoExplicitOverride requests microversion 1.0 explicitly; the server then
// serves legacy behavior (gates 2.0 fields) and rejects the 2.0 Farewell method.
func demoExplicitOverride() {
	fmt.Println("\n=== explicit override to 1.0 (raw gRPC) ===")
	conn := dial(autopingrpc.WithServiceToken(serviceToken), autopingrpc.WithVersion("1.0"))
	defer conn.Close()
	client := greeterv1.NewGreeterServiceClient(conn)

	resp, err := client.Hello(context.Background(), &greeterv1.HelloRequest{
		Name:     "Grace",
		Title:    "Dr.",
		Language: "de",
	})
	switch {
	case err != nil:
		bad("override 1.0 Hello", "unexpected error: "+err.Error())
	case resp.GetServedBy() == "" && resp.GetMessage() == "Hello, Grace!":
		ok("server gates 2.0 fields at 1.0", fmt.Sprintf("message=%q served_by=%q", resp.GetMessage(), resp.GetServedBy()))
	default:
		bad("1.0 should gate 2.0 fields", fmt.Sprintf("message=%q served_by=%q", resp.GetMessage(), resp.GetServedBy()))
	}

	if _, err := client.Farewell(context.Background(), &greeterv1.FarewellRequest{Name: "Hopper"}); status.Code(err) == codes.Unimplemented {
		ok("Farewell rejected at 1.0 (as expected)", fmt.Sprintf("[%s]", status.Code(err)))
	} else {
		bad("Farewell should be rejected at 1.0", fmt.Sprintf("got code=%s err=%v", status.Code(err), err))
	}
}

// Command modern is the T3 client: it was code-generated at T2 from the full
// 2.1 proto snapshot (example/proto/modern). Its generated descriptor carries
// service_versioning{max:"2.1"}, so it auto-pins to microversion 2.1 purely by
// reading its OWN compiled-in annotation — no hardcoded version string. Its
// types include the 2.0 title/language/served_by fields and the Farewell method,
// so it can exercise the modern surface. Recompiling this very program against a
// newer snapshot would raise the auto-pinned version with zero code changes —
// that is the whole point of the T0..T3 story.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"connectrpc.com/connect/v2"

	greeterv1 "go.f0o.dev/version-intercept/example/gen-modern/greeter/v1"
	"go.f0o.dev/version-intercept/example/gen-modern/greeter/v1/greeterv1connect"
	"go.f0o.dev/version-intercept/example/internal/exampleclient"
)

// serviceDescriptor is this client's OWN frozen service descriptor, from which
// the auto-pinned microversion is read.
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
	autoPin, pinned := exampleclient.AutoPinInterceptor(serviceDescriptor)
	client := greeterv1connect.NewGreeterServiceClient(exampleclient.NewConnectClient(autoPin))

	fmt.Println("modern client: T3, compiled from the full 2.1 proto snapshot.")
	fmt.Printf("auto-pinned microversion (read from its own descriptor) = %s\n", pinned)
	fmt.Println("its generated types include title/language/served_by and Farewell.")
	fmt.Println()

	demoAutoPinned(client)
	demoExplicitLegacyOverride()

	fmt.Println()
	if failures == 0 {
		fmt.Println("RESULT: PASS \u2014 the T3 client auto-negotiated 2.1 and every scenario matched.")
		return
	}
	fmt.Printf("RESULT: FAIL \u2014 %d scenario(s) did not behave as expected.\n", failures)
	os.Exit(1)
}

// demoAutoPinned exercises the modern surface at the auto-pinned 2.1 version.
func demoAutoPinned(client greeterv1connect.GreeterServiceClient) {
	fmt.Println("=== auto-pinned 2.1 behavior ===")

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

// demoHelloStream exercises the server-streaming HelloStream RPC (introduced at
// microversion 2.1). It verifies the expected number of messages stream back and
// that each carries the served_by field honored at 2.1.
func demoHelloStream(client greeterv1connect.GreeterServiceClient) {
	stream, err := client.HelloStream(context.Background(), &greeterv1.HelloStreamRequest{
		Name:     "Lovelace",
		Language: "fr",
		Repeat:   3,
	})
	if err != nil {
		bad("HelloStream (introduced at 2.1)", "unexpected error: "+err.Error())
		return
	}
	defer func() { _ = stream.Close() }()

	var count int
	var last string
	var servedBySeen bool
	for {
		msg, recvErr := stream.Receive()
		if errors.Is(recvErr, io.EOF) {
			break
		}
		if recvErr != nil {
			bad("HelloStream receive", "stream error: "+recvErr.Error())
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

// demoExplicitLegacyOverride shows the modern binary can deliberately request an
// older microversion; the server then serves legacy behavior and gates the 2.0
// fields even though the client is capable of sending them.
func demoExplicitLegacyOverride() {
	fmt.Println("\n=== explicit override to 1.0 (same modern binary) ===")

	client := greeterv1connect.NewGreeterServiceClient(exampleclient.NewConnectClient(exampleclient.OverrideInterceptor("1.0")))
	resp, err := client.Hello(context.Background(), &greeterv1.HelloRequest{
		Name:     "Grace",
		Title:    "Dr.",
		Language: "de",
	})
	switch {
	case err != nil:
		bad("override 1.0 Hello", "unexpected error: "+err.Error())
	case resp.GetServedBy() == "" && resp.GetMessage() == "Hello, Grace":
		ok("server gates 2.0 fields at 1.0", fmt.Sprintf("message=%q served_by=%q", resp.GetMessage(), resp.GetServedBy()))
	default:
		bad("1.0 should gate 2.0 fields", fmt.Sprintf("message=%q served_by=%q", resp.GetMessage(), resp.GetServedBy()))
	}

	if _, err := client.Farewell(context.Background(), &greeterv1.FarewellRequest{Name: "Hopper"}); err != nil {
		ok("Farewell rejected at 1.0 (as expected)", connectSummary(err))
	} else {
		bad("Farewell should be rejected at 1.0", "call unexpectedly succeeded")
	}
}

// connectSummary renders a concise description of a Connect error for output.
func connectSummary(err error) string {
	var cerr *connect.Error
	if errors.As(err, &cerr) {
		return fmt.Sprintf("[%s] %s", cerr.Code(), cerr.Message())
	}
	return fmt.Sprintf("[%s] %v", connect.CodeOf(err), err)
}

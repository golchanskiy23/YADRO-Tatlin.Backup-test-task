package main

import (
	"bytes"
	"context"
	"net"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	pb "github.com/golchanskiy23/dns-manager/gen/dns"
)

type fakeServer struct {
	pb.UnimplementedDNSManagerServer
	listResp  *pb.ListDNSServersResponse
	listErr   error
	addErr    error
	removeErr error
}

func (f *fakeServer) ListDNSServers(_ context.Context, _ *pb.ListDNSServersRequest) (*pb.ListDNSServersResponse, error) {
	return f.listResp, f.listErr
}
func (f *fakeServer) AddDNSServer(_ context.Context, _ *pb.AddDNSServerRequest) (*pb.AddDNSServerResponse, error) {
	return &pb.AddDNSServerResponse{}, f.addErr
}
func (f *fakeServer) RemoveDNSServer(_ context.Context, _ *pb.RemoveDNSServerRequest) (*pb.RemoveDNSServerResponse, error) {
	return &pb.RemoveDNSServerResponse{}, f.removeErr
}

func startFakeServer(t *testing.T, srv *fakeServer) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	s := grpc.NewServer()
	pb.RegisterDNSManagerServer(s, srv)
	go s.Serve(lis) //nolint:errcheck
	t.Cleanup(s.Stop)
	return lis.Addr().String()
}

func testDeps(addr string) (*deps, *bytes.Buffer, *bytes.Buffer) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	return &deps{
		stdout: stdout,
		stderr: stderr,
		dial: func(_ string) (*grpc.ClientConn, error) {
			return grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		},
	}, stdout, stderr
}

func runCmd(d *deps, args ...string) error {
	cmd := buildRootCmd(d)
	cmd.SetOut(d.stdout)
	cmd.SetErr(d.stderr)
	cmd.SetArgs(args)
	return cmd.Execute()
}

func TestHelp(t *testing.T) {
	d, stdout, _ := testDeps("")
	_ = runCmd(d, "--help")

	out := stdout.String()
	for _, want := range []string{"dns-client", "list", "add", "remove"} {
		if !strings.Contains(out, want) {
			t.Errorf("help output missing %q; got:\n%s", want, out)
		}
	}
}

func TestListSuccess(t *testing.T) {
	fake := &fakeServer{listResp: &pb.ListDNSServersResponse{Servers: []string{"8.8.8.8", "1.1.1.1"}}}
	addr := startFakeServer(t, fake)
	d, stdout, _ := testDeps(addr)

	if err := runCmd(d, "--server", addr, "list"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "8.8.8.8") || !strings.Contains(out, "1.1.1.1") {
		t.Errorf("unexpected stdout: %s", out)
	}
}

func TestAddSuccess(t *testing.T) {
	addr := startFakeServer(t, &fakeServer{})
	d, stdout, _ := testDeps(addr)

	if err := runCmd(d, "--server", addr, "add", "8.8.8.8"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "8.8.8.8") {
		t.Errorf("expected IP in output, got: %s", stdout)
	}
}

func TestRemoveSuccess(t *testing.T) {
	addr := startFakeServer(t, &fakeServer{})
	d, stdout, _ := testDeps(addr)

	if err := runCmd(d, "--server", addr, "remove", "8.8.8.8"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "8.8.8.8") {
		t.Errorf("expected IP in output, got: %s", stdout)
	}
}

func TestServerReturnsError(t *testing.T) {
	fake := &fakeServer{addErr: status.Error(codes.InvalidArgument, "invalid IP address")}
	addr := startFakeServer(t, fake)
	d, _, _ := testDeps(addr)

	err := runCmd(d, "--server", addr, "add", "not-an-ip")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "invalid IP address") {
		t.Errorf("error should mention server message, got: %v", err)
	}
}

func TestConnectionError(t *testing.T) {
	d := &deps{
		stdout: &bytes.Buffer{},
		stderr: &bytes.Buffer{},
		dial: func(addr string) (*grpc.ClientConn, error) {
			return nil, &net.OpError{
				Op:  "dial",
				Net: "tcp",
				Err: &net.AddrError{Err: "connection refused", Addr: addr},
			}
		},
	}

	err := runCmd(d, "--server", "127.0.0.1:1", "list")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

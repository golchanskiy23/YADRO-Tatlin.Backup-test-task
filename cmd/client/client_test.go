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
	go s.Serve(lis)
	t.Cleanup(s.Stop)
	return lis.Addr().String()
}

func patchDial(t *testing.T, addr string) {
	t.Helper()
	dialFn := func(_ string) (*grpc.ClientConn, error) {
		return grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	orig := dialFn
	t.Cleanup(func() { dialFn = orig })
}

func testDeps() (*deps, *bytes.Buffer, *bytes.Buffer, *int) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	exitCode := 0
	d := &deps{
		stdout: stdout,
		stderr: stderr,
		exit:   func(code int) { exitCode = code },
	}
	return d, stdout, stderr, &exitCode
}

func TestHelp(t *testing.T) {
	d, stdout, _, _ := testDeps()
	cmd := buildRootCmd(d)
	cmd.SetOut(stdout)
	cmd.SetArgs([]string{"--help"})
	_ = cmd.Execute()

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
	patchDial(t, addr)

	d, stdout, stderr, exitCode := testDeps()
	cmd := buildRootCmd(d)
	cmd.SetArgs([]string{"--server", addr, "list"})
	_ = cmd.Execute()

	if *exitCode != 0 {
		t.Errorf("expected exit 0, got %d; stderr: %s", *exitCode, stderr)
	}
	out := stdout.String()
	if !strings.Contains(out, "8.8.8.8") || !strings.Contains(out, "1.1.1.1") {
		t.Errorf("unexpected stdout: %s", out)
	}
}

func TestAddSuccess(t *testing.T) {
	fake := &fakeServer{}
	addr := startFakeServer(t, fake)
	patchDial(t, addr)

	d, stdout, _, exitCode := testDeps()
	cmd := buildRootCmd(d)
	cmd.SetArgs([]string{"--server", addr, "add", "8.8.8.8"})
	_ = cmd.Execute()

	if *exitCode != 0 {
		t.Errorf("expected exit 0, got %d", *exitCode)
	}
	if !strings.Contains(stdout.String(), "8.8.8.8") {
		t.Errorf("expected IP in output, got: %s", stdout)
	}
}

func TestServerError(t *testing.T) {
	fake := &fakeServer{addErr: status.Error(codes.InvalidArgument, "invalid IP address")}
	addr := startFakeServer(t, fake)
	patchDial(t, addr)

	d, _, stderr, exitCode := testDeps()
	cmd := buildRootCmd(d)
	cmd.SetArgs([]string{"--server", addr, "add", "not-an-ip"})
	_ = cmd.Execute()

	if *exitCode != 1 {
		t.Errorf("expected exit 1, got %d", *exitCode)
	}
	if !strings.Contains(stderr.String(), "error") {
		t.Errorf("expected error in stderr, got: %s", stderr)
	}
}

func TestConnectionError(t *testing.T) {
	orig := dialFn
	dialFn = func(addr string) (*grpc.ClientConn, error) {
		return nil, &net.OpError{Op: "dial", Net: "tcp", Err: &net.AddrError{Err: "connection refused", Addr: addr}}
	}
	t.Cleanup(func() { dialFn = orig })

	d, _, _, exitCode := testDeps()
	cmd := buildRootCmd(d)
	cmd.SetArgs([]string{"--server", "127.0.0.1:1", "list"})
	_ = cmd.Execute()

	if *exitCode != 1 {
		t.Errorf("expected exit 1, got %d", *exitCode)
	}
}

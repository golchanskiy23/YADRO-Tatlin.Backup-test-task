package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/golchanskiy23/dns-manager/gen/dns"
	"github.com/golchanskiy23/dns-manager/internal/manager"
)

type fakeManager struct {
	listErr   error
	listIPs   []string
	addErr    error
	removeErr error
}

func (f *fakeManager) List() ([]string, error) {
	return f.listIPs, f.listErr
}

func (f *fakeManager) Add(_ string) error {
	return f.addErr
}

func (f *fakeManager) Remove(_ string) error {
	return f.removeErr
}

func newTestService(m dnsManager) *DNSManagerService {
	return &DNSManagerService{
		manager: m,
		logger:  slog.Default(),
	}
}

func grpcCode(err error) codes.Code {
	if err == nil {
		return codes.OK
	}
	s, ok := status.FromError(err)
	if !ok {
		return codes.Unknown
	}
	return s.Code()
}

func TestListDNSServers_IOError_ReturnsInternal(t *testing.T) {
	svc := newTestService(&fakeManager{listErr: fmt.Errorf("read resolv.conf: permission denied")})
	_, err := svc.ListDNSServers(context.Background(), &pb.ListDNSServersRequest{})
	if got := grpcCode(err); got != codes.Internal {
		t.Errorf("expected INTERNAL, got %v", got)
	}
}

func TestListDNSServers_Success_ReturnsOK(t *testing.T) {
	svc := newTestService(&fakeManager{listIPs: []string{"1.1.1.1", "8.8.8.8"}})
	resp, err := svc.ListDNSServers(context.Background(), &pb.ListDNSServersRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.GetServers()) != 2 {
		t.Errorf("expected 2 servers, got %d", len(resp.GetServers()))
	}
}

func TestAddDNSServer_ErrInvalidIP_ReturnsInvalidArgument(t *testing.T) {
	svc := newTestService(&fakeManager{addErr: manager.ErrInvalidIP})
	_, err := svc.AddDNSServer(context.Background(), &pb.AddDNSServerRequest{Ip: "not-an-ip"})
	if got := grpcCode(err); got != codes.InvalidArgument {
		t.Errorf("expected INVALID_ARGUMENT, got %v", got)
	}
}

func TestAddDNSServer_ErrAlreadyExists_ReturnsAlreadyExists(t *testing.T) {
	svc := newTestService(&fakeManager{addErr: manager.ErrAlreadyExists})
	_, err := svc.AddDNSServer(context.Background(), &pb.AddDNSServerRequest{Ip: "1.1.1.1"})
	if got := grpcCode(err); got != codes.AlreadyExists {
		t.Errorf("expected ALREADY_EXISTS, got %v", got)
	}
}

func TestAddDNSServer_IOError_ReturnsInternal(t *testing.T) {
	svc := newTestService(&fakeManager{addErr: fmt.Errorf("write resolv.conf: permission denied")})
	_, err := svc.AddDNSServer(context.Background(), &pb.AddDNSServerRequest{Ip: "1.1.1.1"})
	if got := grpcCode(err); got != codes.Internal {
		t.Errorf("expected INTERNAL, got %v", got)
	}
}

func TestAddDNSServer_Success_ReturnsOK(t *testing.T) {
	svc := newTestService(&fakeManager{})
	_, err := svc.AddDNSServer(context.Background(), &pb.AddDNSServerRequest{Ip: "1.1.1.1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAddDNSServer_WrappedErrInvalidIP_ReturnsInvalidArgument(t *testing.T) {
	svc := newTestService(&fakeManager{addErr: fmt.Errorf("context: %w", manager.ErrInvalidIP)})
	_, err := svc.AddDNSServer(context.Background(), &pb.AddDNSServerRequest{Ip: "bad"})
	if got := grpcCode(err); got != codes.InvalidArgument {
		t.Errorf("expected INVALID_ARGUMENT for wrapped error, got %v", got)
	}
}

func TestRemoveDNSServer_ErrInvalidIP_ReturnsInvalidArgument(t *testing.T) {
	svc := newTestService(&fakeManager{removeErr: manager.ErrInvalidIP})
	_, err := svc.RemoveDNSServer(context.Background(), &pb.RemoveDNSServerRequest{Ip: "not-an-ip"})
	if got := grpcCode(err); got != codes.InvalidArgument {
		t.Errorf("expected INVALID_ARGUMENT, got %v", got)
	}
}

func TestRemoveDNSServer_ErrNotFound_ReturnsNotFound(t *testing.T) {
	svc := newTestService(&fakeManager{removeErr: manager.ErrNotFound})
	_, err := svc.RemoveDNSServer(context.Background(), &pb.RemoveDNSServerRequest{Ip: "9.9.9.9"})
	if got := grpcCode(err); got != codes.NotFound {
		t.Errorf("expected NOT_FOUND, got %v", got)
	}
}

func TestRemoveDNSServer_IOError_ReturnsInternal(t *testing.T) {
	svc := newTestService(&fakeManager{removeErr: fmt.Errorf("rename temp file: permission denied")})
	_, err := svc.RemoveDNSServer(context.Background(), &pb.RemoveDNSServerRequest{Ip: "1.1.1.1"})
	if got := grpcCode(err); got != codes.Internal {
		t.Errorf("expected INTERNAL, got %v", got)
	}
}

func TestRemoveDNSServer_Success_ReturnsOK(t *testing.T) {
	svc := newTestService(&fakeManager{})
	_, err := svc.RemoveDNSServer(context.Background(), &pb.RemoveDNSServerRequest{Ip: "1.1.1.1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRemoveDNSServer_WrappedErrNotFound_ReturnsNotFound(t *testing.T) {
	svc := newTestService(&fakeManager{removeErr: fmt.Errorf("context: %w", manager.ErrNotFound)})
	_, err := svc.RemoveDNSServer(context.Background(), &pb.RemoveDNSServerRequest{Ip: "1.1.1.1"})
	if got := grpcCode(err); got != codes.NotFound {
		t.Errorf("expected NOT_FOUND for wrapped error, got %v", got)
	}
}

func TestToGRPCError_ErrInvalidIP(t *testing.T) {
	err := toGRPCError(manager.ErrInvalidIP)
	if got := grpcCode(err); got != codes.InvalidArgument {
		t.Errorf("expected INVALID_ARGUMENT, got %v", got)
	}
}

func TestToGRPCError_ErrAlreadyExists(t *testing.T) {
	err := toGRPCError(manager.ErrAlreadyExists)
	if got := grpcCode(err); got != codes.AlreadyExists {
		t.Errorf("expected ALREADY_EXISTS, got %v", got)
	}
}

func TestToGRPCError_ErrNotFound(t *testing.T) {
	err := toGRPCError(manager.ErrNotFound)
	if got := grpcCode(err); got != codes.NotFound {
		t.Errorf("expected NOT_FOUND, got %v", got)
	}
}

func TestToGRPCError_IOError(t *testing.T) {
	err := toGRPCError(errors.New("read resolv.conf: permission denied"))
	if got := grpcCode(err); got != codes.Internal {
		t.Errorf("expected INTERNAL, got %v", got)
	}
}

func TestToGRPCError_WrappedErrAlreadyExists(t *testing.T) {
	wrapped := fmt.Errorf("add: %w", manager.ErrAlreadyExists)
	err := toGRPCError(wrapped)
	if got := grpcCode(err); got != codes.AlreadyExists {
		t.Errorf("expected ALREADY_EXISTS for wrapped error, got %v", got)
	}
}

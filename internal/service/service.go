package service

import (
	"context"
	"errors"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/golchanskiy23/dns-manager/gen/dns"
	"github.com/golchanskiy23/dns-manager/internal/manager"
)

type dnsManager interface {
	List() ([]string, error)
	Add(ip string) error
	Remove(ip string) error
}

type DNSManagerService struct {
	pb.UnimplementedDNSManagerServer
	manager dnsManager
	logger  *slog.Logger
}

func New(m dnsManager, logger *slog.Logger) *DNSManagerService {
	return &DNSManagerService{
		manager: m,
		logger:  logger,
	}
}

func toGRPCError(err error) error {
	switch {
	case errors.Is(err, manager.ErrInvalidIP):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, manager.ErrAlreadyExists):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, manager.ErrNotFound):
		return status.Error(codes.NotFound, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}

func (s *DNSManagerService) ListDNSServers(_ context.Context, _ *pb.ListDNSServersRequest) (*pb.ListDNSServersResponse, error) {
	s.logger.Info("ListDNSServers called")

	ips, err := s.manager.List()
	if err != nil {
		s.logger.Error("ListDNSServers failed", slog.String("error", err.Error()))
		return nil, toGRPCError(err)
	}

	s.logger.Info("ListDNSServers succeeded", slog.Int("count", len(ips)))
	return &pb.ListDNSServersResponse{Servers: ips}, nil
}

func (s *DNSManagerService) AddDNSServer(_ context.Context, req *pb.AddDNSServerRequest) (*pb.AddDNSServerResponse, error) {
	ip := req.GetIp()
	s.logger.Info("AddDNSServer called", slog.String("ip", ip))

	if err := s.manager.Add(ip); err != nil {
		s.logger.Error("AddDNSServer failed", slog.String("ip", ip), slog.String("error", err.Error()))
		return nil, toGRPCError(err)
	}

	s.logger.Info("AddDNSServer succeeded", slog.String("ip", ip))
	return &pb.AddDNSServerResponse{}, nil
}

func (s *DNSManagerService) RemoveDNSServer(_ context.Context, req *pb.RemoveDNSServerRequest) (*pb.RemoveDNSServerResponse, error) {
	ip := req.GetIp()
	s.logger.Info("RemoveDNSServer called", slog.String("ip", ip))

	if err := s.manager.Remove(ip); err != nil {
		s.logger.Error("RemoveDNSServer failed", slog.String("ip", ip), slog.String("error", err.Error()))
		return nil, toGRPCError(err)
	}

	s.logger.Info("RemoveDNSServer succeeded", slog.String("ip", ip))
	return &pb.RemoveDNSServerResponse{}, nil
}

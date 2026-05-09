package server

import (
	"context"
	"errors"
	"log/slog"

	pb "github.com/golchanskiy23/dns-manager/gen/dns"
	"github.com/golchanskiy23/dns-manager/internal/manager"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type DNSManagerService struct {
	pb.UnimplementedDNSManagerServer
	manager *manager.Manager
	logger  *slog.Logger
}

func New(m *manager.Manager, logger *slog.Logger) *DNSManagerService {
	return &DNSManagerService {
		manager: m, 
		logger: logger,
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

func (s *DNSManagerService) ListDNSServers(ctx context.Context, req *pb.ListDNSServersRequest) (*pb.ListDNSServersResponse, error) {
	s.logger.Info("ListDNSServers called")

	ips, err := s.manager.List()
	if err != nil {
		s.logger.Error("ListDNSServers failed", slog.String("error", err.Error()))
		return nil, toGRPCError(err)
	}

	s.logger.Info("ListDNSServers succeeded", slog.Int("count", len(ips)))
	return &pb.ListDNSServersResponse{Servers: ips}, nil
}

func (s *DNSManagerService) AddDNSServer(ctx context.Context, req *pb.AddDNSServerRequest) (*pb.AddDNSServerResponse, error) {
	s.logger.Info("AddDNSServer called", slog.String("ip", req.GetIp()))

	if err := s.manager.Add(req.GetIp()); err != nil {
		s.logger.Error("AddDNSServer failed", slog.String("ip", req.GetIp()), slog.String("error", err.Error()))
		return nil, toGRPCError(err)
	}

	s.logger.Info("AddDNSServer succeeded", slog.String("ip", req.GetIp()))
	return &pb.AddDNSServerResponse{}, nil
}

func (s *DNSManagerService) RemoveDNSServer(ctx context.Context, req *pb.RemoveDNSServerRequest) (*pb.RemoveDNSServerResponse, error) {
	s.logger.Info("RemoveDNSServer called", slog.String("ip", req.GetIp()))

	if err := s.manager.Remove(req.GetIp()); err != nil {
		s.logger.Error("RemoveDNSServer failed", slog.String("ip", req.GetIp()), slog.String("error", err.Error()))
		return nil, toGRPCError(err)
	}

	s.logger.Info("RemoveDNSServer succeeded", slog.String("ip", req.GetIp()))
	return &pb.RemoveDNSServerResponse{}, nil
}
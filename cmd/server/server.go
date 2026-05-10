package main

import (
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"google.golang.org/grpc"

	dnsv1 "github.com/golchanskiy23/dns-manager/gen/dns"
	"github.com/golchanskiy23/dns-manager/internal/manager"
	"github.com/golchanskiy23/dns-manager/internal/service"
)

func parseLogLevel(level string) slog.Level {
	switch strings.ToUpper(level) {
	case "DEBUG":
		return slog.LevelDebug
	case "WARN", "WARNING":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	}
	return slog.LevelInfo
}

func run(args []string, environ func(string) string, listenFn func(network, addr string) (net.Listener, error), stderr *os.File) int {
	envPort := environ("DNS_MANAGER_PORT")
	if envPort == "" {
		envPort = "50051"
	}
	envLogLevel := environ("DNS_MANAGER_LOG_LEVEL")
	if envLogLevel == "" {
		envLogLevel = "INFO"
	}

	fs := flag.NewFlagSet("server", flag.ContinueOnError)
	port := fs.String("port", envPort, "TCP port to listen on")
	logLevel := fs.String("log-level", envLogLevel, "Log level (DEBUG, INFO, WARN, ERROR)")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(stderr, "flag parse error: %v\n", err)
		return 1
	}

	level := parseLogLevel(*logLevel)
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))

	mgr := manager.New("", logger)
	svc := service.New(mgr, logger)

	grpcServer := grpc.NewServer()
	dnsv1.RegisterDNSManagerServer(grpcServer, svc)

	addr := ":" + *port
	lis, err := listenFn("tcp", addr)
	if err != nil {
		fmt.Fprintf(stderr, "failed to listen on %s: %v\n", addr, err)
		return 1
	}

	logger.Info("starting gRPC server", slog.String("addr", addr))

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-quit
		logger.Info("received signal, shutting down", slog.String("signal", sig.String()))
		grpcServer.GracefulStop()
	}()

	if err := grpcServer.Serve(lis); err != nil {
		fmt.Fprintf(stderr, "gRPC server error: %v\n", err)
		return 1
	}
	return 0
}

func main() {
	code := run(os.Args[1:], os.Getenv, net.Listen, os.Stderr)
	if code != 0 {
		os.Exit(code)
	}
}

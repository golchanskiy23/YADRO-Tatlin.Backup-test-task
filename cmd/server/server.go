package main

import (
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"google.golang.org/grpc"

	dnsv1 "github.com/golchanskiy23/dns-manager/gen/dns"
	"github.com/golchanskiy23/dns-manager/internal/manager"
	"github.com/golchanskiy23/dns-manager/internal/service"
)

const defaultResolvConf = "/etc/resolv.conf"

type env func(string) string
type listener func(network, addr string) (net.Listener, error)

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

func run(args []string, environ env, listenFn listener, stderr *os.File) error {
	envPort := environ("DNS_MANAGER_PORT")
	if envPort == "" {
		envPort = "50051"
	}
	envLogLevel := environ("DNS_MANAGER_LOG_LEVEL")
	if envLogLevel == "" {
		envLogLevel = "INFO"
	}

	fs := flag.NewFlagSet("server", flag.ContinueOnError)
	fs.SetOutput(stderr)
	port := fs.String("port", envPort, "TCP port to listen on")
	logLevel := fs.String("log-level", envLogLevel, "Log level (DEBUG, INFO, WARN, ERROR)")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("error detected: %v", err)
	}

	level := parseLogLevel(*logLevel)
	logger := slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: level}))

	mgr := manager.New(defaultResolvConf, logger)
	svc := service.New(mgr, logger)

	grpcServer := grpc.NewServer()
	dnsv1.RegisterDNSManagerServer(grpcServer, svc)

	addr := ":" + *port
	lis, err := listenFn("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	logger.Info("starting gRPC server", slog.String("addr", addr))

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(quit)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		sig := <-quit
		logger.Info("received signal, shutting down", slog.String("signal", sig.String()))
		grpcServer.GracefulStop()
	}()

	if err := grpcServer.Serve(lis); err != nil {
		return fmt.Errorf("gRPC server error: %w", err)
	}

	wg.Wait()
	return nil
}

func main() {
	if err := run(os.Args[1:], os.Getenv, net.Listen, os.Stderr); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

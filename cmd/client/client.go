package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/golchanskiy23/dns-manager/gen/dns"
)

const (
	defaultServerAddr = "localhost:50051"
	rpcTimeout        = 10 * time.Second
)

type dialFunc func(addr string) (*grpc.ClientConn, error)

func defaultDial(addr string) (*grpc.ClientConn, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to server %s: %w", addr, err)
	}
	return conn, nil
}

type deps struct {
	stdout io.Writer
	stderr io.Writer
	dial   dialFunc
}

func newDeps() *deps {
	return &deps{
		stdout: os.Stdout,
		stderr: os.Stderr,
		dial:   defaultDial,
	}
}

func buildRootCmd(d *deps) *cobra.Command {
	var serverAddr string

	root := &cobra.Command{
		Use:   "dns-client",
		Short: "CLI client for dns-manager gRPC service",
	}
	root.PersistentFlags().StringVar(&serverAddr, "server", defaultServerAddr, "server address (host:port)")

	root.AddCommand(
		buildListCmd(d, &serverAddr),
		buildAddCmd(d, &serverAddr),
		buildRemoveCmd(d, &serverAddr),
	)
	return root
}

func connect(d *deps, serverAddr string) (pb.DNSManagerClient, *grpc.ClientConn, error) {
	conn, err := d.dial(serverAddr)
	if err != nil {
		return nil, nil, err
	}
	return pb.NewDNSManagerClient(conn), conn, nil
}

func buildListCmd(d *deps, serverAddr *string) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List DNS servers",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn, err := connect(d, *serverAddr)
			if err != nil {
				return err
			}
			defer conn.Close()

			ctx, cancel := context.WithTimeout(context.Background(), rpcTimeout)
			defer cancel()

			resp, err := client.ListDNSServers(ctx, &pb.ListDNSServersRequest{})
			if err != nil {
				return fmt.Errorf("list: %w", err)
			}
			for _, s := range resp.GetServers() {
				fmt.Fprintln(d.stdout, s)
			}
			return nil
		},
	}
}

func buildAddCmd(d *deps, serverAddr *string) *cobra.Command {
	return &cobra.Command{
		Use:   "add <IP>",
		Short: "Add a DNS server",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn, err := connect(d, *serverAddr)
			if err != nil {
				return err
			}
			defer conn.Close()

			ctx, cancel := context.WithTimeout(context.Background(), rpcTimeout)
			defer cancel()

			_, err = client.AddDNSServer(ctx, &pb.AddDNSServerRequest{Ip: args[0]})
			if err != nil {
				return fmt.Errorf("add %s: %w", args[0], err)
			}
			fmt.Fprintf(d.stdout, "DNS server %s added successfully\n", args[0])
			return nil
		},
	}
}

func buildRemoveCmd(d *deps, serverAddr *string) *cobra.Command {
	return &cobra.Command{
		Use:   "remove <IP>",
		Short: "Remove a DNS server",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn, err := connect(d, *serverAddr)
			if err != nil {
				return err
			}
			defer conn.Close()

			ctx, cancel := context.WithTimeout(context.Background(), rpcTimeout)
			defer cancel()

			_, err = client.RemoveDNSServer(ctx, &pb.RemoveDNSServerRequest{Ip: args[0]})
			if err != nil {
				return fmt.Errorf("remove %s: %w", args[0], err)
			}
			fmt.Fprintf(d.stdout, "DNS server %s removed successfully\n", args[0])
			return nil
		},
	}
}

func main() {
	cmd := buildRootCmd(newDeps())
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}

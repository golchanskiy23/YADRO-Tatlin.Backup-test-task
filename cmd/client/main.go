package main

import (
	"context"
	"fmt"
	"io"
	"os"

	pb "github.com/golchanskiy23/dns-manager/gen/dns"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var serverAddr string

var dialFn = func(addr string) (*grpc.ClientConn, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to server %s: %w", addr, err)
	}
	return conn, nil
}

type deps struct {
	stdout io.Writer
	stderr io.Writer
	exit   func(int)
}

func newClient(d *deps) (pb.DNSManagerClient, *grpc.ClientConn) {
	conn, err := dialFn(serverAddr)
	if err != nil {
		fmt.Fprintln(d.stderr, err)
		d.exit(1)
		return nil, nil
	}
	return pb.NewDNSManagerClient(conn), conn
}

func buildRootCmd(d *deps) *cobra.Command {
	root := &cobra.Command{
		Use:   "dns-client",
		Short: "CLI client for dns-manager gRPC service",
	}
	root.PersistentFlags().StringVar(&serverAddr, "server", "localhost:50051", "server address (host:port)")

	root.AddCommand(
		buildListCmd(d),
		buildAddCmd(d),
		buildRemoveCmd(d),
	)
	return root
}

func buildListCmd(d *deps) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List DNS servers",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn := newClient(d)
			if conn == nil {
				return nil
			}
			defer conn.Close()

			resp, err := client.ListDNSServers(context.Background(), &pb.ListDNSServersRequest{})
			if err != nil {
				fmt.Fprintf(d.stderr, "error: %v\n", err)
				d.exit(1)
				return nil
			}
			for _, s := range resp.GetServers() {
				fmt.Fprintln(d.stdout, s)
			}
			return nil
		},
	}
}

func buildAddCmd(d *deps) *cobra.Command {
	return &cobra.Command{
		Use:   "add <IP>",
		Short: "Add a DNS server",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn := newClient(d)
			if conn == nil {
				return nil
			}
			defer conn.Close()

			_, err := client.AddDNSServer(context.Background(), &pb.AddDNSServerRequest{Ip: args[0]})
			if err != nil {
				fmt.Fprintf(d.stderr, "error: %v\n", err)
				d.exit(1)
				return nil
			}
			fmt.Fprintf(d.stdout, "DNS server %s added successfully\n", args[0])
			return nil
		},
	}
}

func buildRemoveCmd(d *deps) *cobra.Command {
	return &cobra.Command{
		Use:   "remove <IP>",
		Short: "Remove a DNS server",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn := newClient(d)
			if conn == nil {
				return nil
			}
			defer conn.Close()

			_, err := client.RemoveDNSServer(context.Background(), &pb.RemoveDNSServerRequest{Ip: args[0]})
			if err != nil {
				fmt.Fprintf(d.stderr, "error: %v\n", err)
				d.exit(1)
				return nil
			}
			fmt.Fprintf(d.stdout, "DNS server %s removed successfully\n", args[0])
			return nil
		},
	}
}

func main() {
	d := &deps{stdout: os.Stdout, stderr: os.Stderr, exit: os.Exit}
	cmd := buildRootCmd(d)
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}

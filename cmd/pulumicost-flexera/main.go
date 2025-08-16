package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/rshade/pulumi-plugin-flexera/internal/flexera"
	"github.com/rshade/pulumi-plugin-flexera/internal/server"
	"github.com/rshade/pulumi-plugin-flexera/pkg/version"
	// TODO: Add when pulumicost-spec is available
	// pbc "github.com/yourorg/pulumicost-spec/sdk/go/proto"
)

func main() {
	// Parse command line flags
	showVersion := flag.Bool("version", false, "Show version information")
	showVersionFull := flag.Bool("version-full", false, "Show detailed version information")
	flag.Parse()

	// Handle version flags
	if *showVersion {
		fmt.Println(version.String())
		os.Exit(0)
	}
	if *showVersionFull {
		fmt.Println(version.FullString())
		os.Exit(0)
	}

	// Load configuration from environment or config file
	cfg, err := flexera.LoadConfigFromEnvOrFile(os.Getenv("FLEXERA_CONFIG"))
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	// Create Flexera client
	cli, err := flexera.NewClient(contextWithTimeout(context.Background()), cfg)
	if err != nil {
		log.Fatalf("client: %v", err)
	}

	log.Printf("pulumicost-flexera starting, %s", version.String())
	log.Printf("flexera region: %s, org: %s", cfg.Region, cfg.OrgID)

	// Pulumi-style plugins often use stdin/stdout. For simplicity here, use a TCP loopback.
	// Your plugin host can launch and connect to this ephemeral port; or adapt to stdio transport.
	lis, err := net.Listen("tcp", "127.0.0.1:50051")
	if err != nil {
		log.Fatalf("listen: %v", err)
	}

	grpcServer := grpc.NewServer(grpc.Creds(insecure.NewCredentials()))
	_ = server.NewFlexeraServer(cli)
	// TODO: Uncomment when pulumicost-spec protobuf definitions are available
	// flexeraServer := server.NewFlexeraServer(cli)
	// flexeraServer.RegisterService(grpcServer)

	log.Printf("listening on %s", lis.Addr().String())
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("serve: %v", err)
	}
}

func contextWithTimeout(ctx context.Context) context.Context {
	timeout := 30 * time.Second
	if d := os.Getenv("FLEXERA_TIMEOUT"); d != "" {
		if parsed, err := time.ParseDuration(d); err == nil {
			timeout = parsed
		}
	}
	c, _ := context.WithTimeout(ctx, timeout)
	return c
}
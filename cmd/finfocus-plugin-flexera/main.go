package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"

	flexeraclient "github.com/flexera-public/unified-go-client"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"

	"github.com/rshade/finfocus-plugin-flexera/internal/flexera"
	"github.com/rshade/finfocus-plugin-flexera/internal/flexeraapi"
	"github.com/rshade/finfocus-plugin-flexera/internal/server"
	"github.com/rshade/finfocus-plugin-flexera/pkg/version"
)

func main() {
	// Parse command line flags
	showVersion := flag.Bool("version", false, "Show version information")
	showVersionFull := flag.Bool("version-full", false, "Show detailed version information")
	flag.Parse()

	// Handle version flags
	if *showVersion {
		fmt.Fprintln(os.Stdout, version.String())
		os.Exit(0)
	}
	if *showVersionFull {
		fmt.Fprintln(os.Stdout, version.FullString())
		os.Exit(0)
	}

	// Load configuration from environment or config file
	cfg, err := flexera.LoadConfigFromEnvOrFile(os.Getenv("FLEXERA_CONFIG"))
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	// Create Flexera client
	cli, err := flexera.NewClient(context.Background(), cfg)
	if err != nil {
		log.Fatalf("client: %v", err)
	}

	log.Printf("finfocus-plugin-flexera starting, %s", version.String())
	log.Printf("flexera region: %s, org: %s", cfg.Region, cfg.OrgID)

	// FinFocus plugins use gRPC. For simplicity here, use a TCP loopback.
	// The plugin host will launch and connect to this ephemeral port.
	lis, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:50051")
	if err != nil {
		log.Fatalf("listen: %v", err)
	}

	grpcServer := grpc.NewServer(grpc.Creds(insecure.NewCredentials()))
	flexeraServer := server.NewFlexeraServer(cli)
	if apiErr := attachCostAPI(flexeraServer, cfg); apiErr != nil {
		log.Fatalf("flexera client: %v", apiErr)
	}
	flexeraServer.RegisterService(grpcServer)
	reflection.Register(grpcServer)

	log.Printf("listening on %s", lis.Addr().String())
	if serveErr := grpcServer.Serve(lis); serveErr != nil {
		log.Fatalf("serve: %v", serveErr)
	}
}

func attachCostAPI(flexeraServer *server.FlexeraServer, cfg flexera.Config) error {
	if !cfg.HasOAuth() {
		log.Printf("cost RPCs need FLEXERA_REFRESH_TOKEN or FLEXERA_CLIENT_ID and FLEXERA_CLIENT_SECRET")
		return nil
	}
	orgID, err := strconv.ParseInt(cfg.OrgID, 10, 64)
	if err != nil {
		return fmt.Errorf("FLEXERA_ORG_ID must be numeric for the Flexera client: %w", err)
	}
	zone, err := flexeraapi.ZoneForRegion(cfg.Region)
	if err != nil {
		return err
	}
	auth := flexeraclient.OAuthConfig{RefreshToken: cfg.RefreshToken}
	if cfg.RefreshToken == "" {
		auth.ClientID = cfg.ClientID
		auth.ClientSecret = cfg.ClientSecret
	}
	api, err := flexeraapi.New(context.Background(), zone, orgID, auth, nil)
	if err != nil {
		return err
	}
	flexeraServer.UseCostAPI(api, cfg.BillingCenterIDs, cfg.CostMetric)
	return nil
}

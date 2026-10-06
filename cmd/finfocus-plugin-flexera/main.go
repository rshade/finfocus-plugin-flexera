package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"strconv"
	"strings"

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
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "finfocus-plugin-flexera: %s\n", flexeraapi.Redact(err.Error()))
		os.Exit(1)
	}
}

type runOptions struct {
	showVersion     bool
	showVersionFull bool
}

func run(args []string, stdout, stderr io.Writer) error {
	opts, err := parseArgs(args, stderr)
	if err != nil {
		return err
	}
	if opts.showVersion {
		fmt.Fprintln(stdout, version.String())
		return nil
	}
	if opts.showVersionFull {
		fmt.Fprintln(stdout, version.FullString())
		return nil
	}
	return runServer(stderr)
}

func parseArgs(args []string, stderr io.Writer) (runOptions, error) {
	fs := flag.NewFlagSet("finfocus-plugin-flexera", flag.ContinueOnError)
	fs.SetOutput(stderr)
	showVersion := fs.Bool("version", false, "Show version information")
	showVersionFull := fs.Bool("version-full", false, "Show detailed version information")
	if err := fs.Parse(args); err != nil {
		return runOptions{}, err
	}
	return runOptions{showVersion: *showVersion, showVersionFull: *showVersionFull}, nil
}

func runServer(stderr io.Writer) error {
	cfg, err := flexera.LoadConfigFromEnvOrFile(os.Getenv("FLEXERA_CONFIG"))
	if err != nil {
		return err
	}
	logger := newLogger(stderr)
	logger.Info("finfocus-plugin-flexera starting", "version", version.String(), "region", cfg.Region, "org", cfg.OrgID)
	if cfg.TLSSkipVerify {
		logger.Warn("tlsSkipVerify is set; TLS certificate verification is disabled")
	}
	cli, err := flexera.NewClient(context.Background(), cfg)
	if err != nil {
		return flexeraapi.RedactError(err)
	}
	flexeraServer := server.NewFlexeraServer(cli)
	flexeraServer.SetRPCTimeout(cfg.Timeout)
	if apiErr := attachCostAPI(flexeraServer, cfg, logger); apiErr != nil {
		return flexeraapi.RedactError(apiErr)
	}
	return serve(flexeraServer, logger)
}

func newLogger(w io.Writer) *slog.Logger {
	handler := slog.NewTextHandler(w, &slog.HandlerOptions{Level: logLevel()})
	return slog.New(redactingHandler{next: handler})
}

func logLevel() slog.Level {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("FLEXERA_LOG_LEVEL"))) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

type redactingHandler struct {
	next slog.Handler
}

func (h redactingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h redactingHandler) Handle(ctx context.Context, record slog.Record) error {
	record.Message = flexeraapi.Redact(record.Message)
	attrs := make([]slog.Attr, 0, record.NumAttrs())
	record.Attrs(func(attr slog.Attr) bool {
		attrs = append(attrs, redactAttr(attr))
		return true
	})
	cleaned := slog.NewRecord(record.Time, record.Level, record.Message, record.PC)
	cleaned.AddAttrs(attrs...)
	return h.next.Handle(ctx, cleaned)
}

func (h redactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	redacted := make([]slog.Attr, len(attrs))
	for i, attr := range attrs {
		redacted[i] = redactAttr(attr)
	}
	return redactingHandler{next: h.next.WithAttrs(redacted)}
}

func (h redactingHandler) WithGroup(name string) slog.Handler {
	return redactingHandler{next: h.next.WithGroup(name)}
}

func redactAttr(attr slog.Attr) slog.Attr {
	if attr.Value.Kind() == slog.KindString {
		attr.Value = slog.StringValue(flexeraapi.Redact(attr.Value.String()))
	}
	return attr
}

func serve(flexeraServer *server.FlexeraServer, logger *slog.Logger) error {
	lis, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:50051")
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	grpcServer := grpc.NewServer(grpc.Creds(insecure.NewCredentials()))
	flexeraServer.RegisterService(grpcServer)
	reflection.Register(grpcServer)
	logger.Info("listening", "addr", lis.Addr().String())
	if serveErr := grpcServer.Serve(lis); serveErr != nil {
		return fmt.Errorf("serve: %w", serveErr)
	}
	return nil
}

func attachCostAPI(flexeraServer *server.FlexeraServer, cfg flexera.Config, logger *slog.Logger) error {
	if !cfg.HasOAuth() {
		logger.Warn("cost RPCs need FLEXERA_REFRESH_TOKEN or FLEXERA_CLIENT_ID and FLEXERA_CLIENT_SECRET")
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

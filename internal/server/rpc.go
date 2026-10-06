package server

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/rshade/finfocus-plugin-flexera/internal/flexeraapi"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const defaultRPCTimeout = 30 * time.Second

func (s *FlexeraServer) withRPCDeadline(ctx context.Context) (context.Context, context.CancelFunc) {
	timeout := defaultRPCTimeout
	if s != nil && s.rpcTimeout > 0 {
		timeout = s.rpcTimeout
	}
	return context.WithTimeout(ctx, timeout)
}

// SetRPCTimeout sets the deadline each cost RPC applies to its own call.
func (s *FlexeraServer) SetRPCTimeout(timeout time.Duration) {
	if s == nil || timeout <= 0 {
		return
	}
	s.rpcTimeout = timeout
}

func (s *FlexeraServer) beginRPC(ctx context.Context) (context.Context, context.CancelFunc, error) {
	ctx, cancel := s.withRPCDeadline(ctx)
	if err := ctx.Err(); err != nil {
		cancel()
		return nil, nil, mapFailure(err)
	}
	return ctx, cancel, nil
}

// mapFailure maps one dependency failure class to one gRPC status code.
func mapFailure(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := status.FromError(err); ok {
		return err
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return status.Error(codes.DeadlineExceeded, flexeraapi.Redact(err.Error()))
	}
	if errors.Is(err, context.Canceled) {
		return status.Error(codes.Canceled, flexeraapi.Redact(err.Error()))
	}
	var httpErr *flexeraapi.StatusError
	if errors.As(err, &httpErr) {
		return status.Error(codeForHTTP(httpErr.Status), flexeraapi.Redact(httpErr.Error()))
	}
	return status.Error(codes.Internal, flexeraapi.Redact(err.Error()))
}

func codeForHTTP(statusCode int) codes.Code {
	switch statusCode {
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return codes.InvalidArgument
	case http.StatusUnauthorized:
		return codes.Unauthenticated
	case http.StatusForbidden:
		return codes.PermissionDenied
	case http.StatusNotFound:
		return codes.NotFound
	case http.StatusConflict:
		return codes.Aborted
	case http.StatusTooManyRequests:
		return codes.ResourceExhausted
	default:
		if statusCode >= http.StatusInternalServerError {
			return codes.Unavailable
		}
		return codes.Internal
	}
}

func (s *FlexeraServer) costSelectDimensions() []string {
	dims := []string{dimResourceID, dimVendor, dimService, dimRegion}
	if s == nil || s.cli == nil {
		return dims
	}
	return append(dims, s.cli.CostTagDimensions()...)
}

func (s *FlexeraServer) billingCenterID(dimensions map[string]string) string {
	if s == nil || s.cli == nil {
		return ""
	}
	id, err := s.cli.GetBillingCenterForTags(tagsFromDimensions(dimensions))
	if err != nil || id == "" {
		return ""
	}
	return id
}

func tagsFromDimensions(dimensions map[string]string) map[string]string {
	if len(dimensions) == 0 {
		return nil
	}
	tags := make(map[string]string)
	for key, value := range dimensions {
		switch {
		case strings.HasPrefix(key, "tag:"):
			tags[strings.TrimPrefix(key, "tag:")] = value
		case strings.HasPrefix(key, "tag_"):
			tags[strings.TrimPrefix(key, "tag_")] = value
		}
	}
	if len(tags) == 0 {
		return nil
	}
	return tags
}

func billingLineage(resourceID, billingCenterID string) *pbc.LineageNode {
	return &pbc.LineageNode{
		Type: pbc.LineageNodeType_LINEAGE_NODE_TYPE_RESOURCE,
		Id:   resourceID,
		Parent: &pbc.LineageNode{
			Type: pbc.LineageNodeType_LINEAGE_NODE_TYPE_BILLING_ACCOUNT,
			Id:   billingCenterID,
		},
	}
}

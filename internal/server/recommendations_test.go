package server

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	unified "github.com/flexera-public/unified-go-client"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestGetRecommendationsMapsOptimaRows(t *testing.T) {
	active := unified.OptimaRecommendationsRecommendationResultResponse{
		Id:         "rec-active",
		Savings:    12.5,
		ResourceID: "i-abc",
		Status:     string(unified.Active),
		Vendor:     "AWS",
	}
	bare := unified.OptimaRecommendationsRecommendationResultResponse{
		Id:      "rec-bare",
		Savings: 1,
		Status:  string(unified.Active),
	}
	snoozed := unified.OptimaRecommendationsRecommendationResultResponse{
		Id:         "rec-snoozed",
		Savings:    3,
		ResourceID: "i-snooze",
		Status:     string(unified.Snoozed),
	}
	rejected := unified.OptimaRecommendationsRecommendationResultResponse{
		Id:      "rec-rejected",
		Savings: 4,
		Status:  string(unified.Rejected),
	}
	excluded := unified.OptimaRecommendationsRecommendationResultResponse{
		Id:         "rec-excluded",
		Savings:    9,
		ResourceID: "vol-1",
		Status:     string(unified.Active),
	}
	api := &fakeAPI{recommendations: []unified.OptimaRecommendationsRecommendationResultResponse{
		active, bare, snoozed, rejected, excluded,
	}}
	srv := newTestServer(t, "nam")
	srv.UseCostAPI(api, []string{"bc-1"}, "")

	hidden := &pbc.GetRecommendationsRequest{ExcludedRecommendationIds: []string{excluded.Id}}
	resp, err := srv.GetRecommendations(context.Background(), hidden)
	if err != nil {
		t.Fatalf("GetRecommendations: %v", err)
	}
	got := recommendationByID(t, resp, active.Id)
	if got.GetImpact().GetEstimatedSavings() != active.Savings {
		t.Fatalf("savings = %v, source savings = %v", got.GetImpact().GetEstimatedSavings(), active.Savings)
	}
	if got.GetResource().GetId() != active.ResourceID {
		t.Fatalf("resource id = %q, source resource id = %q", got.GetResource().GetId(), active.ResourceID)
	}
	bareGot := recommendationByID(t, resp, bare.Id)
	if bareGot.GetResource() != nil {
		t.Fatalf("resource = %+v", bareGot.GetResource())
	}
	assertAbsent(t, resp, snoozed.Id, rejected.Id, excluded.Id)
	if len(resp.GetRecommendations()) != 2 {
		t.Fatalf("recommendations = %d", len(resp.GetRecommendations()))
	}

	included, err := srv.GetRecommendations(context.Background(), &pbc.GetRecommendationsRequest{
		ExcludedRecommendationIds: []string{excluded.Id},
		IncludeDismissed:          true,
	})
	if err != nil {
		t.Fatalf("include dismissed: %v", err)
	}
	recommendationByID(t, included, snoozed.Id)
	recommendationByID(t, included, rejected.Id)
	assertAbsent(t, included, excluded.Id)

	dismissed, err := srv.DismissRecommendation(context.Background(), &pbc.DismissRecommendationRequest{
		RecommendationId: active.Id,
	})
	if err != nil {
		t.Fatalf("DismissRecommendation: %v", err)
	}
	if !dismissed.GetSuccess() || dismissed.GetRecommendationId() != active.Id {
		t.Fatalf("dismiss response = %+v", dismissed)
	}
	if len(api.statusUpdates) != 1 || api.statusUpdates[0].Id != active.Id {
		t.Fatalf("status updates = %+v", api.statusUpdates)
	}
	rejectedStatus := unified.OptimaRecommendationsRecommendationsUpdateStatusRequestBodyStatusRejected
	if api.statusUpdates[0].Status != rejectedStatus {
		t.Fatalf("status = %q", api.statusUpdates[0].Status)
	}

	after, err := srv.GetRecommendations(context.Background(), hidden)
	if err != nil {
		t.Fatalf("list after dismiss: %v", err)
	}
	assertAbsent(t, after, active.Id)
	recommendationByID(t, after, bare.Id)

	afterInclude, err := srv.GetRecommendations(context.Background(), &pbc.GetRecommendationsRequest{
		IncludeDismissed: true,
	})
	if err != nil {
		t.Fatalf("include after dismiss: %v", err)
	}
	recommendationByID(t, afterInclude, active.Id)

	expires := timestamppb.New(time.Now().Add(24 * time.Hour))
	if _, err := srv.DismissRecommendation(context.Background(), &pbc.DismissRecommendationRequest{
		RecommendationId: bare.Id,
		ExpiresAt:        expires,
	}); err != nil {
		t.Fatalf("snooze: %v", err)
	}
	snoozedStatus := unified.OptimaRecommendationsRecommendationsUpdateStatusRequestBodyStatusSnoozed
	if api.statusUpdates[1].Status != snoozedStatus || api.statusUpdates[1].SnoozedTargetDate == nil {
		t.Fatalf("snooze update = %+v", api.statusUpdates[1])
	}
	if *api.statusUpdates[1].SnoozedTargetDate != expires.AsTime().UTC().Format(time.DateOnly) {
		t.Fatalf("snooze date = %s", *api.statusUpdates[1].SnoozedTargetDate)
	}
	snoozedList, err := srv.GetRecommendations(context.Background(), &pbc.GetRecommendationsRequest{})
	if err != nil {
		t.Fatalf("list after snooze: %v", err)
	}
	assertAbsent(t, snoozedList, bare.Id)
}

func TestGetRecommendationsPagesPastDefaultLimit(t *testing.T) {
	count := defaultPageSize + 1
	rows := make([]unified.OptimaRecommendationsRecommendationResultResponse, count)
	for i := range rows {
		rows[i] = unified.OptimaRecommendationsRecommendationResultResponse{
			Id:         fmt.Sprintf("rec-%02d", i),
			Savings:    float64(i) + 0.5,
			ResourceID: fmt.Sprintf("i-%02d", i),
			Status:     string(unified.Active),
		}
	}
	last := rows[len(rows)-1]
	api := &fakeAPI{recommendations: rows}
	srv := newTestServer(t, "nam")
	srv.UseCostAPI(api, []string{"bc-1"}, "")

	first, err := srv.GetRecommendations(context.Background(), &pbc.GetRecommendationsRequest{})
	if err != nil {
		t.Fatalf("GetRecommendations: %v", err)
	}
	if len(first.GetRecommendations()) != defaultPageSize {
		t.Fatalf("page length = %d", len(first.GetRecommendations()))
	}
	if first.GetNextPageToken() == "" {
		t.Fatal("next_page_token is empty")
	}
	for _, rec := range first.GetRecommendations() {
		if rec.GetId() == last.Id {
			t.Fatal("last source row was returned on the first page")
		}
	}

	second, err := srv.GetRecommendations(context.Background(), &pbc.GetRecommendationsRequest{
		PageToken: first.GetNextPageToken(),
	})
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if len(second.GetRecommendations()) != 1 {
		t.Fatalf("second page length = %d", len(second.GetRecommendations()))
	}
	got := second.GetRecommendations()[0]
	if got.GetId() != last.Id {
		t.Fatalf("id = %q, source id = %q", got.GetId(), last.Id)
	}
	if got.GetImpact().GetEstimatedSavings() != last.Savings {
		t.Fatalf("savings = %v, source savings = %v", got.GetImpact().GetEstimatedSavings(), last.Savings)
	}
	if got.GetResource().GetId() != last.ResourceID {
		t.Fatalf("resource id = %q, source = %q", got.GetResource().GetId(), last.ResourceID)
	}
	if second.GetNextPageToken() != "" {
		t.Fatalf("next_page_token = %q", second.GetNextPageToken())
	}
}

func TestGetRecommendationsRejectsTooManyTargets(t *testing.T) {
	api := &fakeAPI{}
	srv := newTestServer(t, "nam")
	srv.UseCostAPI(api, []string{"bc-1"}, "")
	targets := make([]*pbc.ResourceDescriptor, maxTargetResources+1)
	for i := range targets {
		targets[i] = &pbc.ResourceDescriptor{Provider: "aws", ResourceType: "aws-ec2"}
	}
	_, err := srv.GetRecommendations(context.Background(), &pbc.GetRecommendationsRequest{
		TargetResources: targets,
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("code = %s, err = %v", status.Code(err), err)
	}
	if api.recommendationCalls != 0 {
		t.Fatalf("index calls = %d", api.recommendationCalls)
	}
}

func TestRecommendationsRequireClient(t *testing.T) {
	srv := newTestServer(t, "nam")
	_, err := srv.GetRecommendations(context.Background(), &pbc.GetRecommendationsRequest{})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("GetRecommendations code = %s", status.Code(err))
	}
	_, err = srv.DismissRecommendation(context.Background(), &pbc.DismissRecommendationRequest{
		RecommendationId: "rec-1",
	})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("DismissRecommendation code = %s", status.Code(err))
	}
}

func TestGetPluginInfoAdvertisesRecommendations(t *testing.T) {
	info, err := newTestServer(t, "nam").GetPluginInfo(context.Background(), &pbc.GetPluginInfoRequest{})
	if err != nil {
		t.Fatalf("GetPluginInfo: %v", err)
	}
	caps := map[pbc.PluginCapability]bool{}
	for _, cap := range info.GetCapabilities() {
		caps[cap] = true
	}
	for _, want := range []pbc.PluginCapability{
		pbc.PluginCapability_PLUGIN_CAPABILITY_RECOMMENDATIONS,
		pbc.PluginCapability_PLUGIN_CAPABILITY_DISMISS_RECOMMENDATIONS,
	} {
		if !caps[want] {
			t.Errorf("missing capability %s", want)
		}
	}
	rpcs := info.GetMetadata()["implemented_rpcs"]
	for _, name := range []string{"GetRecommendations", "DismissRecommendation"} {
		if !rpcListed(rpcs, name) {
			t.Errorf("implemented_rpcs = %q, missing %s", rpcs, name)
		}
	}
}

func recommendationByID(t *testing.T, resp *pbc.GetRecommendationsResponse, id string) *pbc.Recommendation {
	t.Helper()
	for _, rec := range resp.GetRecommendations() {
		if rec.GetId() == id {
			return rec
		}
	}
	t.Fatalf("missing recommendation %s", id)
	return nil
}

func assertAbsent(t *testing.T, resp *pbc.GetRecommendationsResponse, ids ...string) {
	t.Helper()
	seen := map[string]bool{}
	for _, rec := range resp.GetRecommendations() {
		seen[rec.GetId()] = true
	}
	for _, id := range ids {
		if seen[id] {
			t.Fatalf("recommendation %s was returned", id)
		}
	}
}

func rpcListed(list, name string) bool {
	for _, part := range strings.Split(list, ",") {
		if part == name {
			return true
		}
	}
	return false
}

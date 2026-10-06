//go:build integration

package integration

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	flexeraclient "github.com/flexera-public/unified-go-client"

	"github.com/rshade/finfocus-plugin-flexera/internal/flexeraapi"
)

// TestLiveCostsSelect calls the Flexera Bill Analysis API.
// It runs only when OAuth credentials and a billing center id are set:
//
//	export FLEXERA_ORG_ID=12345
//	export FLEXERA_REFRESH_TOKEN=...          # or CLIENT_ID and CLIENT_SECRET
//	export FLEXERA_BILLING_CENTER_IDS=bc-...
//	export FLEXERA_REGION=nam                 # nam, eu, or apac
//	go test -tags=integration ./test/integration/...
func TestLiveCostsSelect(t *testing.T) {
	org := os.Getenv("FLEXERA_ORG_ID")
	refresh := os.Getenv("FLEXERA_REFRESH_TOKEN")
	clientID := os.Getenv("FLEXERA_CLIENT_ID")
	clientSecret := os.Getenv("FLEXERA_CLIENT_SECRET")
	centers := os.Getenv("FLEXERA_BILLING_CENTER_IDS")
	if org == "" || centers == "" || (refresh == "" && (clientID == "" || clientSecret == "")) {
		t.Skip("set FLEXERA_ORG_ID, FLEXERA_BILLING_CENTER_IDS, and OAuth credentials")
	}
	orgID, err := strconv.ParseInt(org, 10, 64)
	if err != nil {
		t.Fatalf("FLEXERA_ORG_ID: %v", err)
	}
	zone, err := flexeraapi.ZoneForRegion(os.Getenv("FLEXERA_REGION"))
	if err != nil {
		t.Fatalf("region: %v", err)
	}
	auth := flexeraclient.OAuthConfig{RefreshToken: refresh, ClientID: clientID, ClientSecret: clientSecret}
	api, err := flexeraapi.New(context.Background(), zone, orgID, auth, nil)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	currency, err := api.CurrencyCode(ctx)
	if err != nil {
		t.Fatalf("currency: %v", err)
	}
	if currency == "" {
		t.Fatal("empty currency")
	}
	end := time.Now().UTC().Truncate(24 * time.Hour)
	start := end.AddDate(0, 0, -7)
	granularity := "day"
	ids := strings.Split(centers, ",")
	resp, err := api.CostsSelect(ctx, flexeraapi.CostsSelectRequest{
		BillingCenterIDs: []string{strings.TrimSpace(ids[0])},
		Metrics:          []string{"cost_amortized_unblended_adj"},
		Dimensions:       []string{"vendor", "service"},
		StartAt:          start.Format(time.DateOnly),
		EndAt:            end.Format(time.DateOnly),
		Limit:            10,
		Granularity:      &granularity,
	})
	if err != nil {
		t.Fatalf("costs/select: %v", err)
	}
	t.Logf("currency=%s rows=%d accepted=%t", currency, len(resp.Rows), resp.Accepted)
}

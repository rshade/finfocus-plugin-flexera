package flexeraapi

import (
	"net/http"
	"testing"
	"time"

	flexera "github.com/flexera-public/unified-go-client"
)

func TestSelectPayloadOK(t *testing.T) {
	payload, accepted, err := selectPayload(&flexera.BillAnalysisCostsSelectResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &flexera.BillAnalysisAnalyticsQueryResult{
			Rows: []flexera.BillAnalysisRow{{
				Timestamp: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			}},
		},
	})
	if err != nil || accepted || payload == nil || len(payload.Rows) != 1 {
		t.Fatalf("payload=%v accepted=%t err=%v", payload, accepted, err)
	}
}

func TestSelectPayloadAcceptedWithoutRows(t *testing.T) {
	_, _, err := selectPayload(&flexera.BillAnalysisCostsSelectResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusAccepted},
	})
	if err == nil {
		t.Fatal("expected error for 202 without rows")
	}
}

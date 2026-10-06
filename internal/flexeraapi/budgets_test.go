package flexeraapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	flexera "github.com/flexera-public/unified-go-client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBudgetIndexShowAndReport(t *testing.T) {
	amount := 125.5
	spend := 40.25
	index := flexera.BudgetBudgetList{Values: []flexera.BudgetBudgetLink{{
		Id:   "bud-1",
		Name: "Platform",
	}}}
	shown := flexera.BudgetBudget{
		Id:       "bud-1",
		Name:     "Platform",
		Segments: []flexera.BudgetBudgetSegment{{BudgetAmounts: []float64{amount}}},
	}
	report := flexera.BudgetBudgetReportRowList{Values: []flexera.BudgetBudgetReportRow{{
		Metrics: flexera.BudgetBudgetMetrics{BudgetAmount: &amount, SpendAmount: &spend},
	}}}

	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) == 1 && strings.HasSuffix(r.URL.Path, "/budgets") {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(w, "refresh_token="+sampleRefresh)
			return
		}
		write := r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodDelete
		if write || strings.Contains(r.URL.Path, "/create") {
			t.Errorf("write %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		var payload any
		switch {
		case strings.HasSuffix(r.URL.Path, "/report"):
			payload = report
		case strings.HasSuffix(r.URL.Path, "/budgets"):
			payload = index
		default:
			payload = shown
		}
		require.NoError(t, json.NewEncoder(w).Encode(payload))
	}))
	t.Cleanup(server.Close)

	client := staticClient(t, server.URL)
	gotIndex, err := client.BudgetIndex(context.Background())
	require.NoError(t, err)
	require.Len(t, gotIndex.Values, 1)
	assert.Equal(t, index.Values[0].Id, gotIndex.Values[0].Id)
	assert.Equal(t, index.Values[0].Name, gotIndex.Values[0].Name)
	assert.GreaterOrEqual(t, hits.Load(), int32(2))

	gotShow, err := client.BudgetShow(context.Background(), shown.Id)
	require.NoError(t, err)
	require.NotEmpty(t, gotShow.Segments)
	assert.Equal(t, shown.Segments[0].BudgetAmounts, gotShow.Segments[0].BudgetAmounts)

	gotReport, err := client.BudgetReport(context.Background(), shown.Id, &flexera.BudgetBudgetReportParams{
		StartAt: "2026-01",
		EndAt:   "2026-02",
	})
	require.NoError(t, err)
	require.Len(t, gotReport.Values, 1)
	require.NotNil(t, gotReport.Values[0].Metrics.BudgetAmount)
	require.NotNil(t, gotReport.Values[0].Metrics.SpendAmount)
	assert.Equal(t, *report.Values[0].Metrics.BudgetAmount, *gotReport.Values[0].Metrics.BudgetAmount)
	assert.Equal(t, *report.Values[0].Metrics.SpendAmount, *gotReport.Values[0].Metrics.SpendAmount)

	denied := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, "client_secret="+sampleSecret)
	}))
	t.Cleanup(denied.Close)
	_, err = staticClient(t, denied.URL).BudgetIndex(context.Background())
	require.Error(t, err)
	assert.NotContains(t, err.Error(), sampleSecret)
}

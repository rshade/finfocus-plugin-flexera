package flexeraapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	flexera "github.com/flexera-public/unified-go-client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	sampleRefresh = "refresh-token-super-secret-value"
	sampleSecret  = "client-secret-super-value"
	sampleJWT     = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U"
)

func fastRetry() retryPolicy {
	return retryPolicy{
		maxAttempts:   defaultMaxAttempts,
		baseDelay:     5 * time.Millisecond,
		maxDelay:      30 * time.Millisecond,
		maxRetryAfter: time.Second,
	}
}

func staticClient(t *testing.T, serverURL string) *clientImpl {
	t.Helper()
	helper, err := flexera.NewAuthHelper(flexera.AuthHelperConfig{
		Zone:       flexera.ZoneNAM,
		APIBaseURL: serverURL,
	})
	require.NoError(t, err)
	unified, err := helper.NewStaticTokenClient("static-test-token")
	require.NoError(t, err)
	return &clientImpl{org: 12345, client: unified, retry: fastRetry()}
}

func selectReq() CostsSelectRequest {
	return CostsSelectRequest{
		BillingCenterIDs: []string{"bc-1"},
		Metrics:          []string{"cost_amortized_unblended_adj"},
		Dimensions:       []string{"vendor"},
		StartAt:          "2026-09-01",
		EndAt:            "2026-09-08",
		Limit:            10,
		Granularity:      strPtr("day"),
	}
}

func writeCostRows(w http.ResponseWriter, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"rows": []map[string]any{{
			"timestamp":  "2026-09-01T00:00:00Z",
			"dimensions": map[string]string{"vendor": "aws"},
			"metrics":    map[string]float64{"cost_amortized_unblended_adj": 1.5},
		}},
	})
}

func TestCostsSelectRetries429And5xxNot4xx(t *testing.T) {
	secretBody := "refresh_token=" + sampleRefresh + " client_secret=" + sampleSecret + " Bearer " + sampleJWT

	t.Run("retry after zero", func(t *testing.T) {
		var hits atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			if hits.Add(1) == 1 {
				w.Header().Set("Retry-After", "0")
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = io.WriteString(w, secretBody)
				return
			}
			writeCostRows(w, http.StatusOK)
		}))
		t.Cleanup(server.Close)

		resp, err := staticClient(t, server.URL).CostsSelect(context.Background(), selectReq())
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, int32(2), hits.Load())
		assert.Len(t, resp.Rows, 1)
	})

	t.Run("five hundred then success", func(t *testing.T) {
		var hits atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			if hits.Add(1) < 3 {
				w.WriteHeader(http.StatusBadGateway)
				_, _ = io.WriteString(w, "refresh_token="+sampleRefresh)
				return
			}
			writeCostRows(w, http.StatusOK)
		}))
		t.Cleanup(server.Close)

		resp, err := staticClient(t, server.URL).CostsSelect(context.Background(), selectReq())
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.GreaterOrEqual(t, hits.Load(), int32(3))
	})

	t.Run("other 4xx is not retried and is redacted", func(t *testing.T) {
		var hits atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			hits.Add(1)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, secretBody)
		}))
		t.Cleanup(server.Close)

		resp, err := staticClient(t, server.URL).CostsSelect(context.Background(), selectReq())
		require.Error(t, err)
		assert.Nil(t, resp)
		assert.Equal(t, int32(1), hits.Load())
		assert.NotContains(t, err.Error(), sampleRefresh)
		assert.NotContains(t, err.Error(), sampleSecret)
		assert.NotContains(t, err.Error(), sampleJWT)
		var statusErr *StatusError
		require.ErrorAs(t, err, &statusErr)
		assert.Equal(t, http.StatusBadRequest, statusErr.Status)
	})
}

func TestCostsSelectRetryHonorsCanceledContext(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	t.Cleanup(server.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	_, err := staticClient(t, server.URL).CostsSelect(ctx, selectReq())
	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Equal(t, int32(1), hits.Load())
}

func TestTokenErrorRedactsRefreshToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write(body)
	}))
	t.Cleanup(server.Close)

	helper, err := flexera.NewAuthHelper(flexera.AuthHelperConfig{
		Zone:         flexera.ZoneNAM,
		APIBaseURL:   server.URL,
		LoginBaseURL: server.URL,
	})
	require.NoError(t, err)
	unified, err := helper.NewOAuthClientWithResponses(flexera.OAuthConfig{
		RefreshToken: sampleRefresh,
		ClientID:     "client-id",
		ClientSecret: sampleSecret,
	})
	require.NoError(t, err)
	client := &clientImpl{org: 12345, client: unified, retry: fastRetry()}

	_, err = client.CostsSelect(context.Background(), selectReq())
	require.Error(t, err)
	assert.NotContains(t, err.Error(), sampleRefresh)
	assert.NotContains(t, err.Error(), sampleSecret)
}

func TestCurrencyAndForecastRetryAndErrors(t *testing.T) {
	t.Run("currency retries 429", func(t *testing.T) {
		var hits atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.Contains(r.URL.Path, "currency_code") {
				http.NotFound(w, r)
				return
			}
			if hits.Add(1) == 1 {
				w.Header().Set("Retry-After", "0")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = io.WriteString(w, "access_token="+sampleJWT)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"id":"currency_code","kind":"string","value":"EUR"}`)
		}))
		t.Cleanup(server.Close)
		code, err := staticClient(t, server.URL).CurrencyCode(context.Background())
		require.NoError(t, err)
		assert.Equal(t, "EUR", code)
		assert.Equal(t, int32(2), hits.Load())
	})

	t.Run("currency empty and 404", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"id":"currency_code","kind":"string","value":"  "}`)
		}))
		t.Cleanup(server.Close)
		_, err := staticClient(t, server.URL).CurrencyCode(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "empty")

		denied := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, "client_secret="+sampleSecret)
		}))
		t.Cleanup(denied.Close)
		_, err = staticClient(t, denied.URL).CurrencyCode(context.Background())
		require.Error(t, err)
		assert.NotContains(t, err.Error(), sampleSecret)
		var statusErr *StatusError
		require.ErrorAs(t, err, &statusErr)
		assert.Equal(t, http.StatusNotFound, statusErr.Status)
	})

	t.Run("forecast retries 500 and skips 400", func(t *testing.T) {
		var hits atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			if hits.Add(1) == 1 {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = io.WriteString(w, "refresh_token="+sampleRefresh)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"segments":[{"forecastAmounts":[1.25,2.75],"actualAmounts":[1]}]}`)
		}))
		t.Cleanup(server.Close)
		resp, err := staticClient(t, server.URL).ForecastReport(context.Background(), ForecastRequest{
			BillingCenterIDs: []string{"bc-1"},
			Dimensions:       []string{"vendor"},
			StartAt:          "2026-09",
			EndAt:            "2026-10",
			Granularity:      "month",
			LookbackPeriod:   3,
			Metric:           "cost_amortized_unblended_adj",
		})
		require.NoError(t, err)
		require.Len(t, resp.Segments, 1)
		assert.Equal(t, []float64{1.25, 2.75}, resp.Segments[0].ForecastAmounts)
		assert.GreaterOrEqual(t, hits.Load(), int32(2))

		var badHits atomic.Int32
		bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			badHits.Add(1)
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, "client_secret="+sampleSecret)
		}))
		t.Cleanup(bad.Close)
		_, err = staticClient(t, bad.URL).ForecastReport(context.Background(), ForecastRequest{
			BillingCenterIDs: []string{"bc-1"},
			Dimensions:       []string{"vendor"},
			StartAt:          "2026-09",
			EndAt:            "2026-10",
			LookbackPeriod:   3,
		})
		require.Error(t, err)
		assert.Equal(t, int32(1), badHits.Load())
		assert.NotContains(t, err.Error(), sampleSecret)
	})
}

func TestCostsSelectAcceptedAndValidation(t *testing.T) {
	t.Run("202 with rows", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			writeCostRows(w, http.StatusAccepted)
		}))
		t.Cleanup(server.Close)
		resp, err := staticClient(t, server.URL).CostsSelect(context.Background(), selectReq())
		require.NoError(t, err)
		assert.True(t, resp.Accepted)
		assert.Len(t, resp.Rows, 1)
	})

	t.Run("202 without rows", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusAccepted)
		}))
		t.Cleanup(server.Close)
		_, err := staticClient(t, server.URL).CostsSelect(context.Background(), selectReq())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "202")
	})

	t.Run("validation still rejects before http", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			t.Error("validation should not call the server")
		}))
		t.Cleanup(server.Close)
		req := selectReq()
		req.Limit = 0
		_, err := staticClient(t, server.URL).CostsSelect(context.Background(), req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "limit")
	})
}

func TestNewRejectsBadInputWithoutNetwork(t *testing.T) {
	_, err := New(context.Background(), ZoneUS, 0, flexera.OAuthConfig{}, nil)
	require.Error(t, err)

	_, err = New(context.Background(), Zone("nope"), 12, flexera.OAuthConfig{RefreshToken: sampleRefresh}, nil)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), sampleRefresh)

	_, err = New(context.Background(), ZoneUS, 12, flexera.OAuthConfig{}, nil)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), sampleRefresh)
}

func TestZoneForRegion(t *testing.T) {
	tests := []struct {
		region string
		zone   Zone
		ok     bool
	}{
		{region: "nam", zone: ZoneUS, ok: true},
		{region: "EU", zone: ZoneEU, ok: true},
		{region: "asia-pacific", zone: ZoneAPAC, ok: true},
		{region: "", zone: ZoneUS, ok: true},
		{region: "us-east-1", ok: false},
	}
	for _, test := range tests {
		got, err := ZoneForRegion(test.region)
		if test.ok {
			require.NoError(t, err, test.region)
			assert.Equal(t, test.zone, got)
			continue
		}
		require.Error(t, err, test.region)
	}
}

func TestRetryDelayHonorsRetryAfterAndJitter(t *testing.T) {
	header := http.Header{}
	header.Set("Retry-After", "0")
	assert.Equal(t, time.Duration(0), retryDelay(header, 1, fastRetry()))

	header.Set("Retry-After", time.Now().Add(2*time.Second).UTC().Format(http.TimeFormat))
	delay, ok := parseRetryAfter(header.Get("Retry-After"))
	require.True(t, ok)
	assert.Greater(t, delay, time.Second)

	policy := fastRetry()
	policy.baseDelay = 100 * time.Millisecond
	policy.jitterN = func(n int64) int64 { return 0 }
	low := retryDelay(nil, 1, policy)
	policy.jitterN = func(n int64) int64 { return n - 1 }
	high := retryDelay(nil, 1, policy)
	assert.Less(t, low, high)

	_, ok = parseRetryAfter("not-a-date")
	assert.False(t, ok)
	_, ok = parseRetryAfter("-1")
	assert.False(t, ok)
}

func TestRedactPatterns(t *testing.T) {
	raw := "refresh_token=" + sampleRefresh + ` "client_secret":"` + sampleSecret + `" Bearer ` + sampleJWT
	got := Redact(raw)
	assert.NotContains(t, got, sampleRefresh)
	assert.NotContains(t, got, sampleSecret)
	assert.NotContains(t, got, sampleJWT)
	assert.Nil(t, RedactError(nil))
	assert.NotContains(t, RedactError(errors.New(raw)).Error(), sampleRefresh)
}

func TestFilterSubstringAndNested(t *testing.T) {
	sub := "ec2"
	inner := FilterExpression{Type: "equal", Dimension: strPtr("vendor"), Value: strPtr("aws")}
	expr := &FilterExpression{
		Type:       "not",
		Substring:  &sub,
		Expression: &inner,
	}
	converted := convertFilterExpression(expr)
	require.NotNil(t, converted)
	assert.Equal(t, "ec2", *converted.Substring)
	require.NotNil(t, converted.Expression)
	assert.Equal(t, "aws", *converted.Expression.Value)
}

func TestSelectPayloadEdges(t *testing.T) {
	_, _, err := selectPayload(nil)
	require.Error(t, err)

	_, _, err = selectPayload(&flexera.BillAnalysisCostsSelectResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
	})
	require.Error(t, err)

	truncated := true
	payload, accepted, err := selectPayload(&flexera.BillAnalysisCostsSelectResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusAccepted, Header: http.Header{}},
		JSON200: &flexera.BillAnalysisAnalyticsQueryResult{
			Rows:          []flexera.BillAnalysisRow{{}},
			RowsTruncated: &truncated,
		},
	})
	require.NoError(t, err)
	assert.True(t, accepted)
	assert.True(t, *payload.RowsTruncated)

	_, _, err = selectPayload(&flexera.BillAnalysisCostsSelectResponse{
		Body:         []byte("refresh_token=" + sampleRefresh),
		HTTPResponse: &http.Response{StatusCode: http.StatusUnauthorized},
	})
	require.Error(t, err)
	assert.NotContains(t, err.Error(), sampleRefresh)
}

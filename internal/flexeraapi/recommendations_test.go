package flexeraapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	flexera "github.com/flexera-public/unified-go-client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecommendationsIndexAndStatusUpdate(t *testing.T) {
	want := flexera.OptimaRecommendationsRecommendationResultResponse{
		Id:         "rec-1",
		Savings:    12.5,
		ResourceID: "i-abc",
		Status:     string(flexera.Active),
		CreatedAt:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt:  time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
		Kind:       flexera.OptimaRecommendation,
		Type:       flexera.UsageReduction,
	}

	t.Run("index returns the generated model", func(t *testing.T) {
		var hits atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			if hits.Add(1) == 1 {
				w.Header().Set("Retry-After", "0")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = io.WriteString(w, "refresh_token="+sampleRefresh)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			payload := []flexera.OptimaRecommendationsRecommendationResultResponse{want}
			require.NoError(t, json.NewEncoder(w).Encode(payload))
		}))
		t.Cleanup(server.Close)

		got, err := staticClient(t, server.URL).RecommendationsIndex(context.Background(), nil)
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, want.Id, got[0].Id)
		assert.Equal(t, want.Savings, got[0].Savings)
		assert.Equal(t, want.ResourceID, got[0].ResourceID)
		assert.Equal(t, int32(2), hits.Load())
	})

	t.Run("index 4xx is not retried", func(t *testing.T) {
		var hits atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			hits.Add(1)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, "client_secret="+sampleSecret)
		}))
		t.Cleanup(server.Close)

		got, err := staticClient(t, server.URL).RecommendationsIndex(context.Background(), nil)
		require.Error(t, err)
		assert.Nil(t, got)
		assert.Equal(t, int32(1), hits.Load())
		assert.NotContains(t, err.Error(), sampleSecret)
	})

	t.Run("status update 204 is success", func(t *testing.T) {
		var raw []byte
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw, _ = io.ReadAll(r.Body)
			w.WriteHeader(http.StatusNoContent)
		}))
		t.Cleanup(server.Close)

		body := flexera.OptimaRecommendationsRecommendationsUpdateStatusRequestBody{
			Id:     want.Id,
			Status: flexera.OptimaRecommendationsRecommendationsUpdateStatusRequestBodyStatusRejected,
		}
		err := staticClient(t, server.URL).UpdateRecommendationStatus(context.Background(), body)
		require.NoError(t, err)
		var sent flexera.OptimaRecommendationsRecommendationsUpdateStatusRequestBody
		require.NoError(t, json.Unmarshal(raw, &sent))
		assert.Equal(t, body.Id, sent.Id)
		assert.Equal(t, body.Status, sent.Status)
	})

	t.Run("status update 4xx is not retried", func(t *testing.T) {
		var hits atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			hits.Add(1)
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, "Bearer "+sampleJWT)
		}))
		t.Cleanup(server.Close)

		body := flexera.OptimaRecommendationsRecommendationsUpdateStatusRequestBody{
			Id:     want.Id,
			Status: flexera.OptimaRecommendationsRecommendationsUpdateStatusRequestBodyStatusRejected,
		}
		err := staticClient(t, server.URL).UpdateRecommendationStatus(context.Background(), body)
		require.Error(t, err)
		assert.Equal(t, int32(1), hits.Load())
		assert.NotContains(t, err.Error(), sampleJWT)
	})
}

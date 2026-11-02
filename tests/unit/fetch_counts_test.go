package unit_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github-features/internal/fetchcounts"
	"github.com/aws/aws-lambda-go/events"
)

func TestHandlerRecordsViewAndReturnsSVG(t *testing.T) {
	username := `user"with\characters`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.Method, http.MethodPost; got != want {
			t.Errorf("method = %q, want %q", got, want)
		}
		if got, want := r.URL.Path, "/rest/v1/rpc/record_view"; got != want {
			t.Errorf("path = %q, want %q", got, want)
		}
		if got, want := r.Header.Get("Authorization"), "Bearer test-service-role-key"; got != want {
			t.Errorf("Authorization = %q, want %q", got, want)
		}
		if got, want := r.Header.Get("apikey"), "test-service-role-key"; got != want {
			t.Errorf("apikey = %q, want %q", got, want)
		}

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode RPC request: %v", err)
		}
		if got, want := payload["p_username"], username; got != want {
			t.Errorf("p_username = %#v, want %q", got, want)
		}
		if got, want := payload["p_initial_views"], float64(1); got != want {
			t.Errorf("p_initial_views = %#v, want %v", got, want)
		}
		responseBody, err := json.Marshal([]map[string]any{
			{"id": 1, "username": username, "views_count": 12},
		})
		if err != nil {
			t.Errorf("marshal RPC response: %v", err)
			return
		}
		_, _ = w.Write(responseBody)
	}))
	defer server.Close()
	configureSupabase(t, server.URL)

	response, err := fetchcounts.Handler(context.Background(), events.APIGatewayProxyRequest{
		HTTPMethod:            http.MethodGet,
		QueryStringParameters: map[string]string{"username": username},
	})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if got, want := response.StatusCode, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d; body: %s", got, want, response.Body)
	}
	if got, want := response.Headers["Content-Type"], "image/svg+xml"; got != want {
		t.Errorf("Content-Type = %q, want %q", got, want)
	}
	if got, want := response.Headers["Cache-Control"], "no-store, no-cache, must-revalidate"; got != want {
		t.Errorf("Cache-Control = %q, want %q", got, want)
	}
	if !strings.Contains(response.Body, `aria-label="Profile views: 12"`) {
		t.Errorf("SVG response does not show returned count: %s", response.Body)
	}
}

func TestHandlerUsesOptionalInitialCount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode RPC request: %v", err)
		}
		if got, want := payload["p_initial_views"], float64(25); got != want {
			t.Errorf("p_initial_views = %#v, want %v", got, want)
		}
		_, _ = fmt.Fprint(w, `[{"id":1,"username":"octocat","views_count":25}]`)
	}))
	defer server.Close()
	configureSupabase(t, server.URL)

	response, err := fetchcounts.Handler(context.Background(), events.APIGatewayProxyRequest{
		HTTPMethod: http.MethodGet,
		QueryStringParameters: map[string]string{
			"username":    "octocat",
			"views_count": "25",
		},
	})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if got, want := response.StatusCode, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d; body: %s", got, want, response.Body)
	}
	if !strings.Contains(response.Body, `aria-label="Profile views: 25"`) {
		t.Errorf("SVG response does not show initial count: %s", response.Body)
	}
}

func TestHandlerRejectsInvalidRequests(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		parameters map[string]string
		wantStatus int
	}{
		{
			name:       "method not allowed",
			method:     http.MethodPost,
			parameters: map[string]string{"username": "octocat"},
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:       "missing username",
			method:     http.MethodGet,
			parameters: map[string]string{},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "whitespace username",
			method:     http.MethodGet,
			parameters: map[string]string{"username": "  "},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid count",
			method:     http.MethodGet,
			parameters: map[string]string{"username": "octocat", "views_count": "many"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "negative count",
			method:     http.MethodGet,
			parameters: map[string]string{"username": "octocat", "views_count": "-1"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "count overflow",
			method:     http.MethodGet,
			parameters: map[string]string{"username": "octocat", "views_count": "9223372036854775808"},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response, err := fetchcounts.Handler(context.Background(), events.APIGatewayProxyRequest{
				HTTPMethod:            tt.method,
				QueryStringParameters: tt.parameters,
			})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if got := response.StatusCode; got != tt.wantStatus {
				t.Errorf("status = %d, want %d", got, tt.wantStatus)
			}
		})
	}
}

func TestHandlerReturnsErrorWhenSupabaseFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	configureSupabase(t, server.URL)

	response, err := fetchcounts.Handler(context.Background(), events.APIGatewayProxyRequest{
		HTTPMethod:            http.MethodGet,
		QueryStringParameters: map[string]string{"username": "octocat"},
	})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if got, want := response.StatusCode, http.StatusInternalServerError; got != want {
		t.Errorf("status = %d, want %d", got, want)
	}
	if !strings.Contains(response.Body, "database unavailable") {
		t.Errorf("error response = %q, want Supabase error details", response.Body)
	}
}

func TestHandlerRejectsInvalidRPCResponses(t *testing.T) {
	tests := []struct {
		name         string
		responseBody string
	}{
		{name: "invalid JSON", responseBody: `not json`},
		{name: "no result", responseBody: `[]`},
		{name: "multiple results", responseBody: `[{"id":1,"username":"a","views_count":1},{"id":2,"username":"b","views_count":2}]`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.WriteString(w, tt.responseBody)
			}))
			defer server.Close()
			configureSupabase(t, server.URL)

			response, err := fetchcounts.Handler(context.Background(), events.APIGatewayProxyRequest{
				HTTPMethod:            http.MethodGet,
				QueryStringParameters: map[string]string{"username": "octocat"},
			})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if got, want := response.StatusCode, http.StatusInternalServerError; got != want {
				t.Errorf("status = %d, want %d", got, want)
			}
		})
	}
}

func TestRecordViewRequiresConfiguration(t *testing.T) {
	t.Setenv("SUPABASE_URL", "")
	t.Setenv("SUPABASE_SERVICE_ROLE_KEY", "")

	response, err := fetchcounts.Handler(context.Background(), events.APIGatewayProxyRequest{
		HTTPMethod:            http.MethodGet,
		QueryStringParameters: map[string]string{"username": "octocat"},
	})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if got, want := response.StatusCode, http.StatusInternalServerError; got != want {
		t.Errorf("status = %d, want %d", got, want)
	}
}

func TestHandlerSVGIncludesViewCount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `[{"id":1,"username":"octocat","views_count":123}]`)
	}))
	defer server.Close()
	configureSupabase(t, server.URL)

	response, err := fetchcounts.Handler(context.Background(), events.APIGatewayProxyRequest{
		HTTPMethod:            http.MethodGet,
		QueryStringParameters: map[string]string{"username": "octocat"},
	})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if got, want := response.StatusCode, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	for _, want := range []string{
		`aria-label="Profile views: 123"`,
		"<title>Profile views: 123</title>",
		">123</text>",
	} {
		if !strings.Contains(response.Body, want) {
			t.Errorf("SVG does not contain %q", want)
		}
	}
}

func configureSupabase(t *testing.T, baseURL string) {
	t.Helper()
	t.Setenv("SUPABASE_URL", baseURL)
	t.Setenv("SUPABASE_SERVICE_ROLE_KEY", "test-service-role-key")
}

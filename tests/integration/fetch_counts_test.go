package integration

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

func TestFetchCountsPersistsCountThroughNetlifyAndSupabase(t *testing.T) {
	if os.Getenv("RUN_INTEGRATION_TESTS") != "1" {
		t.Skip("set RUN_INTEGRATION_TESTS=1 to run local integration tests")
	}

	baseURL := os.Getenv("NETLIFY_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8888"
	}

	username := fmt.Sprintf("integration-%d", time.Now().UnixNano())
	firstResponse := requestBadge(t, baseURL, username, "40")
	if !strings.Contains(firstResponse, `aria-label="Profile views: 40"`) {
		t.Fatalf("first response did not contain initial count: %s", firstResponse)
	}

	secondResponse := requestBadge(t, baseURL, username, "900")
	if !strings.Contains(secondResponse, `aria-label="Profile views: 41"`) {
		t.Fatalf("second response did not increment existing count: %s", secondResponse)
	}
}

func requestBadge(t *testing.T, baseURL, username, initialCount string) string {
	t.Helper()

	endpoint, err := url.Parse(strings.TrimRight(baseURL, "/") + "/.netlify/functions/fetch-counts")
	if err != nil {
		t.Fatalf("parse Netlify URL: %v", err)
	}
	query := endpoint.Query()
	query.Set("username", username)
	query.Set("views_count", initialCount)
	endpoint.RawQuery = query.Encode()

	request, err := http.NewRequest(http.MethodGet, endpoint.String(), nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("request local Netlify function at %s: %v", endpoint, err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read function response: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("function returned status %d: %s", response.StatusCode, body)
	}
	if contentType := response.Header.Get("Content-Type"); !strings.HasPrefix(contentType, "image/svg+xml") {
		t.Fatalf("Content-Type = %q, want image/svg+xml", contentType)
	}
	return string(body)
}

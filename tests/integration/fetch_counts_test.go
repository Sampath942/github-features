package integration

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
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
	firstResponse := requestBadge(t, baseURL, username, "")
	if !strings.Contains(firstResponse, `aria-label="Profile views: 1"`) {
		t.Fatalf("first response did not start the count at 1: %s", firstResponse)
	}

	secondResponse := requestBadge(t, baseURL, username, "")
	if !strings.Contains(secondResponse, `aria-label="Profile views: 2"`) {
		t.Fatalf("second response did not increment the count: %s", secondResponse)
	}

	overrideResponse := requestBadge(t, baseURL, username, "40")
	if !strings.Contains(overrideResponse, `aria-label="Profile views: 40"`) {
		t.Fatalf("response did not set the requested count: %s", overrideResponse)
	}

	responses := make(chan string, 2)
	errors := make(chan error, 2)
	start := make(chan struct{})
	var requests sync.WaitGroup

	for range 2 {
		requests.Add(1)
		go func() {
			defer requests.Done()
			<-start
			response, err := fetchBadge(baseURL, username, "")
			if err != nil {
				errors <- err
				return
			}
			responses <- response
		}()
	}

	close(start)
	requests.Wait()
	close(responses)
	close(errors)

	var concurrentCounts []string
	for response := range responses {
		switch {
		case strings.Contains(response, `aria-label="Profile views: 41"`):
			concurrentCounts = append(concurrentCounts, "41")
		case strings.Contains(response, `aria-label="Profile views: 42"`):
			concurrentCounts = append(concurrentCounts, "42")
		default:
			t.Errorf("concurrent response did not show count 41 or 42: %s", response)
		}
	}
	for err := range errors {
		t.Errorf("concurrent request failed: %v", err)
	}
	if len(concurrentCounts) != 2 || concurrentCounts[0] == concurrentCounts[1] {
		t.Fatalf("concurrent requests returned counts %v, want one each of 41 and 42", concurrentCounts)
	}

	finalResponse := requestBadge(t, baseURL, username, "")
	if !strings.Contains(finalResponse, `aria-label="Profile views: 43"`) {
		t.Fatalf("two concurrent requests did not increment the count by 2: %s", finalResponse)
	}
}

func requestBadge(t *testing.T, baseURL, username, initialCount string) string {
	t.Helper()

	response, err := fetchBadge(baseURL, username, initialCount)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func fetchBadge(baseURL, username, initialCount string) (string, error) {
	endpoint, err := url.Parse(strings.TrimRight(baseURL, "/") + "/.netlify/functions/fetch-counts")
	if err != nil {
		return "", fmt.Errorf("parse Netlify URL: %w", err)
	}
	query := endpoint.Query()
	query.Set("username", username)
	if initialCount != "" {
		query.Set("views_count", initialCount)
	}
	endpoint.RawQuery = query.Encode()

	request, err := http.NewRequest(http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("request local Netlify function at %s: %w", endpoint, err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return "", fmt.Errorf("read function response: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("function returned status %d: %s", response.StatusCode, body)
	}
	if contentType := response.Header.Get("Content-Type"); !strings.HasPrefix(contentType, "image/svg+xml") {
		return "", fmt.Errorf("Content-Type = %q, want image/svg+xml", contentType)
	}
	return string(body), nil
}

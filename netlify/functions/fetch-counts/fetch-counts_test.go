package main

import (
	"net/url"
	"testing"
)

func TestViewCountEndpointEscapesUsernameAsOneFilter(t *testing.T) {
	username := `alice&select=*&name,"quoted"\`
	query := url.Values{}
	query.Set("username", postgrestEquals(username))

	endpoint, err := viewCountEndpoint("https://example.supabase.co/", query)
	if err != nil {
		t.Fatalf("viewCountEndpoint() error = %v", err)
	}

	parsed, err := url.Parse(endpoint)
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}
	if got, want := parsed.Path, "/rest/v1/view_count"; got != want {
		t.Errorf("path = %q, want %q", got, want)
	}
	if got, want := parsed.Query().Get("username"), `eq."alice&select=*&name,\"quoted\"\\`+`"`; got != want {
		t.Errorf("username filter = %q, want %q", got, want)
	}
	if got := parsed.Query().Get("select"); got != "" {
		t.Errorf("unexpected extra select parameter = %q", got)
	}
}

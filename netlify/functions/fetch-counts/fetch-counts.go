package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github-features/models"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
)

func handler(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	if req.HTTPMethod != http.MethodPost {
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusMethodNotAllowed,
			Body:       "Method not allowed",
		}, nil
	}

	username := req.QueryStringParameters["username"]
	if strings.TrimSpace(username) == "" {
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusBadRequest,
			Body:       "Username is required",
		}, nil
	}

	supabaseURL := os.Getenv("SUPABASE_URL")
	serviceRoleKey := os.Getenv("SUPABASE_SERVICE_ROLE_KEY")
	if supabaseURL == "" || serviceRoleKey == "" {
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusInternalServerError,
			Body:       "Missing SUPABASE_URL or SUPABASE_SERVICE_ROLE_KEY",
		}, nil
	}

	query := url.Values{}
	query.Set("select", "id,views")
	query.Set("username", postgrestEquals(username))
	endpoint, err := viewCountEndpoint(supabaseURL, query)
	if err != nil {
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusInternalServerError,
			Body:       "Invalid Supabase URL",
		}, nil
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusInternalServerError,
			Body:       err.Error(),
		}, nil
	}

	httpReq.Header.Set("Authorization", "Bearer "+serviceRoleKey)
	httpReq.Header.Set("apikey", serviceRoleKey)
	httpReq.Header.Set("Accept", "application/json")

	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusInternalServerError,
			Body:       err.Error(),
		}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusBadGateway,
			Body:       string(b),
		}, nil
	}

	var viewCounts []models.ViewCount
	if err := json.NewDecoder(resp.Body).Decode(&viewCounts); err != nil {
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusBadGateway,
			Body:       "Invalid response from Supabase",
		}, nil
	}
	if len(viewCounts) == 0 {
		if err := insertViewsCount(ctx, username); err != nil {
			return events.APIGatewayProxyResponse{
				StatusCode: http.StatusBadGateway,
				Body:       "Failed to insert view count",
			}, nil
		}

		respBody, _ := json.Marshal(map[string]string{
			"status": "Inserted views successfully",
			"views":  strconv.FormatInt(1, 10),
		})

		return events.APIGatewayProxyResponse{
			StatusCode: 200,
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
			Body: string(respBody),
		}, nil
		
	}
	if len(viewCounts) != 1 {
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusConflict,
			Body:       "Username matches multiple view counts",
		}, nil
	}

	viewsCount := viewCounts[0].Views + 1
	if err := updateViewsCount(ctx, viewCounts[0].ID, viewsCount); err != nil {
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusBadGateway,
			Body:       "Failed to update view count",
		}, nil
	}

	respBody, _ := json.Marshal(map[string]string{
		"status": "Updated views successfully",
		"views":  strconv.FormatInt(viewsCount, 10),
	})

	return events.APIGatewayProxyResponse{
		StatusCode: 200,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: string(respBody),
	}, nil
}

func insertViewsCount(ctx context.Context, username string) error {
	supabaseURL := os.Getenv("SUPABASE_URL")
	serviceRoleKey := os.Getenv("SUPABASE_SERVICE_ROLE_KEY")
	if supabaseURL == "" || serviceRoleKey == "" {
		return fmt.Errorf("missing SUPABASE_URL or SUPABASE_SERVICE_ROLE_KEY")
	}

	payload := map[string]string{
		"username":      username,
		"views_count": "1",
	}

	body, err := json.Marshal(payload)
	if err != nil {
		fmt.Println("Failed to marshal payload:", err)
		return err
	}


	query := url.Values{}
	endpoint, err := viewCountEndpoint(supabaseURL, query)
	if err != nil {
		return err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBuffer(body))
	if err != nil {
		return err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Prefer", "return=representation")
	httpReq.Header.Set("Authorization", "Bearer "+serviceRoleKey)
	httpReq.Header.Set("apikey", serviceRoleKey)

	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("supabase insert failed with status %d", resp.StatusCode)
	}

	var updated []models.ViewCount
	if err := json.NewDecoder(resp.Body).Decode(&updated); err != nil {
		return fmt.Errorf("decode Supabase update response: %w", err)
	}
	if len(updated) != 1 {
		return fmt.Errorf("expected one updated row, got %d", len(updated))
	}

	return nil
}


func updateViewsCount(ctx context.Context, id int64, viewsCount int64) error {
	supabaseURL := os.Getenv("SUPABASE_URL")
	serviceRoleKey := os.Getenv("SUPABASE_SERVICE_ROLE_KEY")
	if supabaseURL == "" || serviceRoleKey == "" {
		return fmt.Errorf("missing SUPABASE_URL or SUPABASE_SERVICE_ROLE_KEY")
	}

	payload := map[string]int64{
		"views": viewsCount,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	query := url.Values{}
	query.Set("id", fmt.Sprintf("eq.%d", id))
	query.Set("select", "id,views")
	endpoint, err := viewCountEndpoint(supabaseURL, query)
	if err != nil {
		return err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPatch, endpoint, bytes.NewBuffer(body))
	if err != nil {
		return err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Prefer", "return=representation")
	httpReq.Header.Set("Authorization", "Bearer "+serviceRoleKey)
	httpReq.Header.Set("apikey", serviceRoleKey)

	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("supabase update failed with status %d", resp.StatusCode)
	}

	var updated []models.ViewCount
	if err := json.NewDecoder(resp.Body).Decode(&updated); err != nil {
		return fmt.Errorf("decode Supabase update response: %w", err)
	}
	if len(updated) != 1 {
		return fmt.Errorf("expected one updated row, got %d", len(updated))
	}

	return nil
}

func viewCountEndpoint(baseURL string, query url.Values) (string, error) {
	endpoint, err := url.Parse(strings.TrimRight(baseURL, "/") + "/rest/v1/view_count")
	if err != nil {
		return "", err
	}
	endpoint.RawQuery = query.Encode()
	return endpoint.String(), nil
}

func postgrestEquals(value string) string {
	escaped := strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(value)
	return `eq."` + escaped + `"`
}

func main() {
	lambda.Start(handler)
}

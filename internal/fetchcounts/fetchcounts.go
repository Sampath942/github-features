package fetchcounts

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
)

func Handler(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	var updated_views int64 = 1
	if req.HTTPMethod != http.MethodGet {
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusMethodNotAllowed,
			Body:       "Method not allowed" + req.HTTPMethod,
		}, nil
	}

	username := req.QueryStringParameters["username"]
	if strings.TrimSpace(username) == "" {
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusBadRequest,
			Body:       "Username is required",
		}, nil
	}
	view_count := req.QueryStringParameters["views_count"]
	if strings.TrimSpace(view_count) != "" {
		view_count_int, err := strconv.ParseInt(view_count, 10, 64)
		if err != nil {
			return events.APIGatewayProxyResponse{
				StatusCode: http.StatusBadRequest,
				Body:       "Invalid views_count value",
			}, nil
		}
		if view_count_int < 0 {
			return events.APIGatewayProxyResponse{
				StatusCode: http.StatusBadRequest,
				Body:       "views_count must be a non-negative integer",
			}, nil
		}
		updated_views = view_count_int
	}

	viewcount, err := recordView(ctx, username, updated_views)

	if err != nil {
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusInternalServerError,
			Body:       fmt.Sprintf("Error recording view: %v", err),
		}, nil
	}

	svg := generateSVG(viewcount.Views)

	return events.APIGatewayProxyResponse{
		StatusCode: 200,
		Headers: map[string]string{
			"Content-Type":  "image/svg+xml",
			"Cache-Control": "no-store, no-cache, must-revalidate",
		},
		Body: svg,
	}, nil
}

func recordView(
	ctx context.Context,
	username string,
	initialViews int64,
) (models.ViewCount, error) {

	supabaseURL := os.Getenv("SUPABASE_URL")
	serviceRoleKey := os.Getenv("SUPABASE_SERVICE_ROLE_KEY")

	if supabaseURL == "" || serviceRoleKey == "" {
		return models.ViewCount{}, fmt.Errorf(
			"missing SUPABASE_URL or SUPABASE_SERVICE_ROLE_KEY",
		)
	}

	payload := map[string]any{
		"p_username":      username,
		"p_initial_views": initialViews,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return models.ViewCount{}, err
	}

	endpoint, err := url.Parse(
		strings.TrimRight(supabaseURL, "/") +
			"/rest/v1/rpc/record_view",
	)
	if err != nil {
		return models.ViewCount{}, err
	}

	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		endpoint.String(),
		bytes.NewReader(body),
	)
	if err != nil {
		return models.ViewCount{}, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+serviceRoleKey)
	httpReq.Header.Set("apikey", serviceRoleKey)
	httpReq.Header.Set("Accept", "application/json")

	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		return models.ViewCount{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		responseBody, _ := io.ReadAll(resp.Body)
		return models.ViewCount{}, fmt.Errorf(
			"Supabase RPC failed: status %d, body: %s",
			resp.StatusCode,
			string(responseBody),
		)
	}

	var results []models.ViewCount

	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return models.ViewCount{}, fmt.Errorf(
			"decode RPC response: %w",
			err,
		)
	}

	if len(results) != 1 {
		return models.ViewCount{}, fmt.Errorf(
			"expected one result, got %d",
			len(results),
		)
	}

	return results[0], nil
}

func generateSVG(views int64) string {
	return fmt.Sprintf(`
<svg xmlns="http://www.w3.org/2000/svg"
     width="150"
     height="20"
     role="img"
     aria-label="Profile views: %d">
    <title>Profile views: %d</title>
    <linearGradient id="b" x2="0" y2="100%%">
        <stop offset="0" stop-color="#bbb" stop-opacity=".1"/>
        <stop offset="1" stop-opacity=".1"/>
    </linearGradient>
    <clipPath id="r">
        <rect width="150" height="20" rx="3" fill="#fff"/>
    </clipPath>
    <g clip-path="url(#r)">
        <rect width="95" height="20" fill="#555"/>
        <rect x="95" width="55" height="20" fill="#0e75b6"/>
        <rect width="150" height="20" fill="url(#b)"/>
    </g>
    <g fill="#fff" text-anchor="middle"
       font-family="Verdana,Geneva,DejaVu Sans,sans-serif"
       font-size="11">
        <text x="47.5" y="14">Profile views</text>
        <text x="122.5" y="14">%d</text>
    </g>
</svg>`, views, views, views)
}

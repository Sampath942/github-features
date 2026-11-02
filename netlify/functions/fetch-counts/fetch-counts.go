package main

import (
	"github-features/internal/fetchcounts"

	"github.com/aws/aws-lambda-go/lambda"
)

func main() {
	lambda.Start(fetchcounts.Handler)
}

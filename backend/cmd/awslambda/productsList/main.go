package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/aws/aws-lambda-go/lambda"
)

func HandleRequest(ctx context.Context, input map[string]any) (string, error) {
	body, _ := json.Marshal(input)
	return fmt.Sprintf("%s!", string(body)), nil
}

func main() {
	lambda.Start(HandleRequest)
}

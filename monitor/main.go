package main

import (
	"context"

	"github.com/aws/aws-lambda-go/lambda"
	bc "github.com/seniorescobar/bolha/client"
)

func GetActiveAds(ctx context.Context) ([]*bc.ActiveAd, error) {
	client, err := bc.New(&bc.User{
		Username: "",
		Password: "",
	})
	if err != nil {
		return nil, err
	}

	activeAds, err := client.GetActiveAds()
	if err != nil {
		return nil, err
	}

	return activeAds, nil
}

func main() {
	lambda.Start(GetActiveAds)
}

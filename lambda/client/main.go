package main

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/seniorescobar/bolha/client"
)

const (
	qURL = "https://sqs.eu-central-1.amazonaws.com/301808156345/bolha-ads-queue"

	actionUpload = "upload"
	actionRemove = "remove"
)

type Ad struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Price       int    `json:"price"`
	CategoryId  int    `json:"category-id"`
}

func Handler(ctx context.Context, event events.SQSEvent) error {
	for _, record := range event.Records {
		action, ok := record.MessageAttributes["action"]
		if !ok {
			return errors.New("missing action")
		}

		username, ok := record.MessageAttributes["username"]
		if !ok {
			return errors.New("missing username")
		}

		password, ok := record.MessageAttributes["password"]
		if !ok {
			return errors.New("missing password")
		}

		c, err := client.New(&client.User{
			Username: *username.StringValue,
			Password: *password.StringValue,
		})
		if err != nil {
			return err
		}

		switch *action.StringValue {
		case actionUpload:
			var ad Ad
			if err := json.Unmarshal([]byte(record.Body), &ad); err != nil {
				return err
			}

			_, err := uploadAd(c, &ad)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func uploadAd(c *client.Client, ad *Ad) (int64, error) {
	return c.UploadAd(&client.Ad{
		Title:       ad.Title,
		Description: ad.Description,
		Price:       ad.Price,
		CategoryId:  ad.CategoryId,
	})
}

func main() {
	lambda.Start(Handler)
}

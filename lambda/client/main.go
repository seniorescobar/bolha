package main

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/seniorescobar/bolha/client"
	"github.com/seniorescobar/bolha/db/postgres"
)

const (
	qURL = "https://sqs.eu-central-1.amazonaws.com/301808156345/bolha-ads-queue"

	actionUpload = "upload"
	actionRemove = "remove"
)

type Ad struct {
	Id          int64  `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Price       int    `json:"price"`
	CategoryId  int    `json:"category-id"`
}

func Handler(ctx context.Context, event events.SQSEvent) ([]events.SQSMessage, error) {
	pdb, err := postgres.NewFromEnv()
	if err != nil {
		return nil, err
	}
	defer pdb.Close()

	sess, err := session.NewSession()
	if err != nil {
		return nil, err
	}

	svc := sqs.New(sess)

	for _, record := range event.Records {
		action, ok := record.MessageAttributes["action"]
		if !ok {
			return nil, errors.New("missing action")
		}

		username, ok := record.MessageAttributes["username"]
		if !ok {
			return nil, errors.New("missing username")
		}

		password, ok := record.MessageAttributes["password"]
		if !ok {
			return nil, errors.New("missing password")
		}

		c, err := client.New(&client.User{
			Username: *username.StringValue,
			Password: *password.StringValue,
		})
		if err != nil {
			return nil, err
		}

		switch *action.StringValue {
		case actionUpload:
			var ad Ad
			if err := json.Unmarshal([]byte(record.Body), &ad); err != nil {
				return nil, err
			}

			uploadedAdId, err := c.UploadAd(&client.Ad{
				Title:       ad.Title,
				Description: ad.Description,
				Price:       ad.Price,
				CategoryId:  ad.CategoryId,
			})
			if err != nil {
				return nil, err
			}
			if err := pdb.AddUploadedAd(ctx, ad.Id, uploadedAdId); err != nil {
				return nil, err
			}

			if err := deleteSQSMessage(svc, record.ReceiptHandle); err != nil {
				return nil, err
			}

		case actionRemove:
			uploadedAdId, err := strconv.ParseInt(record.Body, 10, 64)
			if err != nil {
				return nil, err
			}

			if err := c.RemoveAd(uploadedAdId); err != nil {
				return nil, err
			}

			if err := pdb.RemoveUploadedAd(ctx, uploadedAdId); err != nil {
				return nil, err
			}

			if err := deleteSQSMessage(svc, record.ReceiptHandle); err != nil {
				return nil, err
			}
		}
	}

	return event.Records, nil
}

func deleteSQSMessage(svc *sqs.SQS, receiptHandle string) error {
	_, err := svc.DeleteMessage(&sqs.DeleteMessageInput{
		QueueUrl:      aws.String(qURL),
		ReceiptHandle: &receiptHandle,
	})

	return err
}

func main() {
	lambda.Start(Handler)
}

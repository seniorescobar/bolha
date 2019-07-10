package main

import (
	"context"
	"encoding/json"
	"os"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"

	"github.com/seniorescobar/bolha/client"
	"github.com/seniorescobar/bolha/db/postgres"
)

const (
	allowedOrder = 5

	qURL = "https://sqs.eu-central-1.amazonaws.com/301808156345/bolha-ads-queue"

	actionUpload = "upload"
	actionRemove = "remove"
)

type Record struct {
	User *User `json:"user"`
	Ad   *Ad   `json:"ad"`
}

type User struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type Ad struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Price       int    `json:"price"`
	CategoryId  int    `json:"category-id"`
}

func GetActiveAds(ctx context.Context) error {
	sess, err := session.NewSession()
	if err != nil {
		return err
	}

	svc := sqs.New(sess)

	pdb, err := postgres.New(&postgres.Conf{
		Host:     os.Getenv("PGHOST"),
		Port:     os.Getenv("PGPORT"),
		User:     os.Getenv("PGUSER"),
		Password: os.Getenv("PGPASSWORD"),
		DBName:   os.Getenv("PGDATABASE"),
	})
	if err != nil {
		return err
	}
	defer pdb.Close()

	users, err := pdb.ListActiveUsers(ctx)
	if err != nil {
		return err
	}

	for _, user := range users {
		c, err := client.New(&client.User{
			Username: user.Username,
			Password: user.Password,
		})
		if err != nil {
			return err
		}

		activeAds, err := c.GetActiveAds()
		if err != nil {
			return err
		}

		for _, activeAd := range activeAds {
			if activeAd.Order > allowedOrder {
				record, err := pdb.GetRecord(ctx, activeAd.Id)
				if err != nil {
					return err
				}

				recordJSON, err := json.Marshal(&Record{
					User: &User{
						Username: record.Username,
						Password: record.Password,
					},
					Ad: &Ad{
						Title:       record.Title,
						Description: record.Description,
						Price:       record.Price,
						CategoryId:  record.CategoryId,
					},
				})
				if err != nil {
					return err
				}

				if _, err := svc.SendMessage(prepareMessage(actionUpload, string(recordJSON))); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func prepareMessage(action string, message string) *sqs.SendMessageInput {
	return &sqs.SendMessageInput{
		MessageAttributes: map[string]*sqs.MessageAttributeValue{
			"action": &sqs.MessageAttributeValue{
				DataType:    aws.String("String"),
				StringValue: aws.String(action),
			},
		},
		MessageBody: aws.String(message),
		QueueUrl:    aws.String(qURL),
	}
}

func main() {
	lambda.Start(GetActiveAds)
}

package main

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"

	"github.com/seniorescobar/bolha/client"
	"github.com/seniorescobar/bolha/db/postgres"

	log "github.com/sirupsen/logrus"
)

const (
	allowedOrder = 0

	qURL = "https://sqs.eu-central-1.amazonaws.com/301808156345/bolha-ads-queue"

	actionUpload = "upload"
	actionRemove = "remove"
)

type Record struct {
	User *User
	Ad   *Ad
}

type User struct {
	Username string
	Password string
}

type Ad struct {
	Id          int64    `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Price       int      `json:"price"`
	CategoryId  int      `json:"category-id"`
	Images      []string `json:"images"`
}

func GetActiveAds(ctx context.Context) ([]*client.ActiveAd, error) {
	sess, err := session.NewSession()
	if err != nil {
		return nil, err
	}

	svc := sqs.New(sess)

	pdb, err := postgres.NewFromEnv()
	if err != nil {
		return nil, err
	}
	defer pdb.Close()

	users, err := pdb.ListActiveUsers(ctx)
	if err != nil {
		return nil, err
	}

	outdatedAds := make([]*client.ActiveAd, 0)
	for _, user := range users {
		c, err := client.New(&client.User{
			Username: user.Username,
			Password: user.Password,
		})
		if err != nil {
			return nil, err
		}

		activeAds, err := c.GetActiveAds()
		if err != nil {
			return nil, err
		}

		for _, activeAd := range activeAds {
			if activeAd.Order > allowedOrder {
				outdatedAds = append(outdatedAds, activeAd)

				record, err := pdb.GetRecord(ctx, activeAd.Id)
				if err != nil {
					return nil, err
				}

				log.Printf("record %+v\n", record)

				if err := sendRemoveMessage(svc, &User{
					Username: record.User.Username,
					Password: record.User.Password,
				}, activeAd.Id); err != nil {
					return nil, err
				}

				if err := sendUploadMessage(svc, &Record{
					User: &User{
						Username: record.User.Username,
						Password: record.User.Password,
					},
					Ad: &Ad{
						Id:          record.Ad.Id,
						Title:       record.Ad.Title,
						Description: record.Ad.Description,
						Price:       record.Ad.Price,
						CategoryId:  record.Ad.CategoryId,
						Images:      record.Ad.Images,
					},
				}); err != nil {
					return nil, err
				}
			}
		}
	}

	return outdatedAds, nil
}

func sendUploadMessage(svc *sqs.SQS, record *Record) error {
	adJSON, err := json.Marshal(record.Ad)
	if err != nil {
		return err
	}

	log.Info("sending ad JSON", string(adJSON))

	_, err = svc.SendMessage(&sqs.SendMessageInput{
		MessageAttributes: map[string]*sqs.MessageAttributeValue{
			"action": &sqs.MessageAttributeValue{
				DataType:    aws.String("String"),
				StringValue: aws.String(actionUpload),
			},
			"username": &sqs.MessageAttributeValue{
				DataType:    aws.String("String"),
				StringValue: aws.String(record.User.Username),
			},
			"password": &sqs.MessageAttributeValue{
				DataType:    aws.String("String"),
				StringValue: aws.String(record.User.Password),
			},
		},
		MessageBody: aws.String(string(adJSON)),
		QueueUrl:    aws.String(qURL),
	})

	return err
}

func sendRemoveMessage(svc *sqs.SQS, user *User, uploadedAdId int64) error {
	_, err := svc.SendMessage(&sqs.SendMessageInput{
		MessageAttributes: map[string]*sqs.MessageAttributeValue{
			"action": &sqs.MessageAttributeValue{
				DataType:    aws.String("String"),
				StringValue: aws.String(actionRemove),
			},
			"username": &sqs.MessageAttributeValue{
				DataType:    aws.String("String"),
				StringValue: aws.String(user.Username),
			},
			"password": &sqs.MessageAttributeValue{
				DataType:    aws.String("String"),
				StringValue: aws.String(user.Password),
			},
		},
		MessageBody: aws.String(strconv.FormatInt(uploadedAdId, 10)),
		QueueUrl:    aws.String(qURL),
	})

	return err
}

func main() {
	lambda.Start(GetActiveAds)
}

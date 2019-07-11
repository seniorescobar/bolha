package common

import (
	"encoding/json"
	"strconv"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/sqs"

	log "github.com/sirupsen/logrus"
)

const (
	qURL = "https://sqs.eu-central-1.amazonaws.com/301808156345/bolha-ads-queue"

	ActionUpload = "upload"
	ActionRemove = "remove"
)

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

func SendUploadMessage(svc *sqs.SQS, user *User, ad *Ad) error {
	log.Info("sending upload message")

	adJSON, err := json.Marshal(ad)
	if err != nil {
		return err
	}

	_, err = svc.SendMessage(&sqs.SendMessageInput{
		MessageAttributes: map[string]*sqs.MessageAttributeValue{
			"action": &sqs.MessageAttributeValue{
				DataType:    aws.String("String"),
				StringValue: aws.String(ActionUpload),
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
		MessageBody: aws.String(string(adJSON)),
		QueueUrl:    aws.String(qURL),
	})

	log.WithFields(log.Fields{
		"username": user.Username,
		"password": user.Password,
		"body":     string(adJSON),
	}).Info("upload message sent")

	return err
}

func SendRemoveMessage(svc *sqs.SQS, user *User, uploadedAdId int64) error {
	log.Info("sending remove message")

	uploadedAtIdString := strconv.FormatInt(uploadedAdId, 10)
	_, err := svc.SendMessage(&sqs.SendMessageInput{
		MessageAttributes: map[string]*sqs.MessageAttributeValue{
			"action": &sqs.MessageAttributeValue{
				DataType:    aws.String("String"),
				StringValue: aws.String(ActionRemove),
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
		MessageBody: aws.String(uploadedAtIdString),
		QueueUrl:    aws.String(qURL),
	})

	log.WithFields(log.Fields{
		"username": user.Username,
		"password": user.Password,
		"body":     uploadedAtIdString,
	}).Info("remove message sent")

	return err
}

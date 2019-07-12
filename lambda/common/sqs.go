package common

import (
	"encoding/json"
	"strconv"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"

	log "github.com/sirupsen/logrus"
)

const (
	qName = "bolha-ads-queue"
)

type SQSClient struct {
	svc  *sqs.SQS
	qURL string
}

func NewSQSClient(sess *session.Session) (*SQSClient, error) {
	svc := sqs.New(sess)

	// qURLRes, err := svc.GetQueueUrl(&sqs.GetQueueUrlInput{
	// 	QueueName: aws.String(qName),
	// })
	// if err != nil {
	// 	return nil, err
	// }

	return &SQSClient{
		svc: svc,
		// qURL: *qURLRes.QueueUrl,
		qURL: "https://sqs.eu-central-1.amazonaws.com/301808156345/bolha-ads-queue",
	}, nil
}

func (sqsClient *SQSClient) SendUploadMessage(user *User, ad *Ad) error {
	log.Info("sending upload message")

	adJSON, err := json.Marshal(ad)
	if err != nil {
		return err
	}

	_, err = sqsClient.svc.SendMessage(&sqs.SendMessageInput{
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
		QueueUrl:    aws.String(sqsClient.qURL),
	})

	log.WithFields(log.Fields{
		"username": user.Username,
		"password": user.Password,
		"body":     string(adJSON),
	}).Info("upload message sent")

	return err
}

func (sqsClient *SQSClient) SendRemoveMessage(user *User, uploadedAdId int64) error {
	log.Info("sending remove message")

	uploadedAtIdString := strconv.FormatInt(uploadedAdId, 10)
	_, err := sqsClient.svc.SendMessage(&sqs.SendMessageInput{
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
		QueueUrl:    aws.String(sqsClient.qURL),
	})

	log.WithFields(log.Fields{
		"username": user.Username,
		"password": user.Password,
		"body":     uploadedAtIdString,
	}).Info("remove message sent")

	return err
}

func (sqsClient *SQSClient) DeleteMessage(receiptHandle string) error {
	log.WithField("receiptHandle", receiptHandle).Info("deleting message from sqs queue")

	_, err := sqsClient.svc.DeleteMessage(&sqs.DeleteMessageInput{
		QueueUrl:      aws.String(sqsClient.qURL),
		ReceiptHandle: aws.String(receiptHandle),
	})

	return err
}

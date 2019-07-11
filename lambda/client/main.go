package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"strconv"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/seniorescobar/bolha/client"
	"github.com/seniorescobar/bolha/db/postgres"

	log "github.com/sirupsen/logrus"

	_ "image/jpeg"
)

const (
	qURL = "https://sqs.eu-central-1.amazonaws.com/301808156345/bolha-ads-queue"

	actionUpload = "upload"
	actionRemove = "remove"

	s3ImagesBucket = "bolha-images"
)

type Ad struct {
	Id          int64    `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Price       int      `json:"price"`
	CategoryId  int      `json:"category-id"`
	Images      []string `json:"images"`
}

func Handler(ctx context.Context, event events.SQSEvent) error {
	pdb, err := postgres.NewFromEnv()
	if err != nil {
		return err
	}
	defer pdb.Close()

	sess, err := session.NewSession()
	if err != nil {
		return err
	}

	sqsClient := sqs.New(sess)

	for _, record := range event.Records {
		log.Println("record", record)

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

			s3downloader := s3manager.NewDownloader(sess)

			log.Println("downloading images from s3 (len=%d)", len(ad.Images))
			images := make([]*image.Image, len(ad.Images))
			for i, imgStr := range ad.Images {
				buff := new(aws.WriteAtBuffer)

				n, err := s3downloader.Download(buff, &s3.GetObjectInput{
					Bucket: aws.String(s3ImagesBucket),
					Key:    aws.String(imgStr),
				})
				if err != nil {
					return err
				}

				if n == 0 {
					return errors.New("n == 0")
				}

				imgBytes := buff.Bytes()
				log.Println(string(imgBytes))

				img, _, err := image.Decode(bytes.NewReader(imgBytes))
				if err != nil {
					return err
				}

				images[i] = &img
			}

			uploadedAdId, err := c.UploadAd(&client.Ad{
				Title:       ad.Title,
				Description: ad.Description,
				Price:       ad.Price,
				CategoryId:  ad.CategoryId,
				Images:      images,
			})
			if err != nil {
				return err
			}

			if err := pdb.AddUploadedAd(ctx, ad.Id, uploadedAdId); err != nil {
				return err
			}

			if err := deleteSQSMessage(sqsClient, record.ReceiptHandle); err != nil {
				return err
			}

		case actionRemove:
			uploadedAdId, err := strconv.ParseInt(record.Body, 10, 64)
			if err != nil {
				return err
			}

			if err := c.RemoveAd(uploadedAdId); err != nil {
				return err
			}

			if err := pdb.RemoveUploadedAd(ctx, uploadedAdId); err != nil {
				return err
			}

			if err := deleteSQSMessage(sqsClient, record.ReceiptHandle); err != nil {
				return err
			}
		}
	}

	return nil
}

func deleteSQSMessage(sqsClient *sqs.SQS, receiptHandle string) error {
	_, err := sqsClient.DeleteMessage(&sqs.DeleteMessageInput{
		QueueUrl:      aws.String(qURL),
		ReceiptHandle: &receiptHandle,
	})

	return err
}

func main() {
	lambda.Start(Handler)
}

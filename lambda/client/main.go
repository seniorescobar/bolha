package main

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"strconv"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws/session"

	"github.com/seniorescobar/bolha/client"
	"github.com/seniorescobar/bolha/db/postgres"
	"github.com/seniorescobar/bolha/lambda/common"

	_ "image/jpeg"
)

func Handler(ctx context.Context, event events.SQSEvent) error {
	pdb, err := postgres.NewFromEnv()
	if err != nil {
		return err
	}
	defer pdb.Close()

	sess := session.Must(session.NewSession())

	sqsClient, err := common.NewSQSClient(sess)
	if err != nil {
		return err
	}

	for _, record := range event.Records {
		var action, username, password string
		getMessageAttributes(record.MessageAttributes, map[string]*string{
			"action":   &action,
			"username": &username,
			"password": &password,
		})

		c, err := client.New(&client.User{
			Username: username,
			Password: password,
		})
		if err != nil {
			return err
		}

		switch action {
		case common.ActionUpload:
			var ad common.Ad
			if err := json.Unmarshal([]byte(record.Body), &ad); err != nil {
				return err
			}

			s3Client := common.NewS3Client(sess)

			images := make([]*image.Image, len(ad.Images))
			for i, imgPath := range ad.Images {
				img, err := s3Client.DownloadImage(imgPath)
				if err != nil {
					return err
				}

				images[i] = img
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

			if err := sqsClient.DeleteMessage(record.ReceiptHandle); err != nil {
				return err
			}

		case common.ActionRemove:
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

			if err := sqsClient.DeleteMessage(record.ReceiptHandle); err != nil {
				return err
			}
		}
	}

	return nil
}

func getMessageAttributes(msga map[string]events.SQSMessageAttribute, pairs map[string]*string) error {
	for key, val := range pairs {
		m, ok := msga[key]
		if !ok {
			return fmt.Errorf(`missing message attributes "%s"`, key)
		}

		*val = *m.StringValue
	}

	return nil
}

func main() {
	lambda.Start(Handler)
}

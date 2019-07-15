package main

import (
	"context"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws/session"

	"github.com/seniorescobar/bolha/client"
	"github.com/seniorescobar/bolha/db/postgres"
	"github.com/seniorescobar/bolha/lambda/common"
)

const (
	allowedOrder = 3
)

type ActiveAd struct {
	Id         int64
	Order      int
	UploadedAt time.Time
}

func Handler(ctx context.Context) error {
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
			// get uploaded at
			uploadedAt, err := pdb.GetUploadedAt(ctx, activeAd.Id)
			if err != nil {
				return err
			}

			// check re-upload condition
			if reUploadCondition(&ActiveAd{Id: activeAd.Id, Order: activeAd.Order, UploadedAt: uploadedAt}) {
				record, err := pdb.GetRecord(ctx, activeAd.Id)
				if err != nil {
					return err
				}

				// send remove message
				if err := sqsClient.SendRemoveMessage(
					&common.User{
						Username: record.User.Username,
						Password: record.User.Password,
					},
					activeAd.Id,
				); err != nil {
					return err
				}

				// send upload message
				if err := sqsClient.SendUploadMessage(
					&common.User{
						Username: record.User.Username,
						Password: record.User.Password,
					},
					&common.Ad{
						Id:          record.Ad.Id,
						Title:       record.Ad.Title,
						Description: record.Ad.Description,
						Price:       record.Ad.Price,
						CategoryId:  record.Ad.CategoryId,
						Images:      record.Ad.Images,
					},
				); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func reUploadCondition(activeAd *ActiveAd) bool {
	if activeAd.Order > allowedOrder {
		return true
	}

	if time.Since(activeAd.UploadedAt) > 24*time.Hour {
		return true
	}

	return false
}

func main() {
	lambda.Start(Handler)
}

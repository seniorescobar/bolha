package main

import (
	"context"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/seniorescobar/bolha/client"
	"github.com/seniorescobar/bolha/lambda/common"

	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
	"github.com/aws/aws-sdk-go/service/dynamodb/expression"
)

type BolhaItem struct {
	AdTitle       string
	AdDescription string
	AdPrice       int
	AdCategoryId  int
	AdImages      []string

	AdUploadedId int64
	AdUploadedAt string

	UserUsername  string
	UserPassword  string
	UserSessionId string

	ReuploadHours int
	ReuploadOrder int
}

func Handler(ctx context.Context) error {
	sess := session.Must(session.NewSession())

	sqsClient, err := common.NewSQSClient(sess)
	if err != nil {
		return err
	}

	// TODO move to db/dynamodb client package
	ddb := dynamodb.New(sess)

	filt := expression.Name("AdUploadedId").GreaterThan(expression.Value(0))
	expr, err := expression.NewBuilder().WithFilter(filt).Build()
	if err != nil {
		return err
	}

	result, err := ddb.Scan(&dynamodb.ScanInput{
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		FilterExpression:          expr.Filter(),
		TableName:                 aws.String("Bolha"),
	})
	if err != nil {
		return err
	}

	bItems := make([]BolhaItem, 0)
	if err := dynamodbattribute.UnmarshalListOfMaps(result.Items, &bItems); err != nil {
		return err
	}

	for _, bItem := range bItems {
		// create new client
		var (
			c   *client.Client
			err error
		)
		if bItem.UserSessionId != "" {
			c, err = client.NewWithSessionId(bItem.UserSessionId)
		} else {
			c, err = client.New(&client.User{bItem.UserUsername, bItem.UserPassword})
		}
		if err != nil {
			return err
		}

		// get active (uploaded) ad
		activeAd, err := c.GetActiveAd(bItem.AdUploadedId)
		if err != nil {
			return err
		}

		// parse ad uploaded at
		adUploadedAt, err := time.Parse(time.RFC3339, bItem.AdUploadedAt)
		if err != nil {
			return err
		}

		// check re-upload condition
		if checkOrder(activeAd.Order, bItem.ReuploadOrder) || checkTimeDiff(adUploadedAt, bItem.ReuploadHours) {
			// send remove message
			if err := sqsClient.SendRemoveMessage(
				&common.User{
					Username: bItem.UserUsername,
					Password: bItem.UserPassword,
				},
				bItem.AdUploadedId,
			); err != nil {
				return err
			}

			// send upload message
			if err := sqsClient.SendUploadMessage(
				&common.User{
					Username: bItem.UserUsername,
					Password: bItem.UserPassword,
				},
				&common.Ad{
					Title:       bItem.AdTitle,
					Description: bItem.AdDescription,
					Price:       bItem.AdPrice,
					CategoryId:  bItem.AdCategoryId,
					Images:      bItem.AdImages,
				},
			); err != nil {
				return err
			}
		}
	}

	return nil
}

func checkOrder(currOrder, allowedOrder int) bool {
	return currOrder > allowedOrder
}

func checkTimeDiff(currUploadedAt time.Time, allowedHours int) bool {
	return time.Since(currUploadedAt) > time.Duration(allowedHours)*time.Hour
}

func main() {
	lambda.Start(Handler)
}

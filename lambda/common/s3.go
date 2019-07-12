package common

import (
	"bytes"
	"image"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"

	log "github.com/sirupsen/logrus"
)

const (
	s3ImagesBucket = "bolha-images"
)

type S3Client struct {
	s3downloader *s3manager.Downloader
}

func NewS3Client(sess *session.Session) *S3Client {
	s3downloader := s3manager.NewDownloader(sess)

	return &S3Client{
		s3downloader: s3downloader,
	}
}

func (s3client *S3Client) DownloadImage(imgPath string) (*image.Image, error) {
	log.WithField("imgPath", imgPath).Info("downloading image from s3")

	buff := new(aws.WriteAtBuffer)

	_, err := s3client.s3downloader.Download(buff, &s3.GetObjectInput{
		Bucket: aws.String(s3ImagesBucket),
		Key:    aws.String(imgPath),
	})
	if err != nil {
		return nil, err
	}

	imgBytes := buff.Bytes()

	img, _, err := image.Decode(bytes.NewReader(imgBytes))

	return &img, err
}

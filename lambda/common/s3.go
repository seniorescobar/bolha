package common

import (
	"bytes"
	"io"

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
	downloader *s3manager.Downloader
	uploader   *s3manager.Uploader
}

func NewS3Client(sess *session.Session) *S3Client {
	downloader := s3manager.NewDownloader(sess)
	uploader := s3manager.NewUploader(sess)

	return &S3Client{
		downloader: downloader,
		uploader:   uploader,
	}
}

func (s3Client *S3Client) DownloadImage(imgKey string) (io.Reader, error) {
	log.WithField("imgKey", imgKey).Info("downloading image from s3")

	buff := new(aws.WriteAtBuffer)

	_, err := s3Client.downloader.Download(buff, &s3.GetObjectInput{
		Bucket: aws.String(s3ImagesBucket),
		Key:    aws.String(imgKey),
	})
	if err != nil {
		return nil, err
	}

	imgBytes := buff.Bytes()

	return bytes.NewReader(imgBytes), nil
}

func (s3Client *S3Client) UploadImage(imgKey string, img io.Reader) error {
	log.WithField("imgKey", imgKey).Info("uploading image to s3")

	if _, err := s3Client.uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(s3ImagesBucket),
		Key:    aws.String(imgKey),
		Body:   img,
	}); err != nil {
		return err
	}

	return nil
}

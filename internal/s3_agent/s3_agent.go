package s3_agent

import (
	"fmt"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

// S3Agent wraps the s3.S3 structure to allow for wrapper methods
type S3Agent struct {
	Client *s3.S3
}

func NewS3Agent(accessKey, secretKey, endpoint string, debug bool) (*S3Agent, error) {
	// TODO: cephRegion must not be hardcoded.
	const cephRegion = "us-east-1"

	logLevel := aws.LogOff
	if debug {
		logLevel = aws.LogDebug
	}
	client := http.Client{
		Timeout: time.Second * 15,
	}
	sess, err := session.NewSession(
		aws.NewConfig().
			WithRegion(cephRegion).
			WithCredentials(credentials.NewStaticCredentials(accessKey, secretKey, "")).
			WithEndpoint(endpoint).
			WithS3ForcePathStyle(true).
			WithMaxRetries(5).
			WithDisableSSL(true).
			WithHTTPClient(&client).
			WithLogLevel(logLevel),
	)
	if err != nil {
		return nil, err
	}
	svc := s3.New(sess)
	return &S3Agent{
		Client: svc,
	}, nil
}

func (s *S3Agent) CreateBucket(name string) error {
	bucketInput := &s3.CreateBucketInput{
		Bucket: &name,
	}
	_, err := s.Client.CreateBucket(bucketInput)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case s3.ErrCodeBucketAlreadyExists:
				return nil
			case s3.ErrCodeBucketAlreadyOwnedByYou:
				return nil
			}
		}
		return fmt.Errorf("failed to create bucket %q. %w", name, err)
	}
	return nil
}

func (s *S3Agent) DeleteBucket(name string) error {
	bucketInput := &s3.DeleteBucketInput{
		Bucket: &name,
	}
	_, err := s.Client.DeleteBucket(bucketInput)
	return err
}

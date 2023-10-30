package s3_agent

import (
	"encoding/json"
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
	Client             *s3.S3
	BucketAccessAction map[string][]string
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

func (s *S3Agent) SetBucketPolicy(subUserAccessMap map[string]string, tenant string,
	owner string, bucket string) error {
	// The map of access levels to the AWS IAM names slice
	accessAWSIAMMap := make(map[string][]string)
	policy := map[string]interface{}{
		"Version": "2012-10-17",
		"Id":      "S3Policy",
	}
	statementSlice := []map[string]interface{}{}
	for subUser, access := range subUserAccessMap {
		// Create AWS IAM Name needed for the policy from the subuser name
		aws_iam := fmt.Sprintf("arn:aws:iam::%s:user/%s:%s", tenant, owner, subUser)
		accessAWSIAMMap[access] = append(accessAWSIAMMap[access], aws_iam)
	}

	// Iterate over different levels
	for access, AWS_iam := range accessAWSIAMMap {
		principal := map[string][]string{}
		statement := map[string]interface{}{
			"Sid":       "BucketAllow",
			"Effect":    "Allow",
			"Principal": map[string][]string{},
			"Action":    []string{},
			"Resource": []string{
				fmt.Sprintf("arn:aws:s3::%s:%s", tenant, bucket),
				fmt.Sprintf("arn:aws:s3::%s:%s/*", tenant, bucket),
			},
		}
		principal["AWS"] = AWS_iam
		statement["Principal"] = principal

		if actions, exists := s.BucketAccessAction[access]; exists {
			statement["Action"] = actions
		} else {
			return fmt.Errorf("the access %s doesn't exists", access)
		}
		// Append the statement
		statementSlice = append(statementSlice, statement)
	}
	policy["Statement"] = statementSlice
	policyMarshal, err := json.Marshal(policy)

	policyInput := s3.PutBucketPolicyInput{Bucket: aws.String(bucket),
		Policy: aws.String(string(policyMarshal))}
	if err != nil {
		return err
	}
	_, err = s.Client.PutBucketPolicy(&policyInput)
	if err != nil {
		return err
	}
	return nil
}

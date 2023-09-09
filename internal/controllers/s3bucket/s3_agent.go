package s3bucket

import (
	"context"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	s3v1alpha1 "github.com/snapp-incubator/s3-operator/api/v1alpha1"
	"github.com/snapp-incubator/s3-operator/pkg/consts"
)

// S3Agent wraps the s3.S3 structure to allow for wrapper methods
type S3Agent struct {
	Client *s3.S3
}

func newS3Agent(accessKey, secretKey, endpoint string, debug bool) (*S3Agent, error) {
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

func (s *S3Agent) createBucket(name string) error {
	bucketInput := &s3.CreateBucketInput{
		Bucket: &name,
	}
	_, err := s.Client.CreateBucket(bucketInput)
	return err
}

func (s *S3Agent) deleteBucket(name string) error {
	bucketInput := &s3.DeleteBucketInput{
		Bucket: &name,
	}
	_, err := s.Client.DeleteBucket(bucketInput)
	return err
}

func (r *Reconciler) setS3Agent(ctx context.Context, req ctrl.Request) error {
	// Set the s3Agent regarding the secret of the s3UserClaim
	s3userclaim := &s3v1alpha1.S3UserClaim{}
	s3userClaimNamespacedName := types.NamespacedName{Namespace: req.Namespace, Name: r.s3UserRef}
	err := r.Get(ctx, s3userClaimNamespacedName, s3userclaim)
	if err != nil {
		return err
	}

	userAdminSecret := &corev1.Secret{}
	secretNamespacedName := types.NamespacedName{Namespace: req.NamespacedName.Namespace, Name: s3userclaim.Spec.AdminSecret}
	err = r.Get(ctx, secretNamespacedName, userAdminSecret)
	if err != nil {
		return err
	}

	accessKey := string(userAdminSecret.Data[consts.DataKeyAccessKey])
	secretKey := string(userAdminSecret.Data[consts.DataKeySecretKey])
	r.s3Agent, err = newS3Agent(accessKey, secretKey, r.rgwEndpoint, true)
	if err != nil {
		return err
	}
	return nil
}

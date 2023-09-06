/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package s3bucket

import (
	"context"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/go-logr/logr"
	"github.com/opdev/subreconciler"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	s3v1alpha1 "github.com/snapp-incubator/s3-operator/api/v1alpha1"
	"github.com/snapp-incubator/s3-operator/internal/config"
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

// S3BucketReconciler reconciles a S3Bucket object
type Reconciler struct {
	client.Client
	scheme  *runtime.Scheme
	logger  logr.Logger
	s3Agent *S3Agent
	// reconcile specific variables
	s3Bucket     *s3v1alpha1.S3Bucket
	s3UserRef    string
	s3BucketName string
	rgwEndpoint  string
}

func NewReconciler(mgr manager.Manager, cfg *config.Config) *Reconciler {

	return &Reconciler{
		Client:      mgr.GetClient(),
		scheme:      mgr.GetScheme(),
		rgwEndpoint: cfg.Rgw.Endpoint,
	}
}

//+kubebuilder:rbac:groups=s3.snappcloud.io,resources=s3buckets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=s3.snappcloud.io,resources=s3buckets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=s3.snappcloud.io,resources=s3buckets/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the S3Bucket object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.logger = log.FromContext(ctx)
	r.s3Bucket = &s3v1alpha1.S3Bucket{}
	r.s3BucketName = req.Name
	switch err := r.Get(ctx, req.NamespacedName, r.s3Bucket); {
	case apierrors.IsNotFound(err):
		return r.Cleanup(ctx)
	case err != nil:
		r.logger.Error(err, "failed to fetch object")
		return subreconciler.Evaluate(subreconciler.Requeue())
	default:
		r.s3UserRef = r.s3Bucket.Spec.S3UserRef
		err = r.setS3Agent(ctx, req)
		if err != nil {
			r.logger.Error(err, "Failed to login on S3 with the user credentials")
			return subreconciler.Evaluate(subreconciler.Requeue())
		}
		if r.s3Bucket.ObjectMeta.DeletionTimestamp != nil {
			return r.Cleanup(ctx)
		}
	}

	return r.Provision(ctx)
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&s3v1alpha1.S3Bucket{}).
		Complete(r)
}

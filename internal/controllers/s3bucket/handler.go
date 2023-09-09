package s3bucket

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/opdev/subreconciler"
	s3v1alpha1 "github.com/snapp-incubator/s3-operator/api/v1alpha1"
	"github.com/snapp-incubator/s3-operator/internal/config"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

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

	// Get s3Bucket object
	switch err := r.Get(ctx, req.NamespacedName, r.s3Bucket); {
	case apierrors.IsNotFound(err):
		r.logger.Info("CR not found.")
		return ctrl.Result{}, nil
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

	if r.s3Bucket.Status.Ready != true {
		return r.Provision(ctx)
	}
	return ctrl.Result{}, nil
}

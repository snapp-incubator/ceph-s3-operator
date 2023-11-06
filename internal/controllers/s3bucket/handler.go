package s3bucket

import (
	"context"
	"fmt"
	"regexp"

	"github.com/go-logr/logr"
	"github.com/opdev/subreconciler"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	corev1 "k8s.io/api/core/v1"

	s3v1alpha1 "github.com/snapp-incubator/s3-operator/api/v1alpha1"
	"github.com/snapp-incubator/s3-operator/internal/config"
	"github.com/snapp-incubator/s3-operator/internal/s3_agent"
	"github.com/snapp-incubator/s3-operator/pkg/consts"
)

// S3BucketReconciler reconciles a S3Bucket object
type Reconciler struct {
	client.Client
	scheme  *runtime.Scheme
	logger  logr.Logger
	s3Agent *s3_agent.S3Agent
	// reconcile specific variables
	s3Bucket         *s3v1alpha1.S3Bucket
	s3UserRef        string
	s3BucketName     string
	rgwEndpoint      string
	clusterName      string
	cephTenant       string
	cephUserFullId   string
	subuserAccessMap map[string]string
	bucketPolicy     string
}

func NewReconciler(mgr manager.Manager, cfg *config.Config) *Reconciler {

	return &Reconciler{
		Client:      mgr.GetClient(),
		scheme:      mgr.GetScheme(),
		rgwEndpoint: cfg.Rgw.Endpoint,
		clusterName: cfg.ClusterName,
	}
}

//+kubebuilder:rbac:groups=s3.snappcloud.io,resources=s3buckets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=s3.snappcloud.io,resources=s3buckets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=s3.snappcloud.io,resources=s3buckets/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	r.logger = log.FromContext(ctx)
	r.s3Bucket = &s3v1alpha1.S3Bucket{}
	r.s3BucketName = req.Name

	// Get s3Bucket object
	switch err := r.Get(ctx, req.NamespacedName, r.s3Bucket); {
	case apierrors.IsNotFound(err):
		r.logger.Info(fmt.Sprintf("S3Bucket %s in namespace %s not found!", req.Name, req.Namespace))
		return ctrl.Result{}, nil
	case err != nil:
		r.logger.Error(err, "failed to fetch object")
		return subreconciler.Evaluate(subreconciler.Requeue())
	default:
		r.s3UserRef = r.s3Bucket.Spec.S3UserRef
		// Create a s3 session with the s3user credentials.
		err = r.setS3Agent(ctx, req)
		if err != nil {
			r.logger.Error(err, "Failed to login on S3 with the user credentials")
			return subreconciler.Evaluate(subreconciler.Requeue())
		}
		// Initialize ceph tenant and cephFullUserId variables
		r.initVars(req)
		// Delete event
		if r.s3Bucket.ObjectMeta.DeletionTimestamp != nil {
			return r.Cleanup(ctx)
		}
	}

	return r.Provision(ctx)
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
	r.s3Agent, err = s3_agent.NewS3Agent(accessKey, secretKey, r.rgwEndpoint, true)
	if err != nil {
		return err
	}

	return nil
}

func (r *Reconciler) initVars(req ctrl.Request) {
	// TODO: This function is mutual with the s3userclaim handler. It should be moved to a higher layer.
	// Only alphanumeric characters and underscore are allowed for tenant name
	k8sNameSpecialChars := regexp.MustCompile(`[.-]`)
	namespace := k8sNameSpecialChars.ReplaceAllString(req.Namespace, "_")
	clusterName := k8sNameSpecialChars.ReplaceAllString(r.clusterName, "_")
	r.cephTenant = fmt.Sprintf("%s__%s", clusterName, namespace)
	r.cephUserFullId = fmt.Sprintf("%s$%s", r.cephTenant, r.s3UserRef)

	r.subuserAccessMap = make(map[string]string)
	for _, binding := range r.s3Bucket.Spec.S3SubuserBinding {
		r.subuserAccessMap[binding.Name] = binding.Access
	}

}

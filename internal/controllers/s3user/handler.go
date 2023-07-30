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

package s3user

import (
	"context"
	goerrors "errors"

	"github.com/ceph/go-ceph/rgw/admin"
	"github.com/go-logr/logr"
	"github.com/opdev/subreconciler"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	s3v1alpha1 "github.com/snapp-incubator/s3-operator/api/v1alpha1"
	"github.com/snapp-incubator/s3-operator/internal/config"
	"github.com/snapp-incubator/s3-operator/internal/controllers/common"
)

// Reconciler reconciles a S3User object
type Reconciler struct {
	client.Client
	scheme    *runtime.Scheme
	logger    logr.Logger
	rgwClient *admin.API

	// reconcile specific variables
	s3User               *s3v1alpha1.S3User
	s3UserClaim          *s3v1alpha1.S3UserClaim
	s3UserClaimNamespace string
	s3UserClaimName      string

	// configurations
	s3UserClass string
	clusterName string
}

func NewReconciler(mgr manager.Manager, cfg *config.Config, rgwClient *admin.API) *Reconciler {
	return &Reconciler{
		Client:      mgr.GetClient(),
		scheme:      mgr.GetScheme(),
		rgwClient:   rgwClient,
		s3UserClass: cfg.S3UserClass,
		clusterName: cfg.ClusterName,
	}
}

//+kubebuilder:rbac:groups=s3.snappcloud.io,resources=s3users,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=s3.snappcloud.io,resources=s3users/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=s3.snappcloud.io,resources=s3users/finalizers,verbs=update

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.logger = log.FromContext(ctx)
	r.s3User = &s3v1alpha1.S3User{}

	// Fetch the object
	switch err := r.Get(ctx, req.NamespacedName, r.s3User); {
	case apierrors.IsNotFound(err):
		return subreconciler.Evaluate(subreconciler.DoNotRequeue())
	case err != nil:
		r.logger.Error(err, "failed to fetch object")
		return subreconciler.Evaluate(subreconciler.Requeue())
	}

	// Ignore object with deletionTimestamp set
	if r.s3User.ObjectMeta.DeletionTimestamp != nil {
		return subreconciler.Evaluate(subreconciler.DoNotRequeue())
	}

	// Do the actual reconcile work
	subrecs := []subreconciler.Fn{
		r.initVars,
		r.findS3UserClaim,
		r.skipIfS3UserClaimExists,
		r.removeCephUser,
		r.removeS3User,
	}
	for _, subrec := range subrecs {
		result, err := subrec(ctx)
		if subreconciler.ShouldHaltOrRequeue(result, err) {
			return subreconciler.Evaluate(result, err)
		}
	}

	return subreconciler.Evaluate(subreconciler.DoNotRequeue())
}

func (r *Reconciler) initVars(context.Context) (*ctrl.Result, error) {
	claimRef := r.s3User.Spec.ClaimRef
	r.s3UserClaimName = claimRef.Name
	r.s3UserClaimNamespace = claimRef.Namespace

	return subreconciler.ContinueReconciling()
}

func (r *Reconciler) findS3UserClaim(ctx context.Context) (*ctrl.Result, error) {
	s3UserClaim := &s3v1alpha1.S3UserClaim{}
	err := r.Get(ctx, types.NamespacedName{Namespace: r.s3UserClaimNamespace, Name: r.s3UserClaimName}, s3UserClaim)
	if err != nil {
		if apierrors.IsNotFound(err) {
			r.s3UserClaim = nil
			return subreconciler.ContinueReconciling()
		}
		r.logger.Error(err, "failed to get s3UserClaim")
		return subreconciler.Requeue()
	}

	r.s3UserClaim = s3UserClaim
	return subreconciler.ContinueReconciling()
}

// skipIfS3UserClaimExists will return a DoNotRequeue response if the S3UserClaim is not deleted, so that
// the reconciliation process be stoppped
func (r *Reconciler) skipIfS3UserClaimExists(context.Context) (*ctrl.Result, error) {
	if r.s3UserClaim != nil {
		return subreconciler.DoNotRequeue()
	}
	return subreconciler.ContinueReconciling()
}

func (r *Reconciler) removeCephUser(ctx context.Context) (*ctrl.Result, error) {
	claimRef := r.s3User.Spec.ClaimRef
	switch err := r.rgwClient.RemoveUser(ctx, admin.User{
		ID: common.GetCephUserFullId(r.clusterName, claimRef.Namespace, claimRef.Name),
	}); {
	case goerrors.Is(err, admin.ErrNoSuchUser):
		return subreconciler.ContinueReconciling()
	case err != nil:
		r.logger.Error(err, "failed to remove Ceph user")
		return subreconciler.Requeue()
	default:
		return subreconciler.ContinueReconciling()
	}
}

func (r *Reconciler) removeS3User(ctx context.Context) (*ctrl.Result, error) {
	switch err := r.Delete(ctx, r.s3User); {
	case apierrors.IsNotFound(err):
		return subreconciler.ContinueReconciling()
	case err != nil:
		r.logger.Error(err, "failed to remove S3User")
		return subreconciler.Requeue()
	default:
		return subreconciler.ContinueReconciling()
	}
}

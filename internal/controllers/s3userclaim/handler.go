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

package s3userclaim

import (
	"context"
	"fmt"

	"github.com/ceph/go-ceph/rgw/admin"
	"github.com/go-logr/logr"
	"github.com/opdev/subreconciler"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	s3v1alpha1 "github.com/snapp-incubator/s3-operator/api/v1alpha1"
	"github.com/snapp-incubator/s3-operator/internal/config"
)

type Reconciler struct {
	client.Client
	scheme *runtime.Scheme

	logger       logr.Logger
	s3UserClaim  *s3v1alpha1.S3UserClaim
	s3UserName   string
	cephUserName string
	rgwAccessKey string
	rgwSecretKey string
	rgwEndpoint  string
}

func NewReconciler(mgr manager.Manager, cfg *config.Config) *Reconciler {
	return &Reconciler{
		Client:       mgr.GetClient(),
		scheme:       mgr.GetScheme(),
		rgwAccessKey: cfg.Rgw.AccessKey,
		rgwSecretKey: cfg.Rgw.SecretKey,
		rgwEndpoint:  cfg.Rgw.Endpoint,
	}
}

//+kubebuilder:rbac:groups=s3.snappcloud.io,resources=s3userclaims,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=s3.snappcloud.io,resources=s3userclaims/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=s3.snappcloud.io,resources=s3userclaims/finalizers,verbs=update

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.logger = log.FromContext(ctx)

	// Handle object fetch error
	switch err := r.Get(ctx, req.NamespacedName, r.s3UserClaim); {
	case errors.IsNotFound(err):
		return subreconciler.Evaluate(subreconciler.DoNotRequeue())
	case err != nil:
		r.logger.Error(err, "failed to fetch object")
		return subreconciler.Evaluate(subreconciler.Requeue())
	}

	// Ignore object with deletionTimestamp set
	if r.s3UserClaim.ObjectMeta.DeletionTimestamp != nil {
		return subreconciler.Evaluate(subreconciler.DoNotRequeue())
	}

	// Do the actual reconcile work
	subrecs := []subreconciler.Fn{
		r.initVars,
		r.ensureCephUser,
		r.ensureS3User,
	}

	for _, subrec := range subrecs {
		result, err := subrec(ctx)
		if subreconciler.ShouldHaltOrRequeue(result, err) {
			return subreconciler.Evaluate(result, err)
		}
	}

	return subreconciler.Evaluate(subreconciler.DoNotRequeue())
}

func (r *Reconciler) initVars(ctx context.Context) (*ctrl.Result, error) {
	r.s3UserName = fmt.Sprint("")
	r.cephUserName = fmt.Sprint("")

	return subreconciler.ContinueReconciling()
}

func (r *Reconciler) ensureCephUser(ctx context.Context) (*ctrl.Result, error) {
	co, err := admin.New(r.rgwEndpoint, r.rgwAccessKey, r.rgwSecretKey, nil)
	if err != nil {
		r.logger.Error(err, "failed to create rgw connection")
		return subreconciler.Requeue()
	}

	// Create user
	user, err := co.CreateUser(context.Background(), admin.User{
		Tenant:      "felan",
		ID:          "testuser",
		DisplayName: "Test User",
	})
	if err != nil {
		r.logger.Error(err, "failed to create ceph user")
		return subreconciler.Requeue()
	}

	// Create subuser
	if err := co.CreateSubuser(context.Background(), user, admin.SubuserSpec{
		Name:   "testsubuser",
		Access: admin.SubuserAccessRead,
	}); err != nil {
		r.logger.Error(err, "failed to create ceph sub-user")
		return subreconciler.Requeue()
	}

	return subreconciler.ContinueReconciling()
}

func (r *Reconciler) ensureS3User(ctx context.Context) (*ctrl.Result, error) {
	return subreconciler.ContinueReconciling()
}

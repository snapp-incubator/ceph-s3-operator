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
	"regexp"

	"github.com/ceph/go-ceph/rgw/admin"
	"github.com/go-logr/logr"
	"github.com/opdev/subreconciler"
	openshiftquota "github.com/openshift/api/quota/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
	scheme    *runtime.Scheme
	logger    logr.Logger
	rgwClient *admin.API

	// reconcile specific variables
	clusterResourceQuota   *openshiftquota.ClusterResourceQuota
	s3UserClaim            *s3v1alpha1.S3UserClaim
	cephUser               admin.User
	s3UserClaimNamespace   string
	cephTenant             string
	cephUserId             string
	cephUserFullId         string
	cephDisplayName        string
	s3UserName             string
	readonlyCephUserId     string
	readonlyCephUserFullId string

	// configurations
	clusterName  string
	rgwAccessKey string
	rgwSecretKey string
	rgwEndpoint  string
	s3UserClass  string
}

func NewReconciler(mgr manager.Manager, cfg *config.Config, rgwClient *admin.API) *Reconciler {
	return &Reconciler{
		Client:    mgr.GetClient(),
		scheme:    mgr.GetScheme(),
		rgwClient: rgwClient,

		s3UserClass:  cfg.S3UserClass,
		clusterName:  cfg.ClusterName,
		rgwAccessKey: cfg.Rgw.AccessKey,
		rgwSecretKey: cfg.Rgw.SecretKey,
		rgwEndpoint:  cfg.Rgw.Endpoint,
	}
}

//+kubebuilder:rbac:groups=s3.snappcloud.io,resources=s3userclaims,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=s3.snappcloud.io,resources=s3userclaims/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=s3.snappcloud.io,resources=s3userclaims/finalizers,verbs=update
//+kubebuilder:rbac:groups=s3.snappcloud.io,resources=s3users,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=s3.snappcloud.io,resources=s3users/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=s3.snappcloud.io,resources=s3users/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=namespaces,verbs=get;list;watch
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=resourcequotas,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=quota.openshift.io,resources=clusterresourcequotas,verbs=get;list;watch;create;update;patch;delete

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.logger = log.FromContext(ctx)
	r.s3UserClaim = &s3v1alpha1.S3UserClaim{}
	r.initVars(req)

	switch err := r.Get(ctx, req.NamespacedName, r.s3UserClaim); {
	case apierrors.IsNotFound(err):
		return r.Cleanup(ctx)
	case err != nil:
		r.logger.Error(err, "failed to fetch object")
		return subreconciler.Evaluate(subreconciler.Requeue())
	default:
		if r.s3UserClaim.ObjectMeta.DeletionTimestamp != nil {
			return r.Cleanup(ctx)
		}
	}

	return r.Provision(ctx)
}

func (r *Reconciler) initVars(req ctrl.Request) {
	r.s3UserClaimNamespace = req.Namespace

	// Only alphanumeric characters and underscore are allowed for tenant name
	k8sNameSpecialChars := regexp.MustCompile(`[.-]`)
	namespace := k8sNameSpecialChars.ReplaceAllString(req.Namespace, "_")
	clusterName := k8sNameSpecialChars.ReplaceAllString(r.clusterName, "_")
	r.cephTenant = fmt.Sprintf("%s__%s", clusterName, namespace)

	r.cephUserId = req.Name

	// Ceph-SDK functions that involve retrieving the user such as GetQuota, GetUser and even SetUser,
	// required tenant name in UID field.
	r.cephUserFullId = fmt.Sprintf("%s$%s", r.cephTenant, r.cephUserId)
	r.cephDisplayName = fmt.Sprintf("%s in %s.%s", req.Name, req.Namespace, r.clusterName)

	r.readonlyCephUserId = "readonly"
	r.readonlyCephUserFullId = fmt.Sprintf("%s:%s", r.cephUserFullId, r.readonlyCephUserId)

	r.s3UserName = fmt.Sprintf("%s.%s", req.Namespace, req.Name)
}

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

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// Reconciler reconciles a S3User object
type Reconciler struct {
	client.Client
	scheme *runtime.Scheme
}

func NewReconciler(mgr manager.Manager) *Reconciler {
	return &Reconciler{
		Client: mgr.GetClient(),
		scheme: mgr.GetScheme(),
	}
}

//+kubebuilder:rbac:groups=s3.snappcloud.io,resources=s3users,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=s3.snappcloud.io,resources=s3users/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=s3.snappcloud.io,resources=s3users/finalizers,verbs=update

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	return ctrl.Result{}, nil
}

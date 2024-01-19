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
	"path/filepath"
	"testing"
	"time"

	"github.com/ceph/go-ceph/rgw/admin"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	openshiftquota "github.com/openshift/api/quota"
	"k8s.io/client-go/kubernetes/scheme"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	s3v1alpha1 "github.com/snapp-incubator/ceph-s3-operator/api/v1alpha1"
	"github.com/snapp-incubator/ceph-s3-operator/internal/config"
	//+kubebuilder:scaffold:imports
)

var (
	restConfig       *rest.Config
	k8sClient        client.Client
	testEnv          *envtest.Environment
	managerCtx       context.Context
	managerCtxCancel context.CancelFunc
)

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	SetDefaultEventuallyTimeout(5 * time.Second)
	SetDefaultEventuallyPollingInterval(time.Second)

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

	var err error
	// restConfig is defined in this file globally.
	restConfig, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(restConfig).NotTo(BeNil())

	// Add schemas
	Expect(openshiftquota.Install(scheme.Scheme)).To(Succeed())
	Expect(clientgoscheme.AddToScheme(scheme.Scheme)).To(Succeed())
	Expect(s3v1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())

	//+kubebuilder:scaffold:scheme

	k8sClient, err = client.New(restConfig, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	k8sManager, err := ctrl.NewManager(restConfig, ctrl.Options{
		Scheme: scheme.Scheme,
	})
	Expect(err).ToNot(HaveOccurred())

	cfg := config.DefaultConfig
	co, err := admin.New(cfg.Rgw.Endpoint, cfg.Rgw.AccessKey, cfg.Rgw.SecretKey, nil)
	Expect(err).NotTo(HaveOccurred(), "failed to create rgw client")

	s3UserClaimReconciler := NewReconciler(k8sManager, &cfg, co)
	err = s3UserClaimReconciler.SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred(), "failed to setup s3UserClaim controller with manager")

	go func() {
		defer GinkgoRecover()
		managerCtx, managerCtxCancel = context.WithCancel(ctrl.SetupSignalHandler())
		err = k8sManager.Start(managerCtx)
		Expect(err).ToNot(HaveOccurred(), "failed to run manager")
	}()
})

var _ = AfterSuite(func() {
	managerCtxCancel()
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

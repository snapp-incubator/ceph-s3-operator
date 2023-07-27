package v1alpha1

import (
	"context"
	goerrors "errors"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	openshiftquota "github.com/openshift/api/quota/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/snapp-incubator/s3-operator/pkg/consts"
)

var _ = Describe("", Ordered, ContinueOnFailure, func() {
	const (
		targetNamespace = "s3userclaim-webhook-test"
		teamName        = "test-team"
		s3UserClaimName = "test-s3userclaim"
	)

	var (
		ctx         = context.Background()
		s3UserClaim = &S3UserClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      s3UserClaimName,
				Namespace: targetNamespace,
			},
			Spec: S3UserClaimSpec{
				AdminSecret:    "sample-admin",
				ReadonlySecret: "sample-readonly",
				S3UserClass:    "sample-s3userclass",
			},
		}
	)

	BeforeAll(func() {
		// Create target namespace
		Expect(k8sClient.Create(ctx, &v1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: targetNamespace,
				Labels: map[string]string{
					consts.LabelTeam: teamName,
				},
			},
		})).To(Succeed())

		// Create Cluster Resource Quota for the team
		Expect(k8sClient.Create(ctx, &openshiftquota.ClusterResourceQuota{
			ObjectMeta: metav1.ObjectMeta{
				Name: teamName,
			},
			Spec: openshiftquota.ClusterResourceQuotaSpec{
				Selector: openshiftquota.ClusterResourceQuotaSelector{
					LabelSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{consts.LabelTeam: teamName},
					},
				},
				Quota: v1.ResourceQuotaSpec{
					Hard: v1.ResourceList{
						consts.ResourceNameS3MaxSize:    resource.MustParse("3k"),
						consts.ResourceNameS3MaxObjects: resource.MustParse("4k"),
					},
				},
			},
		})).To(Succeed())

		// Create namespace quota
		//namespaceQuota := &v1.ResourceQuota{
		//	ObjectMeta: metav1.ObjectMeta{
		//		Name:      "default",
		//		Namespace: targetNamespace,
		//	},
		//	Spec: v1.ResourceQuotaSpec{
		//		Hard: v1.ResourceList{
		//			consts.ResourceNameS3MaxSize:    resource.MustParse("2k"),
		//			consts.ResourceNameS3MaxObjects: resource.MustParse("3k"),
		//		},
		//	},
		//}
		//Expect(k8sClient.Create(ctx, namespaceQuota)).To(Succeed())
	})

	Context("When creating S3UserClaim", func() {
		It("Should deny if requested max objects exceeds cluster quota", func() {
			s3UserClaim.Spec.Quota = &UserQuota{
				MaxSize:    resource.MustParse("2k"),
				MaxObjects: resource.MustParse("5k"),
			}

			Eventually(func(g Gomega) {
				err := k8sClient.Create(ctx, s3UserClaim)
				var apiStatus apierrors.APIStatus
				g.Expect(goerrors.As(err, &apiStatus)).To(BeTrue())
				g.Expect(apiStatus.Status().Code).To(Equal(int32(http.StatusUnprocessableEntity)))
				g.Expect(apiStatus.Status().Message).To(ContainSubstring(consts.ExceededClusterQuotaErrMessage))
				g.Expect(apiStatus.Status().Message).NotTo(ContainSubstring(consts.ExceededNamespaceQuotaErrMessage))
			}).WithTimeout(5 * time.Second).WithPolling(time.Second).Should(Succeed())
		})

		//It("Should deny if requested max size exceeds cluster quota", func() {
		//	s3UserClaim.Spec.Quota = &UserQuota{
		//		MaxSize:    resource.MustParse("5k"),
		//		MaxObjects: resource.MustParse("4k"),
		//	}
		//
		//	Eventually(func(g Gomega) {
		//		err := k8sClient.Create(ctx, s3UserClaim)
		//		var apiStatus apierrors.APIStatus
		//		g.Expect(goerrors.As(err, &apiStatus)).To(BeTrue())
		//		g.Expect(apiStatus.Status().Code).To(Equal(int32(http.StatusUnprocessableEntity)))
		//		g.Expect(apiStatus.Status().Message).To(ContainSubstring(consts.ExceededClusterQuotaErrMessage))
		//		g.Expect(apiStatus.Status().Message).NotTo(ContainSubstring(consts.ExceededNamespaceQuotaErrMessage))
		//	}).WithTimeout(5 * time.Second).WithPolling(time.Second).Should(Succeed())
		//})
		//
		//It("Should deny if requested max size exceeds namespace quota", func() {
		//
		//	s3UserClaim.Spec.Quota = &UserQuota{
		//		MaxSize:    resource.MustParse("2k"),
		//		MaxObjects: resource.MustParse("1"),
		//	}
		//
		//	Eventually(func(g Gomega) {
		//		err := k8sClient.Create(ctx, s3UserClaim)
		//		var apiStatus apierrors.APIStatus
		//		g.Expect(goerrors.As(err, &apiStatus)).To(BeTrue())
		//		g.Expect(apiStatus.Status().Code).To(Equal(int32(http.StatusUnprocessableEntity)))
		//		g.Expect(apiStatus.Status().Message).To(ContainSubstring(consts.ExceededNamespaceQuotaErrMessage))
		//		g.Expect(apiStatus.Status().Message).NotTo(ContainSubstring(consts.ExceededClusterQuotaErrMessage))
		//	}).WithTimeout(5 * time.Second).WithPolling(time.Second).Should(Succeed())
		//})
	})
})

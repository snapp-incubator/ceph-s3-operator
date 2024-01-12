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

	"github.com/snapp-incubator/ceph-s3-operator/pkg/consts"
)

var _ = Describe("", Ordered, ContinueOnFailure, func() {
	const (
		teamName        = "test-team"
		s3UserClaimName = "test-s3userclaim"
	)

	var (
		targetNamespaces = []string{
			"s3userclaim-webhook-test-1",
			"s3userclaim-webhook-test-2",
		}
		ctx = context.Background()
	)

	BeforeAll(func() {
		// Create target namespaces
		for _, ns := range targetNamespaces {
			Expect(k8sClient.Create(ctx, &v1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: ns,
					Labels: map[string]string{
						consts.LabelTeam: teamName,
					},
				},
			})).To(Succeed())
		}

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
						consts.ResourceNameS3MaxSize:    resource.MustParse("5k"),
						consts.ResourceNameS3MaxObjects: resource.MustParse("5k"),
						consts.ResourceNameS3MaxBuckets: resource.MustParse("5k"),
					},
				},
			},
		})).To(Succeed())

		// Create namespace quota
		for _, ns := range targetNamespaces {
			Expect(k8sClient.Create(ctx, &v1.ResourceQuota{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "default",
					Namespace: ns,
				},
				Spec: v1.ResourceQuotaSpec{
					Hard: v1.ResourceList{
						consts.ResourceNameS3MaxSize:    resource.MustParse("3k"),
						consts.ResourceNameS3MaxObjects: resource.MustParse("3k"),
						consts.ResourceNameS3MaxBuckets: resource.MustParse("3k"),
					},
				},
			})).To(Succeed())
		}
	})

	AfterEach(func() {
		// Delete the created s3UserClaims
		Eventually(func(g Gomega) {
			s3UserClaimList := &S3UserClaimList{}
			g.Expect(k8sClient.List(ctx, s3UserClaimList)).To(Succeed())

			for _, s3UserClaim := range s3UserClaimList.Items {
				g.Expect(k8sClient.Delete(ctx, &s3UserClaim)).To(Succeed())
			}
		}).WithTimeout(3 * time.Second).Should(Succeed())
	})

	Context("When creating S3UserClaim", func() {
		// Deny scenarios
		It("Should deny updating if s3UserClass is changed", func() {
			targetNamespace := targetNamespaces[0]
			s3UserClaim := getS3UserClaim(s3UserClaimName, targetNamespace, &UserQuota{
				MaxSize:    resource.MustParse("1k"),
				MaxObjects: resource.MustParse("1k"),
			})

			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Create(ctx, s3UserClaim)).To(Succeed())
			}).Should(Succeed())

			Eventually(func(g Gomega) {
				s3UserClaim.Spec.S3UserClass = "a-new-userclass"
				err := k8sClient.Update(ctx, s3UserClaim)
				var apiStatus apierrors.APIStatus
				g.Expect(goerrors.As(err, &apiStatus)).To(BeTrue())
				g.Expect(apiStatus.Status().Code).To(Equal(int32(http.StatusUnprocessableEntity)))
				g.Expect(apiStatus.Status().Message).To(ContainSubstring(consts.S3UserClassImmutableErrMessage))
			}).Should(Succeed())
		})

		It("Should deny creating if total requested max size exceeds cluster quota", func() {
			Eventually(func(g Gomega) {
				s3UserClaim := getS3UserClaim(s3UserClaimName, targetNamespaces[0], &UserQuota{
					MaxSize:    resource.MustParse("3k"),
					MaxObjects: resource.MustParse("1k"),
				})
				g.Expect(k8sClient.Create(ctx, s3UserClaim)).To(Succeed())
			}).Should(Succeed())

			Eventually(func(g Gomega) {
				s3UserClaim2 := getS3UserClaim(s3UserClaimName, targetNamespaces[1], &UserQuota{
					MaxSize:    resource.MustParse("3k"),
					MaxObjects: resource.MustParse("1k"),
				})
				err := k8sClient.Create(ctx, s3UserClaim2)
				var apiStatus apierrors.APIStatus
				g.Expect(goerrors.As(err, &apiStatus)).To(BeTrue())
				g.Expect(apiStatus.Status().Code).To(Equal(int32(http.StatusUnprocessableEntity)))
				g.Expect(apiStatus.Status().Message).To(ContainSubstring(consts.ErrExceededClusterQuota.Error()))
				g.Expect(apiStatus.Status().Message).NotTo(ContainSubstring(consts.ErrExceededNamespaceQuota.Error()))

				// Ensure only the first claim is created
				s3UserClaimList := &S3UserClaimList{}
				g.Expect(k8sClient.List(ctx, s3UserClaimList)).To(Succeed())
				g.Expect(len(s3UserClaimList.Items)).To(Equal(1))
			}).Should(Succeed())
		})
		It("Should deny updating if total requested max size exceeds cluster quota", func() {
			Eventually(func(g Gomega) {
				s3UserClaim := getS3UserClaim(s3UserClaimName, targetNamespaces[0], &UserQuota{
					MaxSize:    resource.MustParse("3k"),
					MaxObjects: resource.MustParse("1k"),
				})
				g.Expect(k8sClient.Create(ctx, s3UserClaim)).To(Succeed())
			}).Should(Succeed())

			Eventually(func(g Gomega) {
				s3UserClaim2 := getS3UserClaim(s3UserClaimName, targetNamespaces[1], &UserQuota{
					MaxSize:    resource.MustParse("1k"),
					MaxObjects: resource.MustParse("1k"),
				})
				g.Expect(k8sClient.Create(ctx, s3UserClaim2)).To(Succeed())

				s3UserClaim2.Spec.Quota.MaxSize = resource.MustParse("3k")
				err := k8sClient.Update(ctx, s3UserClaim2)
				var apiStatus apierrors.APIStatus
				g.Expect(goerrors.As(err, &apiStatus)).To(BeTrue())
				g.Expect(apiStatus.Status().Code).To(Equal(int32(http.StatusUnprocessableEntity)))
				g.Expect(apiStatus.Status().Message).To(ContainSubstring(consts.ErrExceededClusterQuota.Error()))
				g.Expect(apiStatus.Status().Message).NotTo(ContainSubstring(consts.ErrExceededNamespaceQuota.Error()))
			}).Should(Succeed())
		})

		It("Should deny creating if total requested max objects exceeds cluster quota", func() {
			Eventually(func(g Gomega) {
				s3UserClaim := getS3UserClaim(s3UserClaimName, targetNamespaces[0], &UserQuota{
					MaxSize:    resource.MustParse("1k"),
					MaxObjects: resource.MustParse("3k"),
				})
				g.Expect(k8sClient.Create(ctx, s3UserClaim)).To(Succeed())
			}).Should(Succeed())

			Eventually(func(g Gomega) {
				s3UserClaim2 := getS3UserClaim(s3UserClaimName, targetNamespaces[1], &UserQuota{
					MaxSize:    resource.MustParse("1k"),
					MaxObjects: resource.MustParse("3k"),
				})
				err := k8sClient.Create(ctx, s3UserClaim2)
				var apiStatus apierrors.APIStatus
				g.Expect(goerrors.As(err, &apiStatus)).To(BeTrue())
				g.Expect(apiStatus.Status().Code).To(Equal(int32(http.StatusUnprocessableEntity)))
				g.Expect(apiStatus.Status().Message).To(ContainSubstring(consts.ErrExceededClusterQuota.Error()))
				g.Expect(apiStatus.Status().Message).NotTo(ContainSubstring(consts.ErrExceededNamespaceQuota.Error()))

				// Ensure only the first claim is created
				s3UserClaimList := &S3UserClaimList{}
				g.Expect(k8sClient.List(ctx, s3UserClaimList)).To(Succeed())
				g.Expect(len(s3UserClaimList.Items)).To(Equal(1))
			}).Should(Succeed())
		})
		It("Should deny updating if total requested max objects exceeds cluster quota", func() {
			Eventually(func(g Gomega) {
				s3UserClaim := getS3UserClaim(s3UserClaimName, targetNamespaces[0], &UserQuota{
					MaxSize:    resource.MustParse("1k"),
					MaxObjects: resource.MustParse("3k"),
				})
				g.Expect(k8sClient.Create(ctx, s3UserClaim)).To(Succeed())
			}).Should(Succeed())

			Eventually(func(g Gomega) {
				s3UserClaim2 := getS3UserClaim(s3UserClaimName, targetNamespaces[1], &UserQuota{
					MaxSize:    resource.MustParse("1k"),
					MaxObjects: resource.MustParse("1k"),
				})
				g.Expect(k8sClient.Create(ctx, s3UserClaim2)).To(Succeed())

				s3UserClaim2.Spec.Quota.MaxObjects = resource.MustParse("3k")
				err := k8sClient.Update(ctx, s3UserClaim2)
				var apiStatus apierrors.APIStatus
				g.Expect(goerrors.As(err, &apiStatus)).To(BeTrue())
				g.Expect(apiStatus.Status().Code).To(Equal(int32(http.StatusUnprocessableEntity)))
				g.Expect(apiStatus.Status().Message).To(ContainSubstring(consts.ErrExceededClusterQuota.Error()))
				g.Expect(apiStatus.Status().Message).NotTo(ContainSubstring(consts.ErrExceededNamespaceQuota.Error()))
			}).Should(Succeed())
		})

		It("Should deny creating if total requested max buckets exceeds cluster quota", func() {
			Eventually(func(g Gomega) {
				s3UserClaim := getS3UserClaim(s3UserClaimName, targetNamespaces[0], &UserQuota{
					MaxSize:    resource.MustParse("1k"),
					MaxObjects: resource.MustParse("1k"),
					MaxBuckets: 3000,
				})
				g.Expect(k8sClient.Create(ctx, s3UserClaim)).To(Succeed())
			}).Should(Succeed())

			Eventually(func(g Gomega) {
				s3UserClaim2 := getS3UserClaim(s3UserClaimName, targetNamespaces[1], &UserQuota{
					MaxSize:    resource.MustParse("1k"),
					MaxObjects: resource.MustParse("1k"),
					MaxBuckets: 3000,
				})
				err := k8sClient.Create(ctx, s3UserClaim2)
				var apiStatus apierrors.APIStatus
				g.Expect(goerrors.As(err, &apiStatus)).To(BeTrue())
				g.Expect(apiStatus.Status().Code).To(Equal(int32(http.StatusUnprocessableEntity)))
				g.Expect(apiStatus.Status().Message).To(ContainSubstring(consts.ErrExceededClusterQuota.Error()))
				g.Expect(apiStatus.Status().Message).NotTo(ContainSubstring(consts.ErrExceededNamespaceQuota.Error()))

				// Ensure only the first claim is created
				s3UserClaimList := &S3UserClaimList{}
				g.Expect(k8sClient.List(ctx, s3UserClaimList)).To(Succeed())
				g.Expect(len(s3UserClaimList.Items)).To(Equal(1))
			}).Should(Succeed())
		})
		It("Should deny updating if total requested max buckets exceeds cluster quota", func() {
			Eventually(func(g Gomega) {
				s3UserClaim := getS3UserClaim(s3UserClaimName, targetNamespaces[0], &UserQuota{
					MaxSize:    resource.MustParse("1k"),
					MaxObjects: resource.MustParse("1k"),
					MaxBuckets: 3000,
				})
				g.Expect(k8sClient.Create(ctx, s3UserClaim)).To(Succeed())
			}).Should(Succeed())

			Eventually(func(g Gomega) {
				s3UserClaim2 := getS3UserClaim(s3UserClaimName, targetNamespaces[1], &UserQuota{
					MaxSize:    resource.MustParse("1k"),
					MaxObjects: resource.MustParse("1k"),
					MaxBuckets: 1000,
				})
				g.Expect(k8sClient.Create(ctx, s3UserClaim2)).To(Succeed())

				s3UserClaim2.Spec.Quota.MaxBuckets = 3000
				err := k8sClient.Update(ctx, s3UserClaim2)
				var apiStatus apierrors.APIStatus
				g.Expect(goerrors.As(err, &apiStatus)).To(BeTrue())
				g.Expect(apiStatus.Status().Code).To(Equal(int32(http.StatusUnprocessableEntity)))
				g.Expect(apiStatus.Status().Message).To(ContainSubstring(consts.ErrExceededClusterQuota.Error()))
				g.Expect(apiStatus.Status().Message).NotTo(ContainSubstring(consts.ErrExceededNamespaceQuota.Error()))
			}).Should(Succeed())
		})

		It("Should deny creating if total requested max size exceeds namespace quota", func() {
			targetNamespace := targetNamespaces[0]
			Eventually(func(g Gomega) {
				s3UserClaim := getS3UserClaim(s3UserClaimName, targetNamespace, &UserQuota{
					MaxSize:    resource.MustParse("2k"),
					MaxObjects: resource.MustParse("1k"),
				})
				g.Expect(k8sClient.Create(ctx, s3UserClaim)).To(Succeed())
			}).Should(Succeed())

			Eventually(func(g Gomega) {
				s3UserClaim2 := getS3UserClaim(s3UserClaimName+"2", targetNamespace, &UserQuota{
					MaxSize:    resource.MustParse("2k"),
					MaxObjects: resource.MustParse("1k"),
				})
				err := k8sClient.Create(ctx, s3UserClaim2)
				var apiStatus apierrors.APIStatus
				g.Expect(goerrors.As(err, &apiStatus)).To(BeTrue())
				g.Expect(apiStatus.Status().Code).To(Equal(int32(http.StatusUnprocessableEntity)))
				g.Expect(apiStatus.Status().Message).To(ContainSubstring(consts.ErrExceededNamespaceQuota.Error()))
				g.Expect(apiStatus.Status().Message).NotTo(ContainSubstring(consts.ErrExceededClusterQuota.Error()))

				// Ensure only the first claim is created
				s3UserClaimList := &S3UserClaimList{}
				g.Expect(k8sClient.List(ctx, s3UserClaimList)).To(Succeed())
				g.Expect(len(s3UserClaimList.Items)).To(Equal(1))
			}).Should(Succeed())
		})
		It("Should deny updating if total requested max size exceeds namespace quota", func() {
			targetNamespace := targetNamespaces[0]
			Eventually(func(g Gomega) {
				s3UserClaim := getS3UserClaim(s3UserClaimName, targetNamespace, &UserQuota{
					MaxSize:    resource.MustParse("2k"),
					MaxObjects: resource.MustParse("1k"),
				})
				g.Expect(k8sClient.Create(ctx, s3UserClaim)).To(Succeed())
			}).Should(Succeed())

			Eventually(func(g Gomega) {
				s3UserClaim2 := getS3UserClaim(s3UserClaimName+"2", targetNamespace, &UserQuota{
					MaxSize:    resource.MustParse("1k"),
					MaxObjects: resource.MustParse("1k"),
				})
				g.Expect(k8sClient.Create(ctx, s3UserClaim2)).To(Succeed())

				s3UserClaim2.Spec.Quota.MaxSize = resource.MustParse("2k")
				err := k8sClient.Update(ctx, s3UserClaim2)
				var apiStatus apierrors.APIStatus
				g.Expect(goerrors.As(err, &apiStatus)).To(BeTrue())
				g.Expect(apiStatus.Status().Code).To(Equal(int32(http.StatusUnprocessableEntity)))
				g.Expect(apiStatus.Status().Message).To(ContainSubstring(consts.ErrExceededNamespaceQuota.Error()))
				g.Expect(apiStatus.Status().Message).NotTo(ContainSubstring(consts.ErrExceededClusterQuota.Error()))
			}).Should(Succeed())
		})

		It("Should deny creating if total requested max objects exceeds namespace quota", func() {
			targetNamespace := targetNamespaces[0]
			Eventually(func(g Gomega) {
				s3UserClaim := getS3UserClaim(s3UserClaimName, targetNamespace, &UserQuota{
					MaxSize:    resource.MustParse("1k"),
					MaxObjects: resource.MustParse("2k"),
				})
				g.Expect(k8sClient.Create(ctx, s3UserClaim)).To(Succeed())
			}).Should(Succeed())

			Eventually(func(g Gomega) {
				s3UserClaim2 := getS3UserClaim(s3UserClaimName+"2", targetNamespace, &UserQuota{
					MaxSize:    resource.MustParse("1k"),
					MaxObjects: resource.MustParse("2k"),
				})
				err := k8sClient.Create(ctx, s3UserClaim2)
				var apiStatus apierrors.APIStatus
				g.Expect(goerrors.As(err, &apiStatus)).To(BeTrue())
				g.Expect(apiStatus.Status().Code).To(Equal(int32(http.StatusUnprocessableEntity)))
				g.Expect(apiStatus.Status().Message).To(ContainSubstring(consts.ErrExceededNamespaceQuota.Error()))
				g.Expect(apiStatus.Status().Message).NotTo(ContainSubstring(consts.ErrExceededClusterQuota.Error()))

				// Ensure only the first claim is created
				s3UserClaimList := &S3UserClaimList{}
				g.Expect(k8sClient.List(ctx, s3UserClaimList)).To(Succeed())
				g.Expect(len(s3UserClaimList.Items)).To(Equal(1))
			}).Should(Succeed())
		})
		It("Should deny updating if total requested max objects exceeds namespace quota", func() {
			targetNamespace := targetNamespaces[0]
			Eventually(func(g Gomega) {
				s3UserClaim := getS3UserClaim(s3UserClaimName, targetNamespace, &UserQuota{
					MaxSize:    resource.MustParse("1k"),
					MaxObjects: resource.MustParse("2k"),
				})
				g.Expect(k8sClient.Create(ctx, s3UserClaim)).To(Succeed())
			}).Should(Succeed())

			Eventually(func(g Gomega) {
				s3UserClaim2 := getS3UserClaim(s3UserClaimName+"2", targetNamespace, &UserQuota{
					MaxSize:    resource.MustParse("1k"),
					MaxObjects: resource.MustParse("1k"),
				})
				g.Expect(k8sClient.Create(ctx, s3UserClaim2)).To(Succeed())

				s3UserClaim2.Spec.Quota.MaxObjects = resource.MustParse("2k")
				err := k8sClient.Update(ctx, s3UserClaim2)
				var apiStatus apierrors.APIStatus
				g.Expect(goerrors.As(err, &apiStatus)).To(BeTrue())
				g.Expect(apiStatus.Status().Code).To(Equal(int32(http.StatusUnprocessableEntity)))
				g.Expect(apiStatus.Status().Message).To(ContainSubstring(consts.ErrExceededNamespaceQuota.Error()))
				g.Expect(apiStatus.Status().Message).NotTo(ContainSubstring(consts.ErrExceededClusterQuota.Error()))
			}).Should(Succeed())
		})

		It("Should deny creating if total requested max buckets exceeds namespace quota", func() {
			targetNamespace := targetNamespaces[0]
			Eventually(func(g Gomega) {
				s3UserClaim := getS3UserClaim(s3UserClaimName, targetNamespace, &UserQuota{
					MaxSize:    resource.MustParse("1k"),
					MaxObjects: resource.MustParse("1k"),
					MaxBuckets: 2000,
				})
				g.Expect(k8sClient.Create(ctx, s3UserClaim)).To(Succeed())
			}).Should(Succeed())

			Eventually(func(g Gomega) {
				s3UserClaim2 := getS3UserClaim(s3UserClaimName+"2", targetNamespace, &UserQuota{
					MaxSize:    resource.MustParse("1k"),
					MaxObjects: resource.MustParse("1k"),
					MaxBuckets: 2000,
				})
				err := k8sClient.Create(ctx, s3UserClaim2)
				var apiStatus apierrors.APIStatus
				g.Expect(goerrors.As(err, &apiStatus)).To(BeTrue())
				g.Expect(apiStatus.Status().Code).To(Equal(int32(http.StatusUnprocessableEntity)))
				g.Expect(apiStatus.Status().Message).To(ContainSubstring(consts.ErrExceededNamespaceQuota.Error()))
				g.Expect(apiStatus.Status().Message).NotTo(ContainSubstring(consts.ErrExceededClusterQuota.Error()))

				// Ensure only the first claim is created
				s3UserClaimList := &S3UserClaimList{}
				g.Expect(k8sClient.List(ctx, s3UserClaimList)).To(Succeed())
				g.Expect(len(s3UserClaimList.Items)).To(Equal(1))
			}).Should(Succeed())
		})
		It("Should deny updating if total requested max buckets exceeds namespace quota", func() {
			targetNamespace := targetNamespaces[0]
			Eventually(func(g Gomega) {
				s3UserClaim := getS3UserClaim(s3UserClaimName, targetNamespace, &UserQuota{
					MaxSize:    resource.MustParse("1k"),
					MaxObjects: resource.MustParse("2k"),
					MaxBuckets: 2000,
				})
				g.Expect(k8sClient.Create(ctx, s3UserClaim)).To(Succeed())
			}).Should(Succeed())

			Eventually(func(g Gomega) {
				s3UserClaim2 := getS3UserClaim(s3UserClaimName+"2", targetNamespace, &UserQuota{
					MaxSize:    resource.MustParse("1k"),
					MaxObjects: resource.MustParse("1k"),
					MaxBuckets: 1000,
				})
				g.Expect(k8sClient.Create(ctx, s3UserClaim2)).To(Succeed())

				s3UserClaim2.Spec.Quota.MaxBuckets = 2000
				err := k8sClient.Update(ctx, s3UserClaim2)
				var apiStatus apierrors.APIStatus
				g.Expect(goerrors.As(err, &apiStatus)).To(BeTrue())
				g.Expect(apiStatus.Status().Code).To(Equal(int32(http.StatusUnprocessableEntity)))
				g.Expect(apiStatus.Status().Message).To(ContainSubstring(consts.ErrExceededNamespaceQuota.Error()))
				g.Expect(apiStatus.Status().Message).NotTo(ContainSubstring(consts.ErrExceededClusterQuota.Error()))
			}).Should(Succeed())
		})

		// Allow scenarios
		It("Should allow creating if total requested quota doesn't exceed any quota", func() {
			Eventually(func(g Gomega) {
				s3UserClaim := getS3UserClaim(s3UserClaimName, targetNamespaces[0], &UserQuota{
					MaxSize:    resource.MustParse("1k"),
					MaxObjects: resource.MustParse("1k"),
					MaxBuckets: 1000,
				})
				g.Expect(k8sClient.Create(ctx, s3UserClaim)).To(Succeed())
			}).Should(Succeed())

			Eventually(func(g Gomega) {
				s3UserClaim2 := getS3UserClaim(s3UserClaimName, targetNamespaces[1], &UserQuota{
					MaxSize:    resource.MustParse("1k"),
					MaxObjects: resource.MustParse("1k"),
					MaxBuckets: 1000,
				})
				g.Expect(k8sClient.Create(ctx, s3UserClaim2)).To(Succeed())
			}).Should(Succeed())
		})
		It(
			"Should allow updating if total requested quota doesn't exceed any quota and s3UserClass is not changed",
			func() {
				Eventually(func(g Gomega) {
					s3UserClaim := getS3UserClaim(s3UserClaimName, targetNamespaces[0], &UserQuota{
						MaxSize:    resource.MustParse("1k"),
						MaxObjects: resource.MustParse("1k"),
						MaxBuckets: 1000,
					})
					g.Expect(k8sClient.Create(ctx, s3UserClaim)).To(Succeed())
				}).Should(Succeed())

				Eventually(func(g Gomega) {
					s3UserClaim2 := getS3UserClaim(s3UserClaimName, targetNamespaces[1], &UserQuota{
						MaxSize:    resource.MustParse("10"),
						MaxObjects: resource.MustParse("10"),
						MaxBuckets: 10,
					})
					g.Expect(k8sClient.Create(ctx, s3UserClaim2)).To(Succeed())

					s3UserClaim2.Spec.Quota = &UserQuota{
						MaxSize:    resource.MustParse("1k"),
						MaxObjects: resource.MustParse("1k"),
						MaxBuckets: 1000,
					}
					g.Expect(k8sClient.Update(ctx, s3UserClaim2)).To(Succeed())
				}).Should(Succeed())
			},
		)
	})

	Context("When creating S3UserClaim without ClusterResourceQuota", func() {
		// Deny scenarios
		It("Should deny", func() {
			const (
				ns   = "new-ns"
				team = "new-team"
			)
			Expect(k8sClient.Create(ctx, &v1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:   ns,
					Labels: map[string]string{consts.LabelTeam: team},
				},
			})).To(Succeed())
			s3UserClaim := getS3UserClaim(s3UserClaimName, ns, &UserQuota{
				MaxSize:    resource.MustParse("1k"),
				MaxObjects: resource.MustParse("1k"),
			})

			err := k8sClient.Create(ctx, s3UserClaim)
			var apiStatus apierrors.APIStatus
			Expect(goerrors.As(err, &apiStatus)).To(BeTrue())
			Expect(apiStatus.Status().Code).To(Equal(int32(http.StatusUnprocessableEntity)))
			Expect(apiStatus.Status().Message).To(ContainSubstring(consts.ErrClusterQuotaNotDefined.Error()))
			Expect(apiStatus.Status().Message).NotTo(ContainSubstring(consts.ErrExceededClusterQuota.Error()))
			Expect(apiStatus.Status().Message).NotTo(ContainSubstring(consts.ErrExceededNamespaceQuota.Error()))
		})
	})
})

func getS3UserClaim(name, namespace string, quota *UserQuota) *S3UserClaim {
	return &S3UserClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: S3UserClaimSpec{
			AdminSecret:    "sample-admin",
			ReadonlySecret: "sample-readonly",
			S3UserClass:    "sample-s3userclass",
			Quota:          quota,
		},
	}
}

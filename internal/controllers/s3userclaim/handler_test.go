package s3userclaim

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/ceph/go-ceph/rgw/admin"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	s3v1alpha1 "github.com/snapp-incubator/s3-operator/api/v1alpha1"
	"github.com/snapp-incubator/s3-operator/internal/config"
	"github.com/snapp-incubator/s3-operator/pkg/consts"
)

var _ = Describe("S3UserClaim Controller", func() {
	const (
		s3UserClass          = "ceph-default"
		s3UserClaimName      = "test-s3userclaim"
		s3UserClaimNamespace = "default"
		adminSecretName      = "admin-secret"
		readonlySecretName   = "readonly-secret"
	)
	var (
		s3UserName          = fmt.Sprintf("%s.%s", s3UserClaimNamespace, s3UserClaimName)
		quotaMaxSize        = resource.MustParse("3k")
		quotaMaxObjects     = resource.MustParse("4M")
		cfg                 = config.DefaultConfig
		ctx                 = context.Background()
		k8sNameSpecialChars = regexp.MustCompile(`[.-]`)
		cephUser            = admin.User{
			ID: fmt.Sprintf(
				"%s__%s$%s",
				k8sNameSpecialChars.ReplaceAllString(cfg.ClusterName, "_"),
				k8sNameSpecialChars.ReplaceAllString(s3UserClaimNamespace, "_"),
				s3UserClaimName,
			),
		}
		readonlyCephUser = admin.SubuserSpec{
			Name: fmt.Sprintf("%s:%s", cephUser.ID, "readonly"),
		}
		rgwClient      *admin.API
		s3UserClaim    *s3v1alpha1.S3UserClaim
		s3User         *s3v1alpha1.S3User
		adminSecret    *v1.Secret
		readonlySecret *v1.Secret
	)

	co, err := admin.New(cfg.Rgw.Endpoint, cfg.Rgw.AccessKey, cfg.Rgw.SecretKey, nil)
	Expect(err).NotTo(HaveOccurred())
	rgwClient = co

	Context("When creating a new S3UserClaim", func() {
		BeforeEach(func() {
			s3UserClaim = &s3v1alpha1.S3UserClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      s3UserClaimName,
					Namespace: s3UserClaimNamespace,
				},
				Spec: s3v1alpha1.S3UserClaimSpec{
					S3UserClass:    s3UserClass,
					ReadonlySecret: readonlySecretName,
					AdminSecret:    adminSecretName,
					Quota: &s3v1alpha1.UserQuota{
						MaxSize:    quotaMaxSize,
						MaxObjects: quotaMaxObjects,
					},
				},
			}

			s3User = &s3v1alpha1.S3User{
				ObjectMeta: metav1.ObjectMeta{
					Name: s3UserName,
				},
			}

			adminSecret = &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      adminSecretName,
					Namespace: s3UserClaimNamespace,
				},
			}

			readonlySecret = &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      readonlySecretName,
					Namespace: s3UserClaimNamespace,
				},
			}
			Expect(k8sClient.Create(ctx, s3UserClaim)).To(Succeed())
		})

		AfterEach(func() {
			Expect(k8sClient.Delete(ctx, s3UserClaim)).To(Succeed())
			Expect(k8sClient.Delete(ctx, s3User)).To(Succeed())
			Expect(k8sClient.Delete(ctx, adminSecret)).To(Succeed())
			Expect(k8sClient.Delete(ctx, readonlySecret)).To(Succeed())
			Expect(rgwClient.RemoveUser(ctx, cephUser)).To(Succeed())
		})

		It("Should create Ceph user", func() {
			k8sNameSpecialChars := regexp.MustCompile(`[.-]`)
			initialUser := admin.User{
				ID: fmt.Sprintf(
					"%s__%s$%s",
					k8sNameSpecialChars.ReplaceAllString(cfg.ClusterName, "_"),
					k8sNameSpecialChars.ReplaceAllString(s3UserClaimNamespace, "_"),
					s3UserClaim.Name,
				),
			}
			Eventually(func(g Gomega) {
				gotUser, err := rgwClient.GetUser(ctx, initialUser)
				g.Expect(err).NotTo(HaveOccurred())

				g.Expect(gotUser.UserQuota).NotTo(BeNil())
				g.Expect(gotUser.UserQuota.MaxSize).NotTo(BeNil())
				g.Expect(gotUser.UserQuota.MaxObjects).NotTo(BeNil())
				g.Expect(*gotUser.UserQuota.MaxSize).To(Equal(quotaMaxSize.Value()))
				g.Expect(*gotUser.UserQuota.MaxObjects).To(Equal(quotaMaxObjects.Value()))
			}).WithTimeout(5 * time.Second).WithPolling(time.Second).Should(Succeed())
		})

		It("Should create admin secret", func() {
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(
					ctx,
					types.NamespacedName{Name: adminSecretName, Namespace: s3UserClaimNamespace},
					adminSecret,
				)).To(Succeed())

				g.Expect(metav1.IsControlledBy(adminSecret, s3UserClaim)).To(BeTrue())

				gotCephUser, err := rgwClient.GetUser(ctx, cephUser)
				g.Expect(err).NotTo(HaveOccurred())

				g.Expect(len(gotCephUser.Keys)).NotTo(BeZero())

				g.Expect(gotCephUser.Keys).To(ContainElement(admin.UserKeySpec{
					User:      cephUser.ID,
					AccessKey: string(adminSecret.Data[consts.DataKeyAccessKey]),
					SecretKey: string(adminSecret.Data[consts.DataKeySecretKey]),
				}))
			}).WithTimeout(5 * time.Second).WithPolling(time.Second).Should(Succeed())
		})

		It("Should create readonly secret", func() {
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(
					ctx,
					types.NamespacedName{Name: readonlySecretName, Namespace: s3UserClaimNamespace},
					readonlySecret,
				)).To(Succeed())

				g.Expect(metav1.IsControlledBy(readonlySecret, s3UserClaim)).To(BeTrue())

				gotCephUser, err := rgwClient.GetUser(ctx, cephUser)
				g.Expect(err).NotTo(HaveOccurred())

				g.Expect(len(gotCephUser.Keys)).To(BeNumerically("==", 2))
				g.Expect(gotCephUser.Keys).To(ContainElement(admin.UserKeySpec{
					User:      readonlyCephUser.Name,
					AccessKey: string(readonlySecret.Data[consts.DataKeyAccessKey]),
					SecretKey: string(readonlySecret.Data[consts.DataKeySecretKey]),
				}))
			}).WithTimeout(5 * time.Second).WithPolling(time.Second).Should(Succeed())
		})

		It("Should create S3User", func() {
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: s3UserName}, s3User)
				g.Expect(err).NotTo(HaveOccurred())

				g.Expect(s3User.Spec.Quota).NotTo(BeNil())
				g.Expect(*s3User.Spec.Quota).To(Equal(*s3UserClaim.Spec.Quota))
				g.Expect(s3User.Spec.S3UserClass).To(Equal(s3UserClaim.Spec.S3UserClass))
				g.Expect(s3User.Spec.S3UserClass).To(Equal(s3UserClaim.Spec.S3UserClass))
				g.Expect(s3User.Spec.ClaimRef.Name).To(Equal(s3UserClaim.Name))
			}).WithTimeout(5 * time.Second).WithPolling(time.Second).Should(Succeed())
		})

		It("Should update status of S3UserClaim", func() {
			Eventually(func(g Gomega) {
				err := k8sClient.Get(
					ctx,
					types.NamespacedName{Name: s3UserClaimName, Namespace: s3UserClaimNamespace},
					s3UserClaim,
				)
				g.Expect(err).NotTo(HaveOccurred())

				g.Expect(s3UserClaim.Status.Quota).NotTo(BeNil())
				g.Expect(*s3UserClaim.Status.Quota).To(Equal(*s3UserClaim.Spec.Quota))
				g.Expect(s3UserClaim.Status.S3UserName).To(Equal(s3UserName))
			}).WithTimeout(5 * time.Second).WithPolling(time.Second).Should(Succeed())
		})
	})
})

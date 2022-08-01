package appstudioredhatcom

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appstudiosharedv1 "github.com/redhat-appstudio/managed-gitops/appstudio-shared/apis/appstudio.redhat.com/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/eventlooptypes"
)

var _ = Describe("ApplicationSnapshotEnvironmentBinding Reconciler Tests", func() {
	Context("Testing ApplicationSnapshotEnvironmentBindingReconciler.", func() {

		var ctx context.Context
		var request reconcile.Request
		var environment appstudiosharedv1.Environment
		var promotionRun *appstudiosharedv1.ApplicationPromotionRun
		var promotionRunReconciler ApplicationPromotionRunReconciler

		BeforeEach(func() {
			ctx = context.Background()

			scheme,
				argocdNamespace,
				kubesystemNamespace,
				apiNamespace,
				err := eventlooptypes.GenericTestSetup()
			Expect(err).To(BeNil())

			// Create fake client
			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(apiNamespace, argocdNamespace, kubesystemNamespace).
				Build()

			// Create placeholder environment
			environment = appstudiosharedv1.Environment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "staging",
					Namespace: apiNamespace.Name,
				},
				Spec: appstudiosharedv1.EnvironmentSpec{
					DisplayName:        "my-environment",
					Type:               appstudiosharedv1.EnvironmentType_POC,
					DeploymentStrategy: appstudiosharedv1.DeploymentStrategy_AppStudioAutomated,
					ParentEnvironment:  "",
					Tags:               []string{},
					Configuration:      appstudiosharedv1.EnvironmentConfiguration{},
				},
			}
			err = k8sClient.Create(ctx, &environment)
			Expect(err).To(BeNil())

			promotionRunReconciler = ApplicationPromotionRunReconciler{Client: k8sClient, Scheme: scheme}

			promotionRun = &appstudiosharedv1.ApplicationPromotionRun{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "appstudio.redhat.com/v1alpha1",
					Kind:       "ApplicationPromotionRun",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "new-demo-app-manual-promotion",
					Namespace: apiNamespace.Name,
				},
				Spec: appstudiosharedv1.ApplicationPromotionRunSpec{
					Snapshot:    "my-snapshot",
					Application: "new-demo-app",
					ManualPromotion: appstudiosharedv1.ManualPromotionConfiguration{
						TargetEnvironment: "prod",
					},
				},
			}

			request = newRequest(apiNamespace.Name, promotionRun.Name)
		})

		It("Should do nothing as PromotonRun CR is not created.", func() {
			// Trigger Reconciler
			_, err := promotionRunReconciler.Reconcile(ctx, request)
			Expect(err).To(BeNil())
		})

		It("Should do nothing as status is complete.", func() {
			promotionRun.Status = appstudiosharedv1.ApplicationPromotionRunStatus{
				State: appstudiosharedv1.PromotionRunState_Complete,
			}
			err := promotionRunReconciler.Create(ctx, promotionRun)
			Expect(err).To(BeNil())

			// Trigger Reconciler
			_, err = promotionRunReconciler.Reconcile(ctx, request)
			Expect(err).To(BeNil())
		})

		It("Should checkForExistingActivePromotions 1", func() {
			promotionRunTemp := &appstudiosharedv1.ApplicationPromotionRun{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "appstudio.redhat.com/v1alpha1",
					Kind:       "ApplicationPromotionRun",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "new-demo-app-auto-promotion",
					Namespace: promotionRun.Namespace,
				},
				Spec: appstudiosharedv1.ApplicationPromotionRunSpec{
					Snapshot:    "my-snapshot",
					Application: "new-demo-app",
					ManualPromotion: appstudiosharedv1.ManualPromotionConfiguration{
						TargetEnvironment: "prod",
					},
				},
				Status: appstudiosharedv1.ApplicationPromotionRunStatus{
					State: appstudiosharedv1.PromotionRunState_Complete,
				},
			}

			err := promotionRunReconciler.Create(ctx, promotionRunTemp)
			Expect(err).To(BeNil())

			err = promotionRunReconciler.Create(ctx, promotionRun)
			Expect(err).To(BeNil())

			// Trigger Reconciler
			_, err = promotionRunReconciler.Reconcile(ctx, request)
			Expect(err).To(BeNil())
		})

		It("Should checkForExistingActivePromotions 2", func() {
			promotionRunTemp := &appstudiosharedv1.ApplicationPromotionRun{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "appstudio.redhat.com/v1alpha1",
					Kind:       "ApplicationPromotionRun",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "new-demo-app-auto-promotion",
					Namespace: promotionRun.Namespace,
				},
				Spec: appstudiosharedv1.ApplicationPromotionRunSpec{
					Snapshot:    "my-snapshot",
					Application: "new-demo-app-v1",
					ManualPromotion: appstudiosharedv1.ManualPromotionConfiguration{
						TargetEnvironment: "prod",
					},
				},
			}

			err := promotionRunReconciler.Create(ctx, promotionRunTemp)
			Expect(err).To(BeNil())

			err = promotionRunReconciler.Create(ctx, promotionRun)
			Expect(err).To(BeNil())

			// Trigger Reconciler
			_, err = promotionRunReconciler.Reconcile(ctx, request)
			Expect(err).To(BeNil())
		})

		It("Should checkForExistingActivePromotions 3", func() {
			promotionRunTemp := &appstudiosharedv1.ApplicationPromotionRun{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "appstudio.redhat.com/v1alpha1",
					Kind:       "ApplicationPromotionRun",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:              "new-demo-app-auto-promotion",
					Namespace:         promotionRun.Namespace,
					CreationTimestamp: metav1.NewTime(time.Date(2009, 11, 17, 20, 34, 58, 651387237, time.UTC)),
				},
				Spec: appstudiosharedv1.ApplicationPromotionRunSpec{
					Snapshot:    "my-snapshot",
					Application: "new-demo-app",
					ManualPromotion: appstudiosharedv1.ManualPromotionConfiguration{
						TargetEnvironment: "prod",
					},
				},
			}

			err := promotionRunReconciler.Create(ctx, promotionRunTemp)
			Expect(err).To(BeNil())

			promotionRun.CreationTimestamp = metav1.NewTime(time.Date(2010, 11, 17, 20, 34, 58, 651387237, time.UTC))
			err = promotionRunReconciler.Create(ctx, promotionRun)
			Expect(err).To(BeNil())

			// Trigger Reconciler
			_, err = promotionRunReconciler.Reconcile(ctx, request)
			Expect(err).To(BeNil())
		})

		It("Should not support auto promo.", func() {
			promotionRun.Spec.AutomatedPromotion.InitialEnvironment = "abc"
			err := promotionRunReconciler.Create(ctx, promotionRun)
			Expect(err).To(BeNil())

			// Trigger Reconciler
			_, err = promotionRunReconciler.Reconcile(ctx, request)
			Expect(err).To(BeNil())
		})

		FIt("Should.", func() {
			applicationSnapshot := appstudiosharedv1.ApplicationSnapshot{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "appstudio.redhat.com/v1alpha1",
					Kind:       "ApplicationSnapshot",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-snapshot",
					Namespace: promotionRun.Namespace,
				},
				Spec: appstudiosharedv1.ApplicationSnapshotSpec{
					Application:        "new-demo-app",
					DisplayName:        "new-demo-app",
					DisplayDescription: "new-demo-app",
				},
			}

			err := promotionRunReconciler.Create(ctx, &applicationSnapshot)
			Expect(err).To(BeNil())

			err = promotionRunReconciler.Create(ctx, promotionRun)
			Expect(err).To(BeNil())

			binding := &appstudiosharedv1.ApplicationSnapshotEnvironmentBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "appa-staging-binding",
					Namespace: promotionRun.Namespace,
					Labels: map[string]string{
						"appstudio.application": "new-demo-app",
						"appstudio.environment": "staging",
					},
				},
				Spec: appstudiosharedv1.ApplicationSnapshotEnvironmentBindingSpec{
					Application: "new-demo-app",
					Environment: "prod",
					Snapshot:    "my-snapshot",
					Components: []appstudiosharedv1.BindingComponent{
						{
							Name: "component-a",
							Configuration: appstudiosharedv1.BindingComponentConfiguration{
								Env: []appstudiosharedv1.EnvVarPair{
									{Name: "My_STG_ENV", Value: "1000"},
								},
								Replicas: 3,
							},
						},
					},
				},
				Status: appstudiosharedv1.ApplicationSnapshotEnvironmentBindingStatus{
					Components: []appstudiosharedv1.ComponentStatus{
						{
							Name: "component-a",
							GitOpsRepository: appstudiosharedv1.BindingComponentGitOpsRepository{
								URL:    "https://github.com/redhat-appstudio/gitops-repository-template",
								Branch: "main",
								Path:   "components/componentA/overlays/staging",
							},
						},
					},
				},
			}

			err = promotionRunReconciler.Create(ctx, binding)
			Expect(err).To(BeNil())

			// Trigger Reconciler
			_, err = promotionRunReconciler.Reconcile(ctx, request)
			Expect(err).To(BeNil())

		})
	})
})

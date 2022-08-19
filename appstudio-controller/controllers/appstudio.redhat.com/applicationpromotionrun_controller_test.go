package appstudioredhatcom

import (
	"context"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appstudiosharedv1 "github.com/redhat-appstudio/managed-gitops/appstudio-shared/apis/appstudio.redhat.com/v1alpha1"
	apibackend "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/tests"
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
				err := tests.GenericTestSetup()
			Expect(err).To(BeNil())

			err = appstudiosharedv1.AddToScheme(scheme)
			Expect(err).To(BeNil())

			// Create fake client
			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(apiNamespace, argocdNamespace, kubesystemNamespace).
				Build()

			// Create placeholder environment
			environment = appstudiosharedv1.Environment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "prod",
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

		It("Should fetch other PromotionRun CRs and ignore completed CRs.", func() {
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

		It("Should fetch other PromotionRun CRs and ignore if Spec.Application is different.", func() {
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

		It("Should return error if another PromotionRun CR is available pointing to same Application.", func() {
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

			promotionRun.CreationTimestamp = metav1.NewTime(time.Now().Add(-time.Minute * 5))
			err = promotionRunReconciler.Create(ctx, promotionRun)
			Expect(err).To(BeNil())

			// Trigger Reconciler
			_, err = promotionRunReconciler.Reconcile(ctx, request)
			Expect(err).To(BeNil())

			err = promotionRunReconciler.Client.Get(ctx, client.ObjectKeyFromObject(promotionRun), promotionRun)
			Expect(err).To(BeNil())
			Expect(len(promotionRun.Status.Conditions) > 0)
			Expect(promotionRun.Status.Conditions[0].Type).To(Equal(appstudiosharedv1.PromotionRunConditionErrorOccurred))
			Expect(promotionRun.Status.Conditions[0].Message).To(Equal("Error occureed while checking for existing active promotions."))
			Expect(promotionRun.Status.Conditions[0].Status).To(Equal(appstudiosharedv1.PromotionRunConditionStatusFalse))
			Expect(promotionRun.Status.Conditions[0].Reason).To(Equal(appstudiosharedv1.PromotionRunReasonErrorOccurred))
		})

		It("Should not support auto promo.", func() {
			promotionRun.Spec.AutomatedPromotion.InitialEnvironment = "abc"
			err := promotionRunReconciler.Create(ctx, promotionRun)
			Expect(err).To(BeNil())

			// Trigger Reconciler
			_, err = promotionRunReconciler.Reconcile(ctx, request)
			Expect(err).To(BeNil())

			err = promotionRunReconciler.Client.Get(ctx, client.ObjectKeyFromObject(promotionRun), promotionRun)
			Expect(err).To(BeNil())
			Expect(len(promotionRun.Status.Conditions) > 0)
			Expect(promotionRun.Status.Conditions[0].Type).To(Equal(appstudiosharedv1.PromotionRunConditionErrorOccurred))
			Expect(promotionRun.Status.Conditions[0].Message).To(Equal("Automated promotion are not yet supported."))
			Expect(promotionRun.Status.Conditions[0].Status).To(Equal(appstudiosharedv1.PromotionRunConditionStatusFalse))
			Expect(promotionRun.Status.Conditions[0].Reason).To(Equal(appstudiosharedv1.PromotionRunReasonErrorOccurred))
		})

		It("Should not support invalid value for Target Environment", func() {
			promotionRun.Spec.ManualPromotion.TargetEnvironment = ""
			err := promotionRunReconciler.Create(ctx, promotionRun)
			Expect(err).To(BeNil())

			// Trigger Reconciler
			_, err = promotionRunReconciler.Reconcile(ctx, request)
			Expect(err).To(BeNil())

			err = promotionRunReconciler.Client.Get(ctx, client.ObjectKeyFromObject(promotionRun), promotionRun)
			Expect(err).To(BeNil())
			Expect(len(promotionRun.Status.Conditions) > 0)
			Expect(promotionRun.Status.Conditions[0].Type).To(Equal(appstudiosharedv1.PromotionRunConditionErrorOccurred))
			Expect(promotionRun.Status.Conditions[0].Message).To(Equal("Target Environment has invalid value."))
			Expect(promotionRun.Status.Conditions[0].Status).To(Equal(appstudiosharedv1.PromotionRunConditionStatusFalse))
			Expect(promotionRun.Status.Conditions[0].Reason).To(Equal(appstudiosharedv1.PromotionRunReasonErrorOccurred))
		})

		It("Should return error if binding for application not present.", func() {
			err := promotionRunReconciler.Create(ctx, promotionRun)
			Expect(err).To(BeNil())

			// Trigger Reconciler
			_, err = promotionRunReconciler.Reconcile(ctx, request)
			Expect(err).To(BeNil())

		})

		It("Should fetch the Binding given in PromotionRun CR.", func() {
			applicationSnapshot := appstudiosharedv1.ApplicationSnapshot{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "appstudio.redhat.com/v1alpha1",
					Kind:       "ApplicationSnapshot",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      promotionRun.Spec.Snapshot,
					Namespace: promotionRun.Namespace,
				},
				Spec: appstudiosharedv1.ApplicationSnapshotSpec{
					Application: promotionRun.Spec.Application,
					DisplayName: promotionRun.Spec.Application,
				},
			}

			err := promotionRunReconciler.Create(ctx, &applicationSnapshot)
			Expect(err).To(BeNil())

			binding := &appstudiosharedv1.ApplicationSnapshotEnvironmentBinding{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "appstudio.redhat.com/v1alpha1",
					Kind:       "ApplicationSnapshotEnvironmentBinding",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "appa-staging-binding",
					Namespace: promotionRun.Namespace,
				},
				Spec: appstudiosharedv1.ApplicationSnapshotEnvironmentBindingSpec{
					Application: promotionRun.Spec.Application,
					Environment: promotionRun.Spec.ManualPromotion.TargetEnvironment,
					Snapshot:    promotionRun.Spec.Snapshot,
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
			}

			err = promotionRunReconciler.Create(ctx, binding)
			Expect(err).To(BeNil())

			err = promotionRunReconciler.Create(ctx, promotionRun)
			Expect(err).To(BeNil())

			// Trigger Reconciler
			_, err = promotionRunReconciler.Reconcile(ctx, request)
			Expect(err).To(BeNil())

			err = promotionRunReconciler.Client.Get(ctx, client.ObjectKeyFromObject(promotionRun), promotionRun)
			Expect(err).To(BeNil())
			Expect(promotionRun.Status.State).To(Equal(appstudiosharedv1.PromotionRunState_Active))
			Expect(promotionRun.Status.State).To(Equal(appstudiosharedv1.PromotionRunState_Active))
			Expect(len(promotionRun.Status.ActiveBindings)).To(Equal(1))
			Expect(promotionRun.Status.ActiveBindings[0]).To(Equal(binding.Name))
		})

		It("Should return error if PromotionRun.Status.ActiveBindings do not match with Binding given in PromotionRun CR.", func() {
			applicationSnapshot := appstudiosharedv1.ApplicationSnapshot{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "appstudio.redhat.com/v1alpha1",
					Kind:       "ApplicationSnapshot",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      promotionRun.Spec.Snapshot,
					Namespace: promotionRun.Namespace,
				},
				Spec: appstudiosharedv1.ApplicationSnapshotSpec{
					Application: promotionRun.Spec.Application,
					DisplayName: promotionRun.Spec.Application,
				},
			}

			err := promotionRunReconciler.Create(ctx, &applicationSnapshot)
			Expect(err).To(BeNil())

			binding := &appstudiosharedv1.ApplicationSnapshotEnvironmentBinding{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "appstudio.redhat.com/v1alpha1",
					Kind:       "ApplicationSnapshotEnvironmentBinding",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "appa-staging-binding",
					Namespace: promotionRun.Namespace,
				},
				Spec: appstudiosharedv1.ApplicationSnapshotEnvironmentBindingSpec{
					Application: promotionRun.Spec.Application,
					Environment: promotionRun.Spec.ManualPromotion.TargetEnvironment,
					Snapshot:    promotionRun.Spec.Snapshot,
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
			}

			err = promotionRunReconciler.Create(ctx, binding)
			Expect(err).To(BeNil())

			promotionRun.Status.ActiveBindings = []string{"binding1"}

			err = promotionRunReconciler.Create(ctx, promotionRun)
			Expect(err).To(BeNil())

			// Trigger Reconciler
			_, err = promotionRunReconciler.Reconcile(ctx, request)
			Expect(err).To(BeNil())

			err = promotionRunReconciler.Client.Get(ctx, client.ObjectKeyFromObject(promotionRun), promotionRun)
			Expect(err).To(BeNil())
			Expect(promotionRun.Status.State).To(Equal(appstudiosharedv1.PromotionRunState_Active))

			Expect(promotionRun.Status.Conditions[0].Type).To(Equal(appstudiosharedv1.PromotionRunConditionErrorOccurred))
			Expect(promotionRun.Status.Conditions[0].Message).To(Equal("The binding changed after the PromotionRun first start. The .spec fields of the PromotionRun are immutable, and should not be changed after being created. old-binding: binding1, new-binding: appa-staging-binding"))
			Expect(promotionRun.Status.Conditions[0].Status).To(Equal(appstudiosharedv1.PromotionRunConditionStatusFalse))
			Expect(promotionRun.Status.Conditions[0].Reason).To(Equal(appstudiosharedv1.PromotionRunReasonErrorOccurred))
		})

		It("Should return error if SnapShot given in PromotionRun CR does not exist.", func() {
			binding := &appstudiosharedv1.ApplicationSnapshotEnvironmentBinding{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "appstudio.redhat.com/v1alpha1",
					Kind:       "ApplicationSnapshotEnvironmentBinding",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "appa-staging-binding",
					Namespace: promotionRun.Namespace,
				},
				Spec: appstudiosharedv1.ApplicationSnapshotEnvironmentBindingSpec{
					Application: promotionRun.Spec.Application,
					Environment: promotionRun.Spec.ManualPromotion.TargetEnvironment,
					Snapshot:    promotionRun.Spec.Snapshot,
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
			}

			err := promotionRunReconciler.Create(ctx, binding)
			Expect(err).To(BeNil())

			err = promotionRunReconciler.Create(ctx, promotionRun)
			Expect(err).To(BeNil())

			// Trigger Reconciler
			_, err = promotionRunReconciler.Reconcile(ctx, request)
			Expect(err).NotTo(BeNil())
			Expect(err.Error()).To(Equal("Snapshot refered in Binding does not exist."))
		})

		It("Should wait for the environment binding to create all of the expected GitOpsDeployments.", func() {
			applicationSnapshot := appstudiosharedv1.ApplicationSnapshot{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "appstudio.redhat.com/v1alpha1",
					Kind:       "ApplicationSnapshot",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      promotionRun.Spec.Snapshot,
					Namespace: promotionRun.Namespace,
				},
				Spec: appstudiosharedv1.ApplicationSnapshotSpec{
					Application: promotionRun.Spec.Application,
					DisplayName: promotionRun.Spec.Application,
				},
			}

			err := promotionRunReconciler.Create(ctx, &applicationSnapshot)
			Expect(err).To(BeNil())

			binding := &appstudiosharedv1.ApplicationSnapshotEnvironmentBinding{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "appstudio.redhat.com/v1alpha1",
					Kind:       "ApplicationSnapshotEnvironmentBinding",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "appa-staging-binding",
					Namespace: promotionRun.Namespace,
				},
				Spec: appstudiosharedv1.ApplicationSnapshotEnvironmentBindingSpec{
					Application: promotionRun.Spec.Application,
					Environment: promotionRun.Spec.ManualPromotion.TargetEnvironment,
					Snapshot:    promotionRun.Spec.Snapshot,
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
			}

			err = promotionRunReconciler.Create(ctx, binding)
			Expect(err).To(BeNil())

			promotionRun.Status.ActiveBindings = []string{binding.Name}
			err = promotionRunReconciler.Create(ctx, promotionRun)
			Expect(err).To(BeNil())

			// Trigger Reconciler
			_, err = promotionRunReconciler.Reconcile(ctx, request)
			Expect(err).To(BeNil())

			err = promotionRunReconciler.Client.Get(ctx, client.ObjectKeyFromObject(promotionRun), promotionRun)
			Expect(err).To(BeNil())
			Expect(len(promotionRun.Status.EnvironmentStatus) > 0).To(BeTrue())
			Expect(promotionRun.Status.EnvironmentStatus[0].Step).To(Equal(1))
			Expect(promotionRun.Status.EnvironmentStatus[0].DisplayStatus).To(Equal("Waiting for the environment binding to create all of the expected GitOpsDeployments."))
			Expect(promotionRun.Status.EnvironmentStatus[0].Status).To(Equal(appstudiosharedv1.ApplicationPromotionRunEnvironmentStatus_InProgress))
			Expect(promotionRun.Status.EnvironmentStatus[0].EnvironmentName).To(Equal(environment.Name))
		})

		It("Should waiting for GitOpsDeployments to have expected commit/sync/health: Scenario 2.", func() {
			applicationSnapshot := appstudiosharedv1.ApplicationSnapshot{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "appstudio.redhat.com/v1alpha1",
					Kind:       "ApplicationSnapshot",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      promotionRun.Spec.Snapshot,
					Namespace: promotionRun.Namespace,
				},
				Spec: appstudiosharedv1.ApplicationSnapshotSpec{
					Application: promotionRun.Spec.Application,
					DisplayName: promotionRun.Spec.Application,
				},
			}

			err := promotionRunReconciler.Create(ctx, &applicationSnapshot)
			Expect(err).To(BeNil())

			binding := &appstudiosharedv1.ApplicationSnapshotEnvironmentBinding{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "appstudio.redhat.com/v1alpha1",
					Kind:       "ApplicationSnapshotEnvironmentBinding",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "appa-staging-binding",
					Namespace: promotionRun.Namespace,
				},
				Spec: appstudiosharedv1.ApplicationSnapshotEnvironmentBindingSpec{
					Application: promotionRun.Spec.Application,
					Environment: promotionRun.Spec.ManualPromotion.TargetEnvironment,
					Snapshot:    promotionRun.Spec.Snapshot,
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
					GitOpsDeployments: []appstudiosharedv1.BindingStatusGitOpsDeployment{
						{
							ComponentName:    "component-a",
							GitOpsDeployment: "appa-staging-binding-" + promotionRun.Spec.Application + "-" + promotionRun.Spec.ManualPromotion.TargetEnvironment + "-component-a",
						},
					},
				},
			}

			err = promotionRunReconciler.Create(ctx, binding)
			Expect(err).To(BeNil())

			gitOpsDeployment := &apibackend.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "appa-staging-binding-" + promotionRun.Spec.Application + "-" + promotionRun.Spec.ManualPromotion.TargetEnvironment + "-component-a",
					Namespace: binding.Namespace,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: binding.APIVersion,
							Kind:       binding.Kind,
							Name:       binding.Name,
							UID:        binding.UID,
						},
					},
				},
				Spec: apibackend.GitOpsDeploymentSpec{
					Source: apibackend.ApplicationSource{
						RepoURL:        "https://github.com/redhat-appstudio/gitops-repository-template",
						Path:           "components/componentA/overlays/staging",
						TargetRevision: "main",
					},
					Type:        apibackend.GitOpsDeploymentSpecType_Automated, // Default to automated, for now
					Destination: apibackend.ApplicationDestination{},           // Default to same namespace, for now
				},
				Status: apibackend.GitOpsDeploymentStatus{
					Sync: apibackend.SyncStatus{
						Status: apibackend.SyncStatusCodeSynced,
					},
				},
			}

			err = promotionRunReconciler.Create(ctx, gitOpsDeployment)
			Expect(err).To(BeNil())

			promotionRun.Status.ActiveBindings = []string{binding.Name}
			err = promotionRunReconciler.Create(ctx, promotionRun)
			Expect(err).To(BeNil())

			// Trigger Reconciler
			_, err = promotionRunReconciler.Reconcile(ctx, request)
			Expect(err).To(BeNil())

			err = promotionRunReconciler.Client.Get(ctx, client.ObjectKeyFromObject(promotionRun), promotionRun)
			Expect(err).To(BeNil())
			Expect(len(promotionRun.Status.EnvironmentStatus) > 0).To(BeTrue())
			Expect(promotionRun.Status.EnvironmentStatus[0].Step).To(Equal(1))
			Expect(promotionRun.Status.EnvironmentStatus[0].DisplayStatus).To(Equal("Waiting for following GitOpsDeployments to be Synced/Healthy: " + gitOpsDeployment.Name))
			Expect(promotionRun.Status.EnvironmentStatus[0].Status).To(Equal(appstudiosharedv1.ApplicationPromotionRunEnvironmentStatus_InProgress))
			Expect(promotionRun.Status.EnvironmentStatus[0].EnvironmentName).To(Equal(environment.Name))
		})

		It("Should 1.", func() {
			applicationSnapshot := appstudiosharedv1.ApplicationSnapshot{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "appstudio.redhat.com/v1alpha1",
					Kind:       "ApplicationSnapshot",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      promotionRun.Spec.Snapshot,
					Namespace: promotionRun.Namespace,
				},
				Spec: appstudiosharedv1.ApplicationSnapshotSpec{
					Application: promotionRun.Spec.Application,
					DisplayName: promotionRun.Spec.Application,
				},
			}

			err := promotionRunReconciler.Create(ctx, &applicationSnapshot)
			Expect(err).To(BeNil())

			binding := &appstudiosharedv1.ApplicationSnapshotEnvironmentBinding{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "appstudio.redhat.com/v1alpha1",
					Kind:       "ApplicationSnapshotEnvironmentBinding",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "appa-staging-binding",
					Namespace: promotionRun.Namespace,
				},
				Spec: appstudiosharedv1.ApplicationSnapshotEnvironmentBindingSpec{
					Application: promotionRun.Spec.Application,
					Environment: promotionRun.Spec.ManualPromotion.TargetEnvironment,
					Snapshot:    promotionRun.Spec.Snapshot,
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
					GitOpsDeployments: []appstudiosharedv1.BindingStatusGitOpsDeployment{
						{
							ComponentName:    "component-a",
							GitOpsDeployment: "appa-staging-binding-" + promotionRun.Spec.Application + "-" + promotionRun.Spec.ManualPromotion.TargetEnvironment + "-component-a",
						},
					},
				},
			}

			err = promotionRunReconciler.Create(ctx, binding)
			Expect(err).To(BeNil())

			gitOpsDeployment := &apibackend.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "appa-staging-binding-" + promotionRun.Spec.Application + "-" + promotionRun.Spec.ManualPromotion.TargetEnvironment + "-component-a",
					Namespace: binding.Namespace,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: binding.APIVersion,
							Kind:       binding.Kind,
							Name:       binding.Name,
							UID:        binding.UID,
						},
					},
				},
				Spec: apibackend.GitOpsDeploymentSpec{
					Source: apibackend.ApplicationSource{
						RepoURL:        "https://github.com/redhat-appstudio/gitops-repository-template",
						Path:           "components/componentA/overlays/staging",
						TargetRevision: "main",
					},
					Type:        apibackend.GitOpsDeploymentSpecType_Automated, // Default to automated, for now
					Destination: apibackend.ApplicationDestination{},           // Default to same namespace, for now
				},
				Status: apibackend.GitOpsDeploymentStatus{
					Sync: apibackend.SyncStatus{
						Status:   apibackend.SyncStatusCodeSynced,
						Revision: "main",
					},
					Health: apibackend.HealthStatus{
						Status: apibackend.HeathStatusCodeHealthy,
					},
				},
			}

			err = promotionRunReconciler.Create(ctx, gitOpsDeployment)
			Expect(err).To(BeNil())

			promotionRun.Status.ActiveBindings = []string{binding.Name}
			err = promotionRunReconciler.Create(ctx, promotionRun)
			Expect(err).To(BeNil())

			// Trigger Reconciler
			_, err = promotionRunReconciler.Reconcile(ctx, request)
			Expect(err).To(BeNil())

			err = promotionRunReconciler.Client.Get(ctx, client.ObjectKeyFromObject(promotionRun), promotionRun)
			Expect(err).To(BeNil())
			Expect(len(promotionRun.Status.EnvironmentStatus) > 0).To(BeTrue())
			Expect(promotionRun.Status.EnvironmentStatus[0].Step).To(Equal(1))
			Expect(promotionRun.Status.EnvironmentStatus[0].DisplayStatus).To(Equal("All GitOpsDeployments are Synced/Healthy"))
			Expect(promotionRun.Status.EnvironmentStatus[0].Status).To(Equal(appstudiosharedv1.ApplicationPromotionRunEnvironmentStatus_Success))
			Expect(promotionRun.Status.EnvironmentStatus[0].EnvironmentName).To(Equal(environment.Name))
		})

		It("Should create GitOpsDeployments and it should be Synced/Healthy.", func() {
			applicationSnapshot := appstudiosharedv1.ApplicationSnapshot{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "appstudio.redhat.com/v1alpha1",
					Kind:       "ApplicationSnapshot",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      promotionRun.Spec.Snapshot,
					Namespace: promotionRun.Namespace,
				},
				Spec: appstudiosharedv1.ApplicationSnapshotSpec{
					Application: promotionRun.Spec.Application,
					DisplayName: promotionRun.Spec.Application,
				},
			}

			err := promotionRunReconciler.Create(ctx, &applicationSnapshot)
			Expect(err).To(BeNil())

			binding := &appstudiosharedv1.ApplicationSnapshotEnvironmentBinding{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "appstudio.redhat.com/v1alpha1",
					Kind:       "ApplicationSnapshotEnvironmentBinding",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "appa-staging-binding",
					Namespace: promotionRun.Namespace,
				},
				Spec: appstudiosharedv1.ApplicationSnapshotEnvironmentBindingSpec{
					Application: promotionRun.Spec.Application,
					Environment: promotionRun.Spec.ManualPromotion.TargetEnvironment,
					Snapshot:    promotionRun.Spec.Snapshot,
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
					GitOpsDeployments: []appstudiosharedv1.BindingStatusGitOpsDeployment{
						{
							ComponentName:    "component-a",
							GitOpsDeployment: "appa-staging-binding-" + promotionRun.Spec.Application + "-" + promotionRun.Spec.ManualPromotion.TargetEnvironment + "-component-a",
						},
					},
				},
			}

			err = promotionRunReconciler.Create(ctx, binding)
			Expect(err).To(BeNil())

			gitOpsDeployment := &apibackend.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "appa-staging-binding-" + promotionRun.Spec.Application + "-" + promotionRun.Spec.ManualPromotion.TargetEnvironment + "-component-a",
					Namespace: binding.Namespace,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: binding.APIVersion,
							Kind:       binding.Kind,
							Name:       binding.Name,
							UID:        binding.UID,
						},
					},
				},
				Spec: apibackend.GitOpsDeploymentSpec{
					Source: apibackend.ApplicationSource{
						RepoURL:        "https://github.com/redhat-appstudio/gitops-repository-template",
						Path:           "components/componentA/overlays/staging",
						TargetRevision: "main",
					},
					Type:        apibackend.GitOpsDeploymentSpecType_Automated, // Default to automated, for now
					Destination: apibackend.ApplicationDestination{},           // Default to same namespace, for now
				},
			}

			err = promotionRunReconciler.Create(ctx, gitOpsDeployment)
			Expect(err).To(BeNil())

			promotionRun.Status.ActiveBindings = []string{binding.Name}
			err = promotionRunReconciler.Create(ctx, promotionRun)
			Expect(err).To(BeNil())

			// Trigger Reconciler
			_, err = promotionRunReconciler.Reconcile(ctx, request)
			Expect(err).To(BeNil())

			err = promotionRunReconciler.Client.Get(ctx, client.ObjectKeyFromObject(promotionRun), promotionRun)
			Expect(err).To(BeNil())
			Expect(len(promotionRun.Status.EnvironmentStatus) > 0).To(BeTrue())
			Expect(promotionRun.Status.EnvironmentStatus[0].Step).To(Equal(1))
			Expect(promotionRun.Status.EnvironmentStatus[0].DisplayStatus).To(Equal("All GitOpsDeployments are Synced/Healthy"))
			Expect(promotionRun.Status.EnvironmentStatus[0].Status).To(Equal(appstudiosharedv1.ApplicationPromotionRunEnvironmentStatus_Success))
			Expect(promotionRun.Status.EnvironmentStatus[0].EnvironmentName).To(Equal(environment.Name))
		})

		It("Should fail if GitOpsDeployments are not Synced/Healthy in given time limit.", func() {
			applicationSnapshot := appstudiosharedv1.ApplicationSnapshot{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "appstudio.redhat.com/v1alpha1",
					Kind:       "ApplicationSnapshot",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      promotionRun.Spec.Snapshot,
					Namespace: promotionRun.Namespace,
				},
				Spec: appstudiosharedv1.ApplicationSnapshotSpec{
					Application: promotionRun.Spec.Application,
					DisplayName: promotionRun.Spec.Application,
				},
			}

			err := promotionRunReconciler.Create(ctx, &applicationSnapshot)
			Expect(err).To(BeNil())

			binding := &appstudiosharedv1.ApplicationSnapshotEnvironmentBinding{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "appstudio.redhat.com/v1alpha1",
					Kind:       "ApplicationSnapshotEnvironmentBinding",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "appa-staging-binding",
					Namespace: promotionRun.Namespace,
				},
				Spec: appstudiosharedv1.ApplicationSnapshotEnvironmentBindingSpec{
					Application: promotionRun.Spec.Application,
					Environment: promotionRun.Spec.ManualPromotion.TargetEnvironment,
					Snapshot:    promotionRun.Spec.Snapshot,
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
					GitOpsDeployments: []appstudiosharedv1.BindingStatusGitOpsDeployment{
						{
							ComponentName:    "component-a",
							GitOpsDeployment: "appa-staging-binding-" + promotionRun.Spec.Application + "-" + promotionRun.Spec.ManualPromotion.TargetEnvironment + "-component-a",
						},
					},
				},
			}

			err = promotionRunReconciler.Create(ctx, binding)
			Expect(err).To(BeNil())

			gitOpsDeployment := &apibackend.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "appa-staging-binding-" + promotionRun.Spec.Application + "-" + promotionRun.Spec.ManualPromotion.TargetEnvironment + "-component-a",
					Namespace: binding.Namespace,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: binding.APIVersion,
							Kind:       binding.Kind,
							Name:       binding.Name,
							UID:        binding.UID,
						},
					},
				},
				Spec: apibackend.GitOpsDeploymentSpec{
					Source: apibackend.ApplicationSource{
						RepoURL:        "https://github.com/redhat-appstudio/gitops-repository-template",
						Path:           "components/componentA/overlays/staging",
						TargetRevision: "main",
					},
					Type:        apibackend.GitOpsDeploymentSpecType_Automated, // Default to automated, for now
					Destination: apibackend.ApplicationDestination{},           // Default to same namespace, for now
				},
			}

			err = promotionRunReconciler.Create(ctx, gitOpsDeployment)
			Expect(err).To(BeNil())

			promotionRun.Status.ActiveBindings = []string{binding.Name}
			then := metav1.Now().Add(time.Duration(-(PromotionRunTimeOutLimit + 2)) * time.Minute)

			promotionRun.Status.PromotionStartTime = metav1.NewTime(then)
			err = promotionRunReconciler.Create(ctx, promotionRun)
			Expect(err).To(BeNil())

			// Trigger Reconciler
			_, err = promotionRunReconciler.Reconcile(ctx, request)
			Expect(err).To(BeNil())

			err = promotionRunReconciler.Client.Get(ctx, client.ObjectKeyFromObject(promotionRun), promotionRun)
			Expect(err).To(BeNil())
			Expect(len(promotionRun.Status.EnvironmentStatus) > 0).To(BeTrue())
			Expect(promotionRun.Status.EnvironmentStatus[0].Step).To(Equal(1))
			Expect(promotionRun.Status.EnvironmentStatus[0].DisplayStatus).To(Equal("Promotion Failed. Could not be completed in " + strconv.Itoa(PromotionRunTimeOutLimit) + " Minutes."))
			Expect(promotionRun.Status.EnvironmentStatus[0].Status).To(Equal(appstudiosharedv1.ApplicationPromotionRunEnvironmentStatus_Failed))
			Expect(promotionRun.Status.EnvironmentStatus[0].EnvironmentName).To(Equal(environment.Name))
		})
	})
})

// newRequest contains the information necessary to reconcile a Kubernetes object.
func newRequest(namespace, name string) reconcile.Request {
	return reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		},
	}
}

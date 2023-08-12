package argoprojio

import (
	"context"
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appv1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"github.com/argoproj/gitops-engine/pkg/sync/common"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/fauxargocd"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/operations"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/tests"
	"github.com/redhat-appstudio/managed-gitops/cluster-agent/controllers"
	"github.com/redhat-appstudio/managed-gitops/cluster-agent/controllers/argoproj.io/application_info_cache"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/db"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
	"sigs.k8s.io/yaml"
)

var _ = Describe("Application Controller", func() {
	Context("Application Controller Test", func() {
		const (
			name      = "my-application"
			namespace = "argocd"
			dbID      = controllers.ArgoCDApplicationDatabaseIDLabel
		)
		var ctx context.Context
		var managedEnvironment *db.ManagedEnvironment
		var gitopsEngineInstance *db.GitopsEngineInstance
		var dbQueries db.AllDatabaseQueries
		var guestbookApp *appv1.Application
		var reconciler ApplicationReconciler
		var err error

		BeforeEach(func() {
			scheme, argocdNamespace, kubesystemNamespace, workspace, err := tests.GenericTestSetup()
			Expect(err).ToNot(HaveOccurred())

			err = appv1.AddToScheme(scheme)
			Expect(err).ToNot(HaveOccurred())

			gitopsDepl := &managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-gitops-depl",
					Namespace: workspace.Name,
					UID:       uuid.NewUUID(),
				},
			}

			// Fake kube client.
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(gitopsDepl, workspace, argocdNamespace, kubesystemNamespace).Build()

			// Database Connection.
			dbQueries, err = db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).ToNot(HaveOccurred())

			err = db.SetupForTestingDBGinkgo()
			Expect(err).ToNot(HaveOccurred())

			_, managedEnvironment, _, gitopsEngineInstance, _, err = createSampleData(dbQueries)
			Expect(err).ToNot(HaveOccurred())

			guestbookApp = &appv1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
					Labels: map[string]string{
						dbID: "test-my-application",
					},
				},
				Spec: appv1.ApplicationSpec{
					Source: &appv1.ApplicationSource{
						Path:           "guestbook",
						TargetRevision: "HEAD",
						RepoURL:        "https://github.com/argoproj/argocd-example-apps.git",
					},
					Destination: appv1.ApplicationDestination{
						Namespace: "guestbook",
						Server:    "https://kubernetes.default.svc",
					},
					Project: "default",
				},
				Status: appv1.ApplicationStatus{
					Health: appv1.HealthStatus{
						Status: "Healthy",
					},
					Sync: appv1.SyncStatus{
						Status: "Synced",
						ComparedTo: appv1.ComparedTo{
							Source: appv1.ApplicationSource{
								Path:           "test-path",
								RepoURL:        "test-url",
								TargetRevision: "test-branch",
							},
							Destination: appv1.ApplicationDestination{
								Namespace: "test-namespace",
								Name:      "managed-env-123",
							},
						},
					},
				},
			}

			app := appv1.ApplicationCondition{
				Type:               "SyncError",
				Message:            "Failed to sync",
				LastTransitionTime: &metav1.Time{Time: time.Now()},
			}
			guestbookApp.Status.Conditions = append(guestbookApp.Status.Conditions, app)

			reconciler = ApplicationReconciler{
				Client:                k8sClient,
				Scheme:                scheme,
				DB:                    dbQueries,
				DeletionTaskRetryLoop: sharedutil.NewTaskRetryLoop("application-reconciler"),
				Cache:                 application_info_cache.NewApplicationInfoCache(),
			}
		})

		AfterEach(func() {
			// Prevent race conditions between tests, by shutting down the cache after each test
			reconciler.Cache.DebugOnly_Shutdown(context.Background())
		})

		It("Create New Application CR in namespace and database and verify that ApplicationState is created", func() {
			By("Close database connection")
			defer dbQueries.CloseDatabase()
			defer testTeardown()

			ctx = context.Background()

			By("Add databaseID label to applicationID")
			databaseID := guestbookApp.Labels[dbID]
			applicationDB := &db.Application{
				Application_id:          databaseID,
				Name:                    name,
				Spec_field:              "{}",
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			}

			applicationState := &db.ApplicationState{
				Applicationstate_application_id: applicationDB.Application_id,
			}

			By("Create a new ArgoCD Application")
			err = reconciler.Create(ctx, guestbookApp)
			Expect(err).ToNot(HaveOccurred())

			By("Create Application in Database")
			err = reconciler.DB.CreateApplication(ctx, applicationDB)
			Expect(err).ToNot(HaveOccurred())

			By("Call reconcile function")
			result, err := reconciler.Reconcile(ctx, newRequest(namespace, name))
			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())

			By("Verify Application State is created in database")
			err = reconciler.DB.GetApplicationStateById(ctx, applicationState)
			Expect(err).ToNot(HaveOccurred())

			By("Verify that Health and Status of ArgoCD Application is equal to the Health and Status of Application in database")
			Expect(applicationState.Health).To(Equal(string(guestbookApp.Status.Health.Status)))
			Expect(applicationState.Sync_Status).To(Equal(string(guestbookApp.Status.Sync.Status)))

			By("verify that the OperationState is not populated since the Application's OperationState doesn't exist")
			Expect(applicationState.OperationState).To(BeNil())
		})

		It("Verify if the OperationState DB field is populated with Application CR's .status.OperationState", func() {
			By("Close database connection")
			defer dbQueries.CloseDatabase()
			defer testTeardown()

			ctx = context.Background()

			By("add a sample operation state")
			guestbookApp.Status.OperationState = &appv1.OperationState{
				Message:    "sample message",
				RetryCount: 10,
			}

			By("Add databaseID label to applicationID")
			databaseID := guestbookApp.Labels[dbID]
			applicationDB := &db.Application{
				Application_id:          databaseID,
				Name:                    name,
				Spec_field:              "{}",
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			}

			applicationState := &db.ApplicationState{
				Applicationstate_application_id: applicationDB.Application_id,
			}

			By("Create a new ArgoCD Application")
			err = reconciler.Create(ctx, guestbookApp)
			Expect(err).ToNot(HaveOccurred())

			By("Create Application in Database")
			err = reconciler.DB.CreateApplication(ctx, applicationDB)
			Expect(err).ToNot(HaveOccurred())

			By("Call reconcile function")
			result, err := reconciler.Reconcile(ctx, newRequest(namespace, name))
			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())

			By("Verify Application State is created in database")
			err = reconciler.DB.GetApplicationStateById(ctx, applicationState)
			Expect(err).ToNot(HaveOccurred())

			By("Verify that Health and Status of ArgoCD Application is equal to the Health and Status of Application in database")
			Expect(applicationState.Health).To(Equal(string(guestbookApp.Status.Health.Status)))
			Expect(applicationState.Sync_Status).To(Equal(string(guestbookApp.Status.Sync.Status)))

			By("verify that the OperationState is populated in the ApplicationState")
			Expect(applicationState.OperationState).ToNot(BeNil())

			compareOpState := func(appState *db.ApplicationState, app *appv1.Application) {
				opStateBytes, err := sharedutil.DecompressObject(applicationState.OperationState)
				Expect(err).ToNot(HaveOccurred())
				Expect(opStateBytes).ToNot(BeNil())

				opState := &managedgitopsv1alpha1.OperationState{}
				err = yaml.Unmarshal(opStateBytes, opState)
				Expect(err).ToNot(HaveOccurred())

				Expect(opState.Message).To(Equal(guestbookApp.Status.OperationState.Message))
				Expect(opState.RetryCount).To(Equal(guestbookApp.Status.OperationState.RetryCount))
			}

			compareOpState(applicationState, guestbookApp)

			By("remove .status.OperationState and verify if the ApplicationState is updated")
			guestbookApp.Status.OperationState = nil
			err = reconciler.Update(ctx, guestbookApp)
			Expect(err).ToNot(HaveOccurred())

			result, err = reconciler.Reconcile(ctx, newRequest(namespace, name))
			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())

			By("verify that the OperationState is updated in the ApplicationState")
			err = reconciler.DB.GetApplicationStateById(ctx, applicationState)
			Expect(err).ToNot(HaveOccurred())
			Expect(applicationState.OperationState).To(BeNil())

			By("add .status.OperationState and verify if the ApplicationState is updated")
			guestbookApp.Status.OperationState = &appv1.OperationState{
				Message:    "sample message",
				RetryCount: 10,
			}
			err = reconciler.Update(ctx, guestbookApp)
			Expect(err).ToNot(HaveOccurred())

			result, err = reconciler.Reconcile(ctx, newRequest(namespace, name))
			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())

			By("verify that the OperationState is updated in the ApplicationState")
			err = reconciler.DB.GetApplicationStateById(ctx, applicationState)
			Expect(err).ToNot(HaveOccurred())
			Expect(applicationState.OperationState).ToNot(BeNil())

			compareOpState(applicationState, guestbookApp)
		})

		It("Update an existing Application table in the database, call Reconcile on the Argo CD Application, and verify an existing ApplicationState DB entry is updated", func() {
			By("Close database connection")
			defer dbQueries.CloseDatabase()
			defer testTeardown()

			ctx = context.Background()

			By("Add databaseID label to applicationID")
			databaseID := guestbookApp.Labels[dbID]
			applicationDB := &db.Application{
				Application_id:          databaseID,
				Name:                    name,
				Spec_field:              "{}",
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			}

			reconciledStateString, _, err := dummyApplicationComparedToField()
			Expect(err).ToNot(HaveOccurred())

			applicationState := &db.ApplicationState{
				Applicationstate_application_id: applicationDB.Application_id,
				Health:                          "Progressing",
				Sync_Status:                     "OutOfSync",
				ReconciledState:                 reconciledStateString,
				Conditions:                      []byte("sample-condition"),
			}

			By("Create Application in Database")
			err = reconciler.DB.CreateApplication(ctx, applicationDB)
			Expect(err).ToNot(HaveOccurred())

			By("Create ApplicationState in Database")
			err = reconciler.DB.CreateApplicationState(ctx, applicationState)
			Expect(err).ToNot(HaveOccurred())

			By("Create a new ArgoCD Application")
			err = reconciler.Create(ctx, guestbookApp)
			Expect(err).ToNot(HaveOccurred())

			By("Call reconcile function")
			result, err := reconciler.Reconcile(ctx, newRequest(namespace, name))
			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())

			By("Verify Application State is created in database")
			err = reconciler.DB.GetApplicationStateById(ctx, applicationState)
			Expect(err).ToNot(HaveOccurred())

			By("Verify that Health and Status of ArgoCD Application is equal to the Health and Status of Application in database")
			Expect(applicationState.Health).To(Equal(string(guestbookApp.Status.Health.Status)))
			Expect(applicationState.Sync_Status).To(Equal(string(guestbookApp.Status.Sync.Status)))

		})

		It("Calls Reconcile on an Argo CD Application resource that doesn't exist", func() {
			By("Close database connection")
			defer dbQueries.CloseDatabase()
			defer testTeardown()

			ctx = context.Background()

			By("Add databaseID label to applicationID")
			databaseID := guestbookApp.Labels[dbID]
			applicationDB := &db.Application{
				Application_id:          databaseID,
				Name:                    name,
				Spec_field:              "{}",
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			}

			By("Create Application in Database")
			err = reconciler.DB.CreateApplication(ctx, applicationDB)
			Expect(err).ToNot(HaveOccurred())

			By("Call reconcile function")
			result, err := reconciler.Reconcile(ctx, newRequest(namespace, name))
			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())

		})

		It("Has an Argo CD Application that points to a database entry that doesn't exist", func() {
			By("Close database connection")
			defer dbQueries.CloseDatabase()
			defer testTeardown()
			Expect(err).ToNot(HaveOccurred())

			ctx = context.Background()

			guestbookApp = &appv1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
					Labels: map[string]string{
						dbID: "test-wrong-input",
					},
				},
				Spec: appv1.ApplicationSpec{
					Source: &appv1.ApplicationSource{
						Path:           "guestbook",
						TargetRevision: "HEAD",
						RepoURL:        "https://github.com/argoproj/argocd-example-apps.git",
					},
					Destination: appv1.ApplicationDestination{
						Namespace: "guestbook",
						Server:    "https://kubernetes.default.svc",
					},
					Project: "default",
				},
				Status: appv1.ApplicationStatus{
					Health: appv1.HealthStatus{
						Status: "InProgress",
					},
					Sync: appv1.SyncStatus{
						Status: "OutOfSync",
					},
				},
			}

			By("Add databaseID label to applicationID")
			databaseID := guestbookApp.Labels[dbID]
			applicationDB := &db.Application{
				Application_id:          databaseID,
				Name:                    name,
				Spec_field:              "{}",
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			}

			By("Verify Application is not present in database")
			err = reconciler.DB.GetApplicationById(ctx, applicationDB)
			Expect(err).To(HaveOccurred())

			By("Create a new ArgoCD Application")
			err = reconciler.Create(ctx, guestbookApp)
			Expect(err).ToNot(HaveOccurred())

			By("Call reconcile function")
			result, err := reconciler.Reconcile(ctx, newRequest(namespace, name))
			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())

		})

		It("Delete the Argo CD Application resource, and call Reconcile. Assert that no error is returned", func() {
			By("Close database connection")
			defer dbQueries.CloseDatabase()
			defer testTeardown()

			ctx = context.Background()

			By("Add databaseID label to applicationID")
			databaseID := guestbookApp.Labels[dbID]
			applicationDB := &db.Application{
				Application_id:          databaseID,
				Name:                    name,
				Spec_field:              "{}",
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			}

			applicationState := &db.ApplicationState{
				Applicationstate_application_id: applicationDB.Application_id,
			}

			By("Create a new ArgoCD Application")
			err = reconciler.Create(ctx, guestbookApp)
			Expect(err).ToNot(HaveOccurred())

			By("Create Application in Database")
			err = reconciler.DB.CreateApplication(ctx, applicationDB)
			Expect(err).ToNot(HaveOccurred())

			By("Call reconcile function")
			result, err := reconciler.Reconcile(ctx, newRequest(namespace, name))
			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())

			By("Verify Application State is created in database")
			err = reconciler.DB.GetApplicationStateById(ctx, applicationState)
			Expect(err).ToNot(HaveOccurred())

			By("Delete ArgoCD Application")
			err = reconciler.Delete(ctx, guestbookApp)
			Expect(err).ToNot(HaveOccurred())

			By("Call reconcile function")
			_, err = reconciler.Reconcile(ctx, newRequest(namespace, name))
			Expect(err).ToNot(HaveOccurred())

		})

		It("Health and Sync Status of Application CR is empty, sanitize Health and sync status", func() {
			By("Close database connection")
			defer dbQueries.CloseDatabase()
			defer testTeardown()

			ctx = context.Background()

			guestbookApp = &appv1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
					Labels: map[string]string{
						dbID: "test-my-application",
					},
				},
				Spec: appv1.ApplicationSpec{
					Source: &appv1.ApplicationSource{
						Path:           "guestbook",
						TargetRevision: "HEAD",
						RepoURL:        "https://github.com/argoproj/argocd-example-apps.git",
					},
					Destination: appv1.ApplicationDestination{
						Namespace: "guestbook",
						Server:    "https://kubernetes.default.svc",
					},
					Project: "default",
				},
				Status: appv1.ApplicationStatus{
					Sync: appv1.SyncStatus{
						ComparedTo: appv1.ComparedTo{
							Source: appv1.ApplicationSource{
								Path:           "test-path",
								RepoURL:        "test-url",
								TargetRevision: "test-branch",
							},
							Destination: appv1.ApplicationDestination{
								Namespace: "test-namespace",
								Name:      "managed-env-123",
							},
						},
					},
				},
			}

			By("Add databaseID label to applicationID")
			databaseID := guestbookApp.Labels[dbID]
			applicationDB := &db.Application{
				Application_id:          databaseID,
				Name:                    name,
				Spec_field:              "{}",
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			}

			By("Create a new ArgoCD Application")
			err = reconciler.Create(ctx, guestbookApp)
			Expect(err).ToNot(HaveOccurred())

			By("Create Application in Database")
			err = reconciler.DB.CreateApplication(ctx, applicationDB)
			Expect(err).ToNot(HaveOccurred())

			By("Call reconcile function")
			result, err := reconciler.Reconcile(ctx, newRequest(namespace, name))
			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())

		})

		It("Test to check whether reconciled state is stored in database", func() {
			By("Close database connection")
			defer dbQueries.CloseDatabase()
			defer testTeardown()
			Expect(err).ToNot(HaveOccurred())

			ctx = context.Background()

			By("Add databaseID label to applicationID")
			databaseID := guestbookApp.Labels[dbID]
			applicationDB := &db.Application{
				Application_id:          databaseID,
				Name:                    name,
				Spec_field:              "{}",
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			}

			applicationStateDB := &db.ApplicationState{
				Applicationstate_application_id: applicationDB.Application_id,
			}

			By("Create Application in Database")
			err = reconciler.DB.CreateApplication(ctx, applicationDB)
			Expect(err).ToNot(HaveOccurred())

			By("Verify ApplicationState is not present in database")
			err = reconciler.DB.GetApplicationStateById(ctx, applicationStateDB)
			Expect(err).To(HaveOccurred())
			Expect(db.IsResultNotFoundError(err)).To(BeTrue())

			By("Create a new ArgoCD Application")
			err = reconciler.Create(ctx, guestbookApp)
			Expect(err).ToNot(HaveOccurred())

			By("Call reconcile function")
			result, err := reconciler.Reconcile(ctx, newRequest(namespace, name))
			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())

			err = reconciler.DB.GetApplicationStateById(ctx, applicationStateDB)
			Expect(applicationStateDB.ReconciledState).ToNot(BeNil())
			Expect(err).ToNot(HaveOccurred())

			By("Verify ReconciledState is stored in db")
			comparedTo := fauxargocd.FauxComparedTo{}

			err = json.Unmarshal([]byte(applicationStateDB.ReconciledState), &comparedTo)
			Expect(err).ToNot(HaveOccurred())

			reconciledStateObj, err := convertReconciledStateStringToObject(applicationStateDB.ReconciledState)
			Expect(err).ToNot(HaveOccurred())

			// Checking for new values in database object
			Expect(reconciledStateObj.Source.Path).To(Equal(guestbookApp.Status.Sync.ComparedTo.Source.Path))
			Expect(reconciledStateObj.Source.RepoURL).To(Equal(guestbookApp.Status.Sync.ComparedTo.Source.RepoURL))
			Expect(reconciledStateObj.Source.TargetRevision).To(Equal(guestbookApp.Status.Sync.ComparedTo.Source.TargetRevision))
			Expect(reconciledStateObj.Destination.Namespace).To(Equal(guestbookApp.Status.Sync.ComparedTo.Destination.Namespace))

		})

		It("Test to check whether existing value in reconciled state updated to new value when comparedTo field of ArgoCD Application changes", func() {
			By("Close database connection")
			defer dbQueries.CloseDatabase()
			defer testTeardown()

			ctx = context.Background()

			By("Add databaseID label to applicationID")
			databaseID := guestbookApp.Labels[dbID]
			applicationDB := &db.Application{
				Application_id:          databaseID,
				Name:                    name,
				Spec_field:              "{}",
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			}

			reconciledStateString, reconciledObj, err := dummyApplicationComparedToField()
			Expect(reconciledObj.Destination.Namespace).ToNot(Equal(guestbookApp.Status.Sync.ComparedTo.Destination.Namespace))
			Expect(err).ToNot(HaveOccurred())

			// applicationState which already exists in database
			applicationState := &db.ApplicationState{
				Applicationstate_application_id: applicationDB.Application_id,
				Health:                          "Healthy",
				Sync_Status:                     "Synced",
				ReconciledState:                 reconciledStateString,
			}

			By("Create a new ArgoCD Application")
			err = reconciler.Create(ctx, guestbookApp)
			Expect(err).ToNot(HaveOccurred())

			By("Create Application in Database")
			err = reconciler.DB.CreateApplication(ctx, applicationDB)
			Expect(err).ToNot(HaveOccurred())

			By("Create ApplicationState in Database")
			err = reconciler.DB.CreateApplicationState(ctx, applicationState)
			Expect(err).ToNot(HaveOccurred())

			By("Call reconcile function")
			result, err := reconciler.Reconcile(ctx, newRequest(namespace, name))
			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())

			applicationStateget := &db.ApplicationState{
				Applicationstate_application_id: applicationDB.Application_id,
			}

			By("Verify ReconcidedState in ApplicationState is updated in database")
			err = reconciler.DB.GetApplicationStateById(ctx, applicationStateget)
			Expect(err).ToNot(HaveOccurred())

			reconciledStateObj, err := convertReconciledStateStringToObject(applicationStateget.ReconciledState)
			Expect(err).ToNot(HaveOccurred())

			// Checking for new values in database object
			Expect(reconciledStateObj.Source.Path).To(Equal(guestbookApp.Status.Sync.ComparedTo.Source.Path))
			Expect(reconciledStateObj.Source.RepoURL).To(Equal(guestbookApp.Status.Sync.ComparedTo.Source.RepoURL))
			Expect(reconciledStateObj.Source.TargetRevision).To(Equal(guestbookApp.Status.Sync.ComparedTo.Source.TargetRevision))
			Expect(reconciledStateObj.Destination.Namespace).To(Equal(guestbookApp.Status.Sync.ComparedTo.Destination.Namespace))
		})

		It("Test to check whether conditions field is stored in database", func() {
			By("Close database connection")
			defer dbQueries.CloseDatabase()
			defer testTeardown()
			Expect(err).ToNot(HaveOccurred())

			ctx = context.Background()

			appConditions := []appv1.ApplicationCondition{
				{
					Type:               appv1.ApplicationConditionSyncError,
					Message:            "Failed to sync",
					LastTransitionTime: &metav1.Time{Time: time.Now()},
				},
				{
					Type:               appv1.ApplicationConditionOrphanedResourceWarning,
					Message:            "orphaned resource warning",
					LastTransitionTime: &metav1.Time{Time: time.Now()},
				},
			}
			guestbookApp.Status.Conditions = append(guestbookApp.Status.Conditions, appConditions...)

			By("Add databaseID label to applicationID")
			databaseID := guestbookApp.Labels[dbID]
			applicationDB := &db.Application{
				Application_id:          databaseID,
				Name:                    name,
				Spec_field:              "{}",
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			}

			applicationStateDB := &db.ApplicationState{
				Applicationstate_application_id: applicationDB.Application_id,
			}

			By("Create Application in Database")
			err = reconciler.DB.CreateApplication(ctx, applicationDB)
			Expect(err).ToNot(HaveOccurred())

			By("GetApplicationStateById to verify SyncError field is empty in database")
			err = reconciler.DB.GetApplicationStateById(ctx, applicationStateDB)
			Expect(err).To(HaveOccurred())
			Expect(db.IsResultNotFoundError(err)).To(BeTrue())
			Expect(applicationStateDB.Conditions).To(BeNil())

			By("Create a new ArgoCD Application")
			err = reconciler.Create(ctx, guestbookApp)
			Expect(err).ToNot(HaveOccurred())

			By("Call reconcile function")
			result, err := reconciler.Reconcile(ctx, newRequest(namespace, name))
			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())

			err = reconciler.DB.GetApplicationStateById(ctx, applicationStateDB)
			Expect(applicationStateDB.ReconciledState).ToNot(BeNil())
			Expect(err).ToNot(HaveOccurred())

			By("Verifying that the ApplicationState DB row has been updated to match the Application conditions")
			resultConditions := []appv1.ApplicationCondition{}
			err = yaml.Unmarshal(applicationStateDB.Conditions, &resultConditions)
			Expect(err).ToNot(HaveOccurred())
			Expect(resultConditions).To(HaveLen(len(guestbookApp.Status.Conditions)))
		})

		It("Test to verify if the conditions is empty if there are no Application conditions", func() {
			By("Close database connection")
			defer dbQueries.CloseDatabase()
			defer testTeardown()
			Expect(err).ToNot(HaveOccurred())

			ctx = context.Background()

			guestbookApp := &appv1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
					Labels: map[string]string{
						dbID: "test-my-application",
					},
				},
				Spec: appv1.ApplicationSpec{
					Source: &appv1.ApplicationSource{
						Path:           "guestbook",
						TargetRevision: "HEAD",
						RepoURL:        "https://github.com/argoproj/argocd-example-apps.git",
					},
					Destination: appv1.ApplicationDestination{
						Namespace: "guestbook",
						Server:    "https://kubernetes.default.svc",
					},
					Project: "default",
				},
				Status: appv1.ApplicationStatus{
					Health: appv1.HealthStatus{
						Status: "Healthy",
					},
					Sync: appv1.SyncStatus{
						Status: "Synced",
						ComparedTo: appv1.ComparedTo{
							Source: appv1.ApplicationSource{
								Path:           "test-path",
								RepoURL:        "test-url",
								TargetRevision: "test-branch",
							},
							Destination: appv1.ApplicationDestination{
								Namespace: "test-namespace",
								Name:      "managed-env-123",
							},
						},
					},
				},
			}

			By("Add databaseID label to applicationID")
			databaseID := guestbookApp.Labels[dbID]
			applicationDB := &db.Application{
				Application_id:          databaseID,
				Name:                    name,
				Spec_field:              "{}",
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			}

			applicationStateDB := &db.ApplicationState{
				Applicationstate_application_id: applicationDB.Application_id,
			}

			By("Create Application in Database")
			err = reconciler.DB.CreateApplication(ctx, applicationDB)
			Expect(err).ToNot(HaveOccurred())

			By("GetApplicationStateById to verify SyncError field is empty in database")
			err = reconciler.DB.GetApplicationStateById(ctx, applicationStateDB)
			Expect(err).To(HaveOccurred())
			Expect(db.IsResultNotFoundError(err)).To(BeTrue())
			Expect(applicationStateDB.Conditions).To(BeEmpty())

			By("Create a new ArgoCD Application")
			err = reconciler.Create(ctx, guestbookApp)
			Expect(err).ToNot(HaveOccurred())

			By("Call reconcile function")
			result, err := reconciler.Reconcile(ctx, newRequest(namespace, name))
			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())

			err = reconciler.DB.GetApplicationStateById(ctx, applicationStateDB)
			Expect(applicationStateDB.ReconciledState).ToNot(BeNil())
			Expect(err).ToNot(HaveOccurred())

			By("Verifying that the ApplicationState DB row has been updated to match the Application conditions")
			Expect(applicationStateDB.Conditions).To(BeEmpty())
		})

		It("Test to check whether existing condition value is updated to new value when conditions.Message of ArgoCD Application changes", func() {
			By("Close database connection")
			defer dbQueries.CloseDatabase()
			defer testTeardown()

			ctx = context.Background()

			By("Add databaseID label to applicationID")
			databaseID := guestbookApp.Labels[dbID]
			applicationDB := &db.Application{
				Application_id:          databaseID,
				Name:                    name,
				Spec_field:              "{}",
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			}

			reconciledStateString, reconciledObj, err := dummyApplicationComparedToField()
			Expect(reconciledObj.Destination.Namespace).ToNot(Equal(guestbookApp.Status.Sync.ComparedTo.Destination.Namespace))
			Expect(err).ToNot(HaveOccurred())

			appConditions := []appv1.ApplicationCondition{
				{
					Type:               appv1.ApplicationConditionSyncError,
					Message:            "Failed to sync",
					LastTransitionTime: &metav1.Time{Time: time.Now()},
				},
			}

			conditionBytes, err := yaml.Marshal(&appConditions)
			Expect(err).ToNot(HaveOccurred())

			// applicationState which already exists in database
			applicationState := &db.ApplicationState{
				Applicationstate_application_id: applicationDB.Application_id,
				Health:                          "Healthy",
				Sync_Status:                     "Synced",
				ReconciledState:                 reconciledStateString,
				Conditions:                      conditionBytes,
			}

			By("Create a new ArgoCD Application")
			err = reconciler.Create(ctx, guestbookApp)
			Expect(err).ToNot(HaveOccurred())

			By("Create Application in Database")
			err = reconciler.DB.CreateApplication(ctx, applicationDB)
			Expect(err).ToNot(HaveOccurred())

			By("Create ApplicationState in Database")
			err = reconciler.DB.CreateApplicationState(ctx, applicationState)
			Expect(err).ToNot(HaveOccurred())

			By("Call reconcile function")
			result, err := reconciler.Reconcile(ctx, newRequest(namespace, name))
			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())

			applicationStateget := &db.ApplicationState{
				Applicationstate_application_id: applicationDB.Application_id,
			}

			err = reconciler.DB.GetApplicationStateById(ctx, applicationStateget)
			Expect(err).ToNot(HaveOccurred())

			outputConditions := []appv1.ApplicationCondition{}
			err = yaml.Unmarshal(applicationStateget.Conditions, &outputConditions)
			Expect(err).ToNot(HaveOccurred())

			compareConditions := func(cond1, cond2 []appv1.ApplicationCondition) {
				Expect(cond1).To(HaveLen(len(cond2)))

				for i, cond := range cond1 {
					other := cond2[i]
					Expect(cond.Type).To(Equal(other.Type))
					Expect(cond.Message).To(Equal(other.Message))
				}
			}

			compareConditions(appConditions, outputConditions)

			By("update ApplicationState conditions")
			newConditions := []appv1.ApplicationCondition{
				{
					Type:               appv1.ApplicationConditionComparisonError,
					Message:            "Failed to compare",
					LastTransitionTime: &metav1.Time{Time: time.Now()},
				},
			}

			conditionBytes, err = yaml.Marshal(&newConditions)
			Expect(err).ToNot(HaveOccurred())

			applicationState.Conditions = conditionBytes
			err = reconciler.DB.UpdateApplicationState(ctx, applicationState)
			Expect(err).ToNot(HaveOccurred())

			By("Call reconcile function")
			result, err = reconciler.Reconcile(ctx, newRequest(namespace, name))
			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())

			By("Verifying that the ApplicationState DB row has been updated to match the Application Conditions")

			err = yaml.Unmarshal(applicationState.Conditions, &outputConditions)
			Expect(err).ToNot(HaveOccurred())

			compareConditions(outputConditions, newConditions)
		})
	})

	Context("Test compressObject function", func() {
		It("Should compress resource data into byte array", func() {
			resourceStatus := appv1.ResourceStatus{
				Group:     "apps",
				Version:   "v1",
				Kind:      "Deployment",
				Namespace: "argoCD",
				Name:      "component-a",
				Status:    "Synced",
				Health: &appv1.HealthStatus{
					Status:  "Healthy",
					Message: "success",
				},
			}

			var resources []appv1.ResourceStatus
			resources = append(resources, resourceStatus)

			byteArr, err := sharedutil.CompressObject(resources)

			Expect(err).ToNot(HaveOccurred())
			Expect(byteArr).NotTo(BeEmpty())
		})

		It("Should compress operationState data into byte array", func() {
			operationState := appv1.OperationState{
				Operation: appv1.Operation{
					InitiatedBy: appv1.OperationInitiator{
						Automated: true,
					},
					Retry: appv1.RetryStrategy{
						Limit: -1,
					},
				},
				SyncResult: &appv1.SyncOperationResult{
					Resources: appv1.ResourceResults{
						&appv1.ResourceResult{
							Group:     "",
							HookPhase: common.OperationRunning,
							Namespace: "jane",
							Status:    common.ResultCodeSynced,
						},
					},
				},
				StartedAt:  metav1.Time{Time: time.Now()},
				FinishedAt: &metav1.Time{Time: time.Now()},
			}

			byteArr, err := sharedutil.CompressObject(operationState)

			Expect(err).ToNot(HaveOccurred())
			Expect(byteArr).NotTo(BeEmpty())
		})

		It("Should work for empty ResourceStatus", func() {
			var resourceStatus appv1.ResourceStatus
			var resources []appv1.ResourceStatus
			resources = append(resources, resourceStatus)

			byteArr, err := sharedutil.CompressObject(resources)

			Expect(err).ToNot(HaveOccurred())
			Expect(byteArr).NotTo(BeEmpty())
		})

		It("Should work for empty Resource Array", func() {
			var resources []appv1.ResourceStatus

			byteArr, err := sharedutil.CompressObject(resources)

			Expect(err).ToNot(HaveOccurred())
			Expect(byteArr).NotTo(BeEmpty())
		})
	})
})

var _ = Describe("Namespace Reconciler Tests.", func() {
	var reconciler ApplicationReconciler

	Context("Testing for Namespace Reconciler.", func() {
		var err error
		var ctx context.Context
		var argoCdApp appv1.Application
		var dummyApplicationSpec string
		var applicationput db.Application
		var dbQueries db.AllDatabaseQueries

		BeforeEach(func() {
			ctx = context.Background()

			err = db.SetupForTestingDBGinkgo()
			Expect(err).ToNot(HaveOccurred())

			dbQueries, err = db.NewUnsafePostgresDBQueries(false, true)
			Expect(err).ToNot(HaveOccurred())

			_, managedEnvironment, _, gitopsEngineInstance, _, err := db.CreateSampleData(dbQueries)
			Expect(err).ToNot(HaveOccurred())

			scheme, argocdNamespace, kubesystemNamespace, workspace, err := tests.GenericTestSetup()
			Expect(err).ToNot(HaveOccurred())

			_, dummyApplicationSpec, argoCdApp, err = createDummyApplicationData()
			Expect(err).ToNot(HaveOccurred())

			applicationput = db.Application{
				Application_id:          "test-my-application",
				Name:                    "test-my-application",
				Spec_field:              dummyApplicationSpec,
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			}

			err = dbQueries.CreateApplication(ctx, &applicationput)
			Expect(err).ToNot(HaveOccurred())

			err = appv1.AddToScheme(scheme)
			Expect(err).ToNot(HaveOccurred())

			gitopsDepl := &managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-my-gitops-depl",
					Namespace: workspace.Name,
					UID:       uuid.NewUUID(),
				},
			}

			// Fake kube client.
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(gitopsDepl, workspace, argocdNamespace, kubesystemNamespace).Build()

			reconciler = ApplicationReconciler{
				Client: k8sClient,
				DB:     dbQueries,
				Cache:  application_info_cache.NewApplicationInfoCache(),
			}

			err = reconciler.Create(ctx, &argoCdApp)
			Expect(err).ToNot(HaveOccurred())

			var specialClusterUser db.ClusterUser
			err = dbQueries.GetOrCreateSpecialClusterUser(context.Background(), &specialClusterUser)
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			listOfK8sOperation := managedgitopsv1alpha1.OperationList{}
			err = reconciler.List(ctx, &listOfK8sOperation)
			Expect(err).ToNot(HaveOccurred())

			for _, k8sOperation := range listOfK8sOperation.Items {
				// Look for Operation created by Namespace Reconciler.
				if k8sOperation.Annotations[operations.IdentifierKey] == operations.IdentifierValue {
					rowsAffected, err := dbQueries.DeleteOperationById(ctx, k8sOperation.Spec.OperationID)
					Expect(err).ToNot(HaveOccurred())
					Expect(rowsAffected).Should((Equal(1)))

					err = reconciler.Delete(ctx, &k8sOperation)
					Expect(err).ToNot(HaveOccurred())
				}
			}
		})

		It("Should do nothing as ArgoCD Application is in sync with DB entry.", func() {
			ctx := context.Background()
			log := log.FromContext(ctx)

			// Call function for workSpace/Namespace reconciler
			syncCRsWithDB_Applications(ctx, reconciler.DB, reconciler.Client, log)

			// We are using a fake k8s client and because of that we can not check if ArgoCD application has been created/updated.
			// We will just check if k8s Operation created or not.

			By("Verify that K8s Operation and Operation table entry are created.")

			listOfK8sOperation := managedgitopsv1alpha1.OperationList{}

			err = reconciler.List(ctx, &listOfK8sOperation)
			Expect(err).ToNot(HaveOccurred())

			count := 0
			for _, k8sOperation := range listOfK8sOperation.Items {
				// Look for Operation created by Namespace Reconciler.
				if k8sOperation.Annotations[operations.IdentifierKey] == operations.IdentifierValue {
					// Fetch corresponding DB entry
					dbOperation := db.Operation{
						Operation_id: k8sOperation.Spec.OperationID,
					}
					err = dbQueries.GetOperationById(ctx, &dbOperation)
					Expect(err).ToNot(HaveOccurred())
					if dbOperation.Resource_id == applicationput.Application_id {
						count += 1
					}
				}
			}
			Expect(count).To(Equal(0))
		})

		It("Should update OutofSync ArgoCD Application to keep it in sync with DB entry.", func() {

			// Update ArgoCD Application, so it will be out of Sync with DB entry.
			argoCdApp.Spec.Source.RepoURL = "https://github.com/test/gitops-repository-template"

			err = reconciler.Update(ctx, &argoCdApp)
			Expect(err).ToNot(HaveOccurred())

			ctx := context.Background()
			log := log.FromContext(ctx)

			// Call function for workSpace/Namespace reconciler
			syncCRsWithDB_Applications(ctx, reconciler.DB, reconciler.Client, log)

			// We are using a fake k8s client and because of that we can not check if ArgoCD application has been created/updated.
			// We will just check if k8s Operation and DB entries are created and assume that in actual environment ArgoCD will pick up this Operation and update/create the application.

			By("Verify that K8s Operation and Operation table entry are created.")

			listOfK8sOperation := managedgitopsv1alpha1.OperationList{}

			err = reconciler.List(ctx, &listOfK8sOperation)
			Expect(err).ToNot(HaveOccurred())
			Expect(listOfK8sOperation.Items).NotTo(BeEmpty())

			for _, k8sOperation := range listOfK8sOperation.Items {
				// Look for Operation created by Namespace Reconciler.
				if k8sOperation.Annotations[operations.IdentifierKey] == operations.IdentifierValue {
					// Fetch corresponding DB entry
					dbOperation := db.Operation{
						Operation_id: k8sOperation.Spec.OperationID,
					}
					err = dbQueries.GetOperationById(ctx, &dbOperation)
					Expect(err).ToNot(HaveOccurred())
					if dbOperation.Resource_id == applicationput.Application_id {
						Expect(dbOperation.Resource_id).To(Equal(applicationput.Application_id))
						Expect(dbOperation.Instance_id).To(Equal(applicationput.Engine_instance_inst_id))
					}
				}
			}
		})

		It("Should create new ArgoCD Application if application exists in DB, but it is not available in ArgoCD.", func() {

			// Delete the application from ArgoCD, but keep all DB entries.
			err = reconciler.Delete(ctx, &argoCdApp)
			Expect(err).ToNot(HaveOccurred())

			ctx := context.Background()
			log := log.FromContext(ctx)

			// Call function for workSpace/Namespace reconciler
			syncCRsWithDB_Applications(ctx, reconciler.DB, reconciler.Client, log)

			// We are using a fake k8s client and because of that we can not check if ArgoCD application has been created/updated.
			// We will just check if k8s Operation and DB entries are created and assume that in actual environment ArgoCD will pick up this Operation and update/create the application.

			By("Verify that K8s Operation and Operation table entry are created.")

			listOfK8sOperation := managedgitopsv1alpha1.OperationList{}

			err = reconciler.List(ctx, &listOfK8sOperation)
			Expect(err).ToNot(HaveOccurred())
			Expect(listOfK8sOperation.Items).NotTo(BeEmpty())

			for _, k8sOperation := range listOfK8sOperation.Items {
				// Look for Operation created by Namespace Reconciler.
				if k8sOperation.Annotations[operations.IdentifierKey] == operations.IdentifierValue {
					// Fetch corresponding DB entry
					dbOperation := db.Operation{
						Operation_id: k8sOperation.Spec.OperationID,
					}
					err = dbQueries.GetOperationById(ctx, &dbOperation)
					Expect(err).ToNot(HaveOccurred())
					if dbOperation.Resource_id == applicationput.Application_id {
						Expect(dbOperation.Resource_id).To(Equal(applicationput.Application_id))
						Expect(dbOperation.Instance_id).To(Equal(applicationput.Engine_instance_inst_id))
					}
				}
			}
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

// Create sample data for testing.
func createSampleData(dbq db.AllDatabaseQueries) (*db.ClusterCredentials, *db.ManagedEnvironment, *db.GitopsEngineCluster, *db.GitopsEngineInstance, *db.ClusterAccess, error) {

	ctx := context.Background()
	var err error

	clusterCredentials, managedEnvironment, engineCluster, engineInstance, clusterAccess := generateSampleData()

	if err = dbq.CreateClusterCredentials(ctx, &clusterCredentials); err != nil {
		return nil, nil, nil, nil, nil, err
	}

	if err = dbq.CreateManagedEnvironment(ctx, &managedEnvironment); err != nil {
		return nil, nil, nil, nil, nil, err
	}

	if err = dbq.CreateGitopsEngineCluster(ctx, &engineCluster); err != nil {
		return nil, nil, nil, nil, nil, err
	}

	if err = dbq.CreateGitopsEngineInstance(ctx, &engineInstance); err != nil {
		return nil, nil, nil, nil, nil, err
	}

	if err = dbq.CreateClusterAccess(ctx, &clusterAccess); err != nil {
		return nil, nil, nil, nil, nil, err
	}

	return &clusterCredentials, &managedEnvironment, &engineCluster, &engineInstance, &clusterAccess, nil

}

var testClusterUser = &db.ClusterUser{
	Clusteruser_id: "test-user",
	User_name:      "test-user",
}

// Generate fake cluster for testing.
func generateSampleData() (db.ClusterCredentials, db.ManagedEnvironment, db.GitopsEngineCluster, db.GitopsEngineInstance, db.ClusterAccess) {
	clusterCredentials := db.ClusterCredentials{
		Clustercredentials_cred_id:  "test-cluster-creds-test",
		Host:                        "host",
		Kube_config:                 "kube-config",
		Kube_config_context:         "kube-config-context",
		Serviceaccount_bearer_token: "serviceaccount_bearer_token",
		Serviceaccount_ns:           "Serviceaccount_ns",
	}

	managedEnvironment := db.ManagedEnvironment{
		Managedenvironment_id: "test-managed-env",
		Clustercredentials_id: clusterCredentials.Clustercredentials_cred_id,
		Name:                  "my env",
	}

	gitopsEngineCluster := db.GitopsEngineCluster{
		Gitopsenginecluster_id: "test-fake-cluster",
		Clustercredentials_id:  clusterCredentials.Clustercredentials_cred_id,
	}

	gitopsEngineInstance := db.GitopsEngineInstance{
		Gitopsengineinstance_id: "test-fake-engine-instance-id",
		Namespace_name:          "test-fake-namespace",
		Namespace_uid:           "test-fake-namespace",
		EngineCluster_id:        gitopsEngineCluster.Gitopsenginecluster_id,
	}

	clusterAccess := db.ClusterAccess{
		Clusteraccess_user_id:                   testClusterUser.Clusteruser_id,
		Clusteraccess_managed_environment_id:    managedEnvironment.Managedenvironment_id,
		Clusteraccess_gitops_engine_instance_id: gitopsEngineInstance.Gitopsengineinstance_id,
	}

	return clusterCredentials, managedEnvironment, gitopsEngineCluster, gitopsEngineInstance, clusterAccess
}

func testTeardown() {
	err := db.SetupForTestingDBGinkgo()
	Expect(err).ToNot(HaveOccurred())
}

func createDummyApplicationData() (fauxargocd.FauxApplication, string, appv1.Application, error) {
	// Create dummy ArgoCD Application CR.
	dummyArgoCdApplication := appv1.Application{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Application",
			APIVersion: "argoproj.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-my-application",
			Namespace: "gitops-service-argocd",
		},
		Spec: appv1.ApplicationSpec{
			Source: &appv1.ApplicationSource{
				RepoURL:        "https://github.com/redhat-appstudio/managed-gitops",
				Path:           "resources/test-data/sample-gitops-repository/environments/overlays/dev",
				TargetRevision: "",
			},
			Destination: appv1.ApplicationDestination{
				Name:      "in-cluster",
				Namespace: "test-fake-namespace",
			},
			SyncPolicy: &appv1.SyncPolicy{
				Automated: &appv1.SyncPolicyAutomated{
					Prune: false,
				},
			},
			Project: "default",
		},
	}

	// Create dummy Application Spec to be saved in DB
	dummyApplicationSpec := fauxargocd.FauxApplication{
		FauxTypeMeta: fauxargocd.FauxTypeMeta{
			Kind:       "Application",
			APIVersion: "argoproj.io/v1alpha1",
		},
		FauxObjectMeta: fauxargocd.FauxObjectMeta{
			Name:      "test-my-application",
			Namespace: "gitops-service-argocd",
		},
		Spec: fauxargocd.FauxApplicationSpec{
			Source: fauxargocd.ApplicationSource{
				RepoURL:        "https://github.com/redhat-appstudio/managed-gitops",
				Path:           "resources/test-data/sample-gitops-repository/environments/overlays/dev",
				TargetRevision: "",
			},
			Destination: fauxargocd.ApplicationDestination{
				Name:      "in-cluster",
				Namespace: "test-fake-namespace",
			},
			SyncPolicy: &fauxargocd.SyncPolicy{
				Automated: &fauxargocd.SyncPolicyAutomated{
					Prune: false,
				},
			},
			Project: "default",
		},
	}

	dummyApplicationSpecBytes, err := yaml.Marshal(dummyApplicationSpec)

	if err != nil {
		return fauxargocd.FauxApplication{}, "", appv1.Application{}, err
	}

	return dummyApplicationSpec, string(dummyApplicationSpecBytes), dummyArgoCdApplication, nil
}

func dummyApplicationComparedToField() (string, fauxargocd.FauxComparedTo, error) {

	fauxcomparedTo := fauxargocd.FauxComparedTo{
		Source: fauxargocd.ApplicationSource{
			RepoURL:        "test-url",
			Path:           "test-path",
			TargetRevision: "test-branch",
		},
		Destination: fauxargocd.ApplicationDestination{
			Namespace: "test-namespace-1",
			Name:      "managed-env-123",
		},
	}

	fauxcomparedToBytes, err := yaml.Marshal(fauxcomparedTo)
	if err != nil {
		return "", fauxcomparedTo, err
	}

	return string(fauxcomparedToBytes), fauxcomparedTo, nil
}

// Unmarshal the reconciledState stored in db
func convertReconciledStateStringToObject(reconciledState string) (fauxargocd.FauxComparedTo, error) {
	comparedTo := fauxargocd.FauxComparedTo{}

	err := json.Unmarshal([]byte(reconciledState), &comparedTo)
	if err != nil {
		return comparedTo, err
	}
	return comparedTo, nil
}

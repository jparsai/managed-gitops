package eventloop

import (
	"context"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/tests"
	"k8s.io/apimachinery/pkg/util/uuid"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	db "github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logger "sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("DB Reconciler Test", func() {
	Context("Testing Reconcile for DeploymentToApplicationMapping table entries.", func() {

		var log logr.Logger
		var ctx context.Context
		var dbq db.AllDatabaseQueries
		var k8sClient client.WithWatch
		var application db.Application
		var syncOperation db.SyncOperation
		var applicationState db.ApplicationState
		var managedEnvironment *db.ManagedEnvironment
		var gitopsEngineInstance *db.GitopsEngineInstance
		var gitopsDepl managedgitopsv1alpha1.GitOpsDeployment
		var deploymentToApplicationMapping db.DeploymentToApplicationMapping

		BeforeEach(func() {
			scheme,
				argocdNamespace,
				kubesystemNamespace,
				apiNamespace,
				err := tests.GenericTestSetup()
			Expect(err).To(BeNil())

			// Create fake client
			k8sClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(apiNamespace, argocdNamespace, kubesystemNamespace).
				Build()

			err = db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())

			ctx = context.Background()
			log = logger.FromContext(ctx)
			dbq, err = db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())

			_, managedEnvironment, _, gitopsEngineInstance, _, err = db.CreateSampleData(dbq)
			Expect(err).To(BeNil())

			// Create Application entry
			application = db.Application{
				Application_id:          "test-my-application",
				Name:                    "my-application",
				Spec_field:              "{}",
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			}
			err = dbq.CreateApplication(ctx, &application)
			Expect(err).To(BeNil())

			// Create ApplicationState entry
			applicationState = db.ApplicationState{
				Applicationstate_application_id: application.Application_id,
				Health:                          "Healthy",
				Sync_Status:                     "Synced",
				ReconciledState:                 "Healthy",
			}
			err = dbq.CreateApplicationState(ctx, &applicationState)
			Expect(err).To(BeNil())

			gitopsDepl = managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-deployment",
					Namespace: "test-namespace",
					UID:       "test-" + uuid.NewUUID(),
				},
				Spec: managedgitopsv1alpha1.GitOpsDeploymentSpec{
					Source: managedgitopsv1alpha1.ApplicationSource{},
					Type:   managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated,
				},
			}

			// Create DeploymentToApplicationMapping entry
			deploymentToApplicationMapping = db.DeploymentToApplicationMapping{
				Deploymenttoapplicationmapping_uid_id: string(gitopsDepl.UID),
				Application_id:                        application.Application_id,
				DeploymentName:                        gitopsDepl.Name,
				DeploymentNamespace:                   gitopsDepl.Namespace,
				NamespaceUID:                          "demo-namespace",
			}
			err = dbq.CreateDeploymentToApplicationMapping(ctx, &deploymentToApplicationMapping)
			Expect(err).To(BeNil())

			// Create GitOpsDeployment CR in cluster
			err = k8sClient.Create(context.Background(), &gitopsDepl)
			Expect(err).To(BeNil())

			// Create SyncOperation entry
			syncOperation = db.SyncOperation{
				SyncOperation_id:    "test-syncOperation",
				Application_id:      application.Application_id,
				Revision:            "master",
				DeploymentNameField: deploymentToApplicationMapping.DeploymentName,
				DesiredState:        "Synced",
			}
			err = dbq.CreateSyncOperation(ctx, &syncOperation)
			Expect(err).To(BeNil())
		})

		It("Should not delete any of the database entries as long as the GitOpsDeployment CR is present in cluster, and the UID matches the DTAM value", func() {
			defer dbq.CloseDatabase()

			By("Call function for databaseReconcile to check delete DB entries if GitOpsDeployment CR is not present.")
			deplToAppMappingDbReconcile(ctx, dbq, k8sClient, log)

			By("Verify that no entry is deleted from DB.")
			err := dbq.GetApplicationStateById(ctx, &applicationState)
			Expect(err).To(BeNil())

			err = dbq.GetSyncOperationById(ctx, &syncOperation)
			Expect(err).To(BeNil())
			Expect(syncOperation.Application_id).NotTo(BeEmpty())

			err = dbq.GetDeploymentToApplicationMappingByApplicationId(ctx, &deploymentToApplicationMapping)
			Expect(err).To(BeNil())

			err = dbq.GetApplicationById(ctx, &application)
			Expect(err).To(BeNil())
		})

		It("Should delete related database entries from DB, if the GitOpsDeployment CRs of the DTAM is not present on cluster.", func() {
			defer dbq.CloseDatabase()

			// Create another Application entry
			applicationOne := application
			applicationOne.Application_id = "test-my-application-1"
			applicationOne.Name = "my-application-1"
			err := dbq.CreateApplication(ctx, &applicationOne)
			Expect(err).To(BeNil())

			// Create another DeploymentToApplicationMapping entry
			deploymentToApplicationMappingOne := deploymentToApplicationMapping
			deploymentToApplicationMappingOne.Deploymenttoapplicationmapping_uid_id = "test-" + string(uuid.NewUUID())
			deploymentToApplicationMappingOne.Application_id = applicationOne.Application_id
			deploymentToApplicationMappingOne.DeploymentName = "test-deployment-1"
			err = dbq.CreateDeploymentToApplicationMapping(ctx, &deploymentToApplicationMappingOne)
			Expect(err).To(BeNil())

			// Create another ApplicationState entry
			applicationStateOne := applicationState
			applicationStateOne.Applicationstate_application_id = applicationOne.Application_id
			err = dbq.CreateApplicationState(ctx, &applicationStateOne)
			Expect(err).To(BeNil())

			// Create another SyncOperation entry
			syncOperationOne := syncOperation
			syncOperationOne.SyncOperation_id = "test-syncOperation-1"
			syncOperationOne.Application_id = applicationOne.Application_id
			syncOperationOne.DeploymentNameField = deploymentToApplicationMappingOne.DeploymentName
			err = dbq.CreateSyncOperation(ctx, &syncOperationOne)
			Expect(err).To(BeNil())

			By("Call function for databaseReconcile to check/delete DB entries if GitOpsDeployment CR is not present.")
			deplToAppMappingDbReconcile(ctx, dbq, k8sClient, log)

			By("Verify that entries for the GitOpsDeployment which is available in cluster, are not deleted from DB.")

			err = dbq.GetApplicationStateById(ctx, &applicationState)
			Expect(err).To(BeNil())

			err = dbq.GetSyncOperationById(ctx, &syncOperation)
			Expect(err).To(BeNil())
			Expect(syncOperation.Application_id).To(Equal(application.Application_id))

			err = dbq.GetDeploymentToApplicationMappingByApplicationId(ctx, &deploymentToApplicationMapping)
			Expect(err).To(BeNil())

			err = dbq.GetApplicationById(ctx, &application)
			Expect(err).To(BeNil())

			By("Verify that entries for the GitOpsDeployment which is not available in cluster, are deleted from DB.")

			err = dbq.GetApplicationStateById(ctx, &applicationStateOne)
			Expect(db.IsResultNotFoundError(err)).To(BeTrue())

			err = dbq.GetSyncOperationById(ctx, &syncOperationOne)
			Expect(err).To(BeNil())
			Expect(syncOperationOne.Application_id).To(BeEmpty())

			err = dbq.GetDeploymentToApplicationMappingByApplicationId(ctx, &deploymentToApplicationMappingOne)
			Expect(db.IsResultNotFoundError(err)).To(BeTrue())

			err = dbq.GetApplicationById(ctx, &applicationOne)
			Expect(db.IsResultNotFoundError(err)).To(BeTrue())
		})

		It("should delete the DTAM if the GitOpsDeployment CR if it is present, but the UID doesn't match what is in the DTAM", func() {
			defer dbq.CloseDatabase()

			By("We ensure that the GitOpsDeployment still has the same name, but has a different UID. This simulates a new GitOpsDeployment with the same name/namespace.")
			newUID := "test-" + uuid.NewUUID()
			gitopsDepl.UID = newUID
			err := k8sClient.Update(ctx, &gitopsDepl)
			Expect(err).To(BeNil())

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&gitopsDepl), &gitopsDepl)
			Expect(err).To(BeNil())
			Expect(gitopsDepl.UID).To(Equal(newUID))

			By("calling function for databaseReconcile to check delete DB entries if GitOpsDeployment CR is not present.")
			deplToAppMappingDbReconcile(ctx, dbq, k8sClient, log)

			By("Verify that entries for the GitOpsDeployment which is not available in cluster, are deleted from DB.")

			err = dbq.GetApplicationStateById(ctx, &applicationState)
			Expect(db.IsResultNotFoundError(err)).To(BeTrue())

			err = dbq.GetSyncOperationById(ctx, &syncOperation)
			Expect(err).To(BeNil())
			Expect(syncOperation.Application_id).To(BeEmpty())

			err = dbq.GetDeploymentToApplicationMappingByApplicationId(ctx, &deploymentToApplicationMapping)
			Expect(db.IsResultNotFoundError(err)).To(BeTrue())

			err = dbq.GetApplicationById(ctx, &application)
			Expect(db.IsResultNotFoundError(err)).To(BeTrue())

		})
	})

	Context("Testing Reconcile for APICRToDBMapping table entries.", func() {
		Context("Testing Reconcile for APICRToDBMapping table entries of ManagedEnvironment CR.", func() {
			var log logr.Logger
			var ctx context.Context
			var dbq db.AllDatabaseQueries
			var k8sClient client.WithWatch
			var clusterCredentials db.ClusterCredentials
			var managedEnvironment db.ManagedEnvironment
			var apiCRToDatabaseMapping db.APICRToDatabaseMapping

			BeforeEach(func() {
				scheme,
					argocdNamespace,
					kubesystemNamespace,
					apiNamespace,
					err := tests.GenericTestSetup()
				Expect(err).To(BeNil())

				// Create fake client
				k8sClient = fake.NewClientBuilder().
					WithScheme(scheme).
					WithObjects(apiNamespace, argocdNamespace, kubesystemNamespace).
					Build()

				err = db.SetupForTestingDBGinkgo()
				Expect(err).To(BeNil())

				ctx = context.Background()
				log = logger.FromContext(ctx)
				dbq, err = db.NewUnsafePostgresDBQueries(true, true)
				Expect(err).To(BeNil())

				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-managed-env-secret",
						Namespace: "test-k8s-namespace",
					},
					Type:       "managed-gitops.redhat.com/managed-environment",
					StringData: map[string]string{"kubeconfig": "abc"},
				}

				err = k8sClient.Create(context.Background(), secret)
				Expect(err).To(BeNil())

				managedEnv := &managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-env-" + string(uuid.NewUUID()),
						Namespace: "test-k8s-namespace",
						UID:       uuid.NewUUID(),
					},
					Spec: managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironmentSpec{
						APIURL:                     "",
						ClusterCredentialsSecret:   secret.Name,
						AllowInsecureSkipTLSVerify: true,
					},
				}

				err = k8sClient.Create(context.Background(), managedEnv)
				Expect(err).To(BeNil())

				clusterCredentials = db.ClusterCredentials{
					Clustercredentials_cred_id:  "test-" + string(uuid.NewUUID()),
					Host:                        "host",
					Kube_config:                 "kube-config",
					Kube_config_context:         "kube-config-context",
					Serviceaccount_bearer_token: "serviceaccount_bearer_token",
					Serviceaccount_ns:           "Serviceaccount_ns",
				}

				err = dbq.CreateClusterCredentials(ctx, &clusterCredentials)
				Expect(err).To(BeNil())

				managedEnvironment = db.ManagedEnvironment{
					Managedenvironment_id: "test-env-" + string(managedEnv.UID),
					Clustercredentials_id: clusterCredentials.Clustercredentials_cred_id,
					Name:                  managedEnv.Name,
				}

				err = dbq.CreateManagedEnvironment(ctx, &managedEnvironment)
				Expect(err).To(BeNil())

				apiCRToDatabaseMapping = db.APICRToDatabaseMapping{
					APIResourceType:      db.APICRToDatabaseMapping_ResourceType_GitOpsDeploymentManagedEnvironment,
					APIResourceUID:       string(managedEnv.UID),
					APIResourceName:      managedEnv.Name,
					APIResourceNamespace: managedEnv.Namespace,
					NamespaceUID:         "test-" + string(uuid.NewUUID()),
					DBRelationType:       db.APICRToDatabaseMapping_DBRelationType_ManagedEnvironment,
					DBRelationKey:        managedEnvironment.Managedenvironment_id,
				}

			})

			It("Should not delete any of the database entries as long as the Managed Environment CR is present in cluster, and the UID matches the APICRToDatabaseMapping value", func() {
				defer dbq.CloseDatabase()

				err := dbq.CreateAPICRToDatabaseMapping(ctx, &apiCRToDatabaseMapping)
				Expect(err).To(BeNil())

				By("Call function for apiCrToDbMappingDbReconcile.")
				apiCrToDbMappingDbReconcile(ctx, dbq, k8sClient, log)

				By("Verify that no entry is deleted from DB.")
				err = dbq.GetManagedEnvironmentById(ctx, &managedEnvironment)
				Expect(err).To(BeNil())

				err = dbq.GetAPICRForDatabaseUID(ctx, &apiCRToDatabaseMapping)
				Expect(err).To(BeNil())
			})

			It("Should delete related database entries from DB, if the Managed Environment CR of the APICRToDatabaseMapping is not present on cluster.", func() {
				defer dbq.CloseDatabase()

				err := dbq.CreateAPICRToDatabaseMapping(ctx, &apiCRToDatabaseMapping)
				Expect(err).To(BeNil())

				managedEnvironment.Name = "test-env-" + string(uuid.NewUUID())
				managedEnvironment.Managedenvironment_id = "test-" + string(uuid.NewUUID())
				err = dbq.CreateManagedEnvironment(ctx, &managedEnvironment)
				Expect(err).To(BeNil())

				apiCRToDatabaseMapping.DBRelationKey = managedEnvironment.Managedenvironment_id
				apiCRToDatabaseMapping.APIResourceUID = "test-" + string(uuid.NewUUID())
				apiCRToDatabaseMapping.APIResourceName = managedEnvironment.Name
				err = dbq.CreateAPICRToDatabaseMapping(ctx, &apiCRToDatabaseMapping)
				Expect(err).To(BeNil())

				By("Call function for apiCrToDbMappingDbReconcile.")
				apiCrToDbMappingDbReconcile(ctx, dbq, k8sClient, log)

				By("Verify that entries for the GitOpsDeployment which is not available in cluster, are deleted from DB.")

				err = dbq.GetManagedEnvironmentById(ctx, &managedEnvironment)
				Expect(db.IsResultNotFoundError(err)).To(BeTrue())

				err = dbq.GetAPICRForDatabaseUID(ctx, &apiCRToDatabaseMapping)
				Expect(db.IsResultNotFoundError(err)).To(BeTrue())
			})

			It("should delete related database entries from DB, if the Managed Environment CR is present in cluster, but the UID doesn't match what is in the APICRToDatabaseMapping", func() {
				defer dbq.CloseDatabase()

				apiCRToDatabaseMapping.APIResourceUID = "test-" + string(uuid.NewUUID())
				err := dbq.CreateAPICRToDatabaseMapping(ctx, &apiCRToDatabaseMapping)
				Expect(err).To(BeNil())

				By("Call function for apiCrToDbMappingDbReconcile.")
				apiCrToDbMappingDbReconcile(ctx, dbq, k8sClient, log)

				By("Verify that entries for the GitOpsDeployment which is not available in cluster, are deleted from DB.")

				err = dbq.GetManagedEnvironmentById(ctx, &managedEnvironment)
				Expect(db.IsResultNotFoundError(err)).To(BeTrue())

				err = dbq.GetAPICRForDatabaseUID(ctx, &apiCRToDatabaseMapping)
				Expect(db.IsResultNotFoundError(err)).To(BeTrue())
			})
		})

		Context("Testing Reconcile for APICRToDBMapping table entries of RepositoryCredential CR.", func() {
			var log logr.Logger
			var ctx context.Context
			var dbq db.AllDatabaseQueries
			var k8sClient client.WithWatch
			var clusterUser *db.ClusterUser
			var apiCRToDatabaseMapping db.APICRToDatabaseMapping
			var gitopsRepositoryCredentials db.RepositoryCredentials

			BeforeEach(func() {
				scheme,
					argocdNamespace,
					kubesystemNamespace,
					apiNamespace,
					err := tests.GenericTestSetup()
				Expect(err).To(BeNil())

				// Create fake client
				k8sClient = fake.NewClientBuilder().
					WithScheme(scheme).
					WithObjects(apiNamespace, argocdNamespace, kubesystemNamespace).
					Build()

				err = db.SetupForTestingDBGinkgo()
				Expect(err).To(BeNil())

				ctx = context.Background()
				log = logger.FromContext(ctx)
				dbq, err = db.NewUnsafePostgresDBQueries(true, true)
				Expect(err).To(BeNil())

				_, _, _, gitopsEngineInstance, _, err := db.CreateSampleData(dbq)
				Expect(err).To(BeNil())

				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-secret",
						Namespace: "test-k8s-namespace",
					},
					Type: "managed-gitops.redhat.com/managed-environment",
					StringData: map[string]string{
						"username": "test-user",
						"password": "test@123",
					},
				}
				err = k8sClient.Create(context.Background(), secret)
				Expect(err).To(BeNil())

				repoCredential := managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredential{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-repo-" + string(uuid.NewUUID()),
						Namespace: "test-k8s-namespace",
						UID:       uuid.NewUUID(),
					},
					Spec: managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialSpec{
						Repository: "https://test-private-url",
						Secret:     "test-secret",
					},
				}
				err = k8sClient.Create(context.Background(), &repoCredential)
				Expect(err).To(BeNil())

				clusterUser = &db.ClusterUser{
					Clusteruser_id: "test-repocred-user-id",
					User_name:      "test-repocred-user",
				}
				err = dbq.CreateClusterUser(ctx, clusterUser)
				Expect(err).To(BeNil())

				By("Creating a RepositoryCredentials object")
				gitopsRepositoryCredentials = db.RepositoryCredentials{
					RepositoryCredentialsID: "test-repo-" + string(uuid.NewUUID()),
					UserID:                  clusterUser.Clusteruser_id,
					PrivateURL:              "https://test-private-url",
					AuthUsername:            "test-auth-username",
					AuthPassword:            "test-auth-password",
					AuthSSHKey:              "test-auth-ssh-key",
					SecretObj:               "test-secret-obj",
					EngineClusterID:         gitopsEngineInstance.Gitopsengineinstance_id,
				}
				err = dbq.CreateRepositoryCredentials(ctx, &gitopsRepositoryCredentials)
				Expect(err).To(BeNil())

				apiCRToDatabaseMapping = db.APICRToDatabaseMapping{
					APIResourceType:      db.APICRToDatabaseMapping_ResourceType_GitOpsDeploymentRepositoryCredential,
					APIResourceUID:       string(repoCredential.UID),
					APIResourceName:      repoCredential.Name,
					APIResourceNamespace: repoCredential.Namespace,
					NamespaceUID:         "test-" + string(uuid.NewUUID()),
					DBRelationType:       db.APICRToDatabaseMapping_DBRelationType_ManagedEnvironment,
					DBRelationKey:        gitopsRepositoryCredentials.RepositoryCredentialsID,
				}
			})

			It("Should not delete any of the database entries as long as the RepositoryCredentials CR is present in cluster, and the UID matches the APICRToDatabaseMapping value", func() {
				defer dbq.CloseDatabase()

				err := dbq.CreateAPICRToDatabaseMapping(ctx, &apiCRToDatabaseMapping)
				Expect(err).To(BeNil())

				By("Call function for apiCrToDbMappingDbReconcile.")
				apiCrToDbMappingDbReconcile(ctx, dbq, k8sClient, log)

				By("Verify that no entry is deleted from DB.")
				_, err = dbq.GetRepositoryCredentialsByID(ctx, gitopsRepositoryCredentials.RepositoryCredentialsID)
				Expect(err).To(BeNil())

				err = dbq.GetAPICRForDatabaseUID(ctx, &apiCRToDatabaseMapping)
				Expect(err).To(BeNil())
			})

			It("Should delete related database entries from DB, if the Managed Environment CR of the APICRToDatabaseMapping is not present on cluster.", func() {
				defer dbq.CloseDatabase()

				err := dbq.CreateAPICRToDatabaseMapping(ctx, &apiCRToDatabaseMapping)
				Expect(err).To(BeNil())

				gitopsRepositoryCredentials.RepositoryCredentialsID = "test-repo-" + string(uuid.NewUUID())
				err = dbq.CreateRepositoryCredentials(ctx, &gitopsRepositoryCredentials)
				Expect(err).To(BeNil())

				apiCRToDatabaseMapping.DBRelationKey = gitopsRepositoryCredentials.RepositoryCredentialsID
				apiCRToDatabaseMapping.APIResourceUID = "test-" + string(uuid.NewUUID())
				apiCRToDatabaseMapping.APIResourceName = "test-" + string(uuid.NewUUID())
				err = dbq.CreateAPICRToDatabaseMapping(ctx, &apiCRToDatabaseMapping)
				Expect(err).To(BeNil())

				By("Call function for apiCrToDbMappingDbReconcile.")
				apiCrToDbMappingDbReconcile(ctx, dbq, k8sClient, log)

				By("Verify that entries for the GitOpsDeployment which is not available in cluster, are deleted from DB.")

				_, err = dbq.GetRepositoryCredentialsByID(ctx, gitopsRepositoryCredentials.RepositoryCredentialsID)
				Expect(db.IsResultNotFoundError(err)).To(BeTrue())

				err = dbq.GetAPICRForDatabaseUID(ctx, &apiCRToDatabaseMapping)
				Expect(db.IsResultNotFoundError(err)).To(BeTrue())
			})

			It("should delete related database entries from DB, if the Managed Environment CR is present in cluster, but the UID doesn't match what is in the APICRToDatabaseMapping", func() {
				defer dbq.CloseDatabase()

				err := dbq.CreateAPICRToDatabaseMapping(ctx, &apiCRToDatabaseMapping)
				Expect(err).To(BeNil())

				gitopsRepositoryCredentials.RepositoryCredentialsID = "test-repo-" + string(uuid.NewUUID())
				err = dbq.CreateRepositoryCredentials(ctx, &gitopsRepositoryCredentials)
				Expect(err).To(BeNil())

				apiCRToDatabaseMapping.DBRelationKey = gitopsRepositoryCredentials.RepositoryCredentialsID
				apiCRToDatabaseMapping.APIResourceUID = "test-" + string(uuid.NewUUID())
				err = dbq.CreateAPICRToDatabaseMapping(ctx, &apiCRToDatabaseMapping)
				Expect(err).To(BeNil())

				By("Call function for apiCrToDbMappingDbReconcile.")
				apiCrToDbMappingDbReconcile(ctx, dbq, k8sClient, log)

				By("Verify that entries for the GitOpsDeployment which is not available in cluster, are deleted from DB.")

				_, err = dbq.GetRepositoryCredentialsByID(ctx, gitopsRepositoryCredentials.RepositoryCredentialsID)
				Expect(db.IsResultNotFoundError(err)).To(BeTrue())

				err = dbq.GetAPICRForDatabaseUID(ctx, &apiCRToDatabaseMapping)
				Expect(db.IsResultNotFoundError(err)).To(BeTrue())
			})
		})

		Context("Testing Reconcile for APICRToDBMapping table entries of GitOpsDeploymentSyncRun CR.", func() {
			var log logr.Logger
			var ctx context.Context
			var dbq db.AllDatabaseQueries
			var k8sClient client.WithWatch
			var syncOperation db.SyncOperation
			var apiCRToDatabaseMapping db.APICRToDatabaseMapping

			BeforeEach(func() {
				scheme,
					argocdNamespace,
					kubesystemNamespace,
					apiNamespace,
					err := tests.GenericTestSetup()
				Expect(err).To(BeNil())

				// Create fake client
				k8sClient = fake.NewClientBuilder().
					WithScheme(scheme).
					WithObjects(apiNamespace, argocdNamespace, kubesystemNamespace).
					Build()

				err = db.SetupForTestingDBGinkgo()
				Expect(err).To(BeNil())

				ctx = context.Background()
				log = logger.FromContext(ctx)
				dbq, err = db.NewUnsafePostgresDBQueries(true, true)
				Expect(err).To(BeNil())

				_, managedEnvironment, _, gitopsEngineInstance, _, err := db.CreateSampleData(dbq)
				Expect(err).To(BeNil())

				application := &db.Application{
					Application_id:          "test-app-" + string(uuid.NewUUID()),
					Name:                    "test-app",
					Spec_field:              "{}",
					Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
					Managed_environment_id:  managedEnvironment.Managedenvironment_id,
				}

				err = dbq.CreateApplication(ctx, application)
				Expect(err).To(BeNil())

				gitopsDeplSyncRun := managedgitopsv1alpha1.GitOpsDeploymentSyncRun{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-gitopsdeployment-syncrun",
						Namespace: "test-k8s-namespace",
						UID:       uuid.NewUUID(),
					},
					Spec: managedgitopsv1alpha1.GitOpsDeploymentSyncRunSpec{
						GitopsDeploymentName: application.Name,
						RevisionID:           "HEAD",
					},
				}

				err = k8sClient.Create(context.Background(), &gitopsDeplSyncRun)
				Expect(err).To(BeNil())

				syncOperation = db.SyncOperation{
					SyncOperation_id:    "test-op-" + string(uuid.NewUUID()),
					Application_id:      application.Application_id,
					DeploymentNameField: "test-depl-" + string(uuid.NewUUID()),
					Revision:            "Head",
					DesiredState:        "Terminated",
				}

				err = dbq.CreateSyncOperation(ctx, &syncOperation)

				apiCRToDatabaseMapping = db.APICRToDatabaseMapping{
					APIResourceType:      db.APICRToDatabaseMapping_ResourceType_GitOpsDeploymentSyncRun,
					APIResourceUID:       string(gitopsDeplSyncRun.UID),
					APIResourceName:      gitopsDeplSyncRun.Name,
					APIResourceNamespace: gitopsDeplSyncRun.Namespace,
					NamespaceUID:         "test-" + string(uuid.NewUUID()),
					DBRelationType:       db.APICRToDatabaseMapping_DBRelationType_ManagedEnvironment,
					DBRelationKey:        syncOperation.SyncOperation_id,
				}
			})

			It("Should not delete any of the database entries as long as the GitOpsDeploymentSyncRun CR is present in cluster, and the UID matches the APICRToDatabaseMapping value", func() {
				defer dbq.CloseDatabase()

				err := dbq.CreateAPICRToDatabaseMapping(ctx, &apiCRToDatabaseMapping)
				Expect(err).To(BeNil())

				By("Call function for apiCrToDbMappingDbReconcile.")
				apiCrToDbMappingDbReconcile(ctx, dbq, k8sClient, log)

				By("Verify that no entry is deleted from DB.")
				err = dbq.GetSyncOperationById(ctx, &syncOperation)
				Expect(err).To(BeNil())

				err = dbq.GetAPICRForDatabaseUID(ctx, &apiCRToDatabaseMapping)
				Expect(err).To(BeNil())
			})

			It("Should delete related database entries from DB, if the GitOpsDeploymentSyncRun CR of the APICRToDatabaseMapping is not present on cluster.", func() {
				defer dbq.CloseDatabase()

				err := dbq.CreateAPICRToDatabaseMapping(ctx, &apiCRToDatabaseMapping)
				Expect(err).To(BeNil())

				syncOperation.SyncOperation_id = "test-sync-" + string(uuid.NewUUID())
				err = dbq.CreateSyncOperation(ctx, &syncOperation)
				Expect(err).To(BeNil())

				apiCRToDatabaseMapping.DBRelationKey = syncOperation.SyncOperation_id
				apiCRToDatabaseMapping.APIResourceUID = "test-" + string(uuid.NewUUID())
				apiCRToDatabaseMapping.APIResourceName = "test-" + string(uuid.NewUUID())
				err = dbq.CreateAPICRToDatabaseMapping(ctx, &apiCRToDatabaseMapping)
				Expect(err).To(BeNil())

				By("Call function for apiCrToDbMappingDbReconcile.")
				apiCrToDbMappingDbReconcile(ctx, dbq, k8sClient, log)

				By("Verify that entries for the GitOpsDeployment which is not available in cluster, are deleted from DB.")

				err = dbq.GetSyncOperationById(ctx, &syncOperation)
				Expect(db.IsResultNotFoundError(err)).To(BeTrue())

				err = dbq.GetAPICRForDatabaseUID(ctx, &apiCRToDatabaseMapping)
				Expect(db.IsResultNotFoundError(err)).To(BeTrue())
			})

			It("should delete related database entries from DB, if the Managed Environment CR is present in cluster, but the UID doesn't match what is in the APICRToDatabaseMapping", func() {
				defer dbq.CloseDatabase()

				err := dbq.CreateAPICRToDatabaseMapping(ctx, &apiCRToDatabaseMapping)
				Expect(err).To(BeNil())

				syncOperation.SyncOperation_id = "test-sync-" + string(uuid.NewUUID())
				err = dbq.CreateSyncOperation(ctx, &syncOperation)
				Expect(err).To(BeNil())

				apiCRToDatabaseMapping.DBRelationKey = syncOperation.SyncOperation_id
				apiCRToDatabaseMapping.APIResourceUID = "test-" + string(uuid.NewUUID())
				err = dbq.CreateAPICRToDatabaseMapping(ctx, &apiCRToDatabaseMapping)
				Expect(err).To(BeNil())

				By("Call function for apiCrToDbMappingDbReconcile.")
				apiCrToDbMappingDbReconcile(ctx, dbq, k8sClient, log)

				By("Verify that entries for the GitOpsDeployment which is not available in cluster, are deleted from DB.")

				err = dbq.GetSyncOperationById(ctx, &syncOperation)
				Expect(db.IsResultNotFoundError(err)).To(BeTrue())

				err = dbq.GetAPICRForDatabaseUID(ctx, &apiCRToDatabaseMapping)
				Expect(db.IsResultNotFoundError(err)).To(BeTrue())
			})

		})
	})
})

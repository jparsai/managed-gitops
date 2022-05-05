package application_event_loop

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/golang/mock/gomock"
	"github.com/redhat-appstudio/managed-gitops/backend/apis/managed-gitops/v1alpha1/mocks"
	condition "github.com/redhat-appstudio/managed-gitops/backend/condition/mocks"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/shared_resource_loop"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	db "github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend/apis/managed-gitops/v1alpha1"
	testStructs "github.com/redhat-appstudio/managed-gitops/backend/apis/managed-gitops/v1alpha1/mocks/structs"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/eventlooptypes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var _ = Describe("ApplicationEventLoop Test", func() {

	Context("Handle deployment modified", func() {

		It("create a deployment and ensure it processed, then delete it an ensure that is processed", func() {
			ctx := context.Background()

			scheme, argocdNamespace, kubesystemNamespace, workspace, err := eventlooptypes.GenericTestSetup()
			Expect(err).To(BeNil())

			gitopsDepl := &managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-gitops-depl",
					Namespace: workspace.Name,
					UID:       uuid.NewUUID(),
				},
			}

			informer := sharedutil.ListEventReceiver{}

			k8sClientOuter := fake.NewClientBuilder().WithScheme(scheme).WithObjects(gitopsDepl, workspace, argocdNamespace, kubesystemNamespace).Build()
			k8sClient := &sharedutil.ProxyClient{
				InnerClient: k8sClientOuter,
				Informer:    &informer,
			}

			workspaceID := string(workspace.UID)

			dbQueries, err := db.NewUnsafePostgresDBQueries(false, false)
			Expect(err).To(BeNil())

			a := applicationEventLoopRunner_Action{
				// When the code asks for a new k8s client, give it our fake client
				getK8sClientForGitOpsEngineInstance: func(gitopsEngineInstance *db.GitopsEngineInstance) (client.Client, error) {
					return k8sClient, nil
				},
				eventResourceName:           gitopsDepl.Name,
				eventResourceNamespace:      gitopsDepl.Namespace,
				workspaceClient:             k8sClient,
				log:                         log.FromContext(context.Background()),
				sharedResourceEventLoop:     shared_resource_loop.NewSharedResourceLoop(),
				workspaceID:                 workspaceID,
				testOnlySkipCreateOperation: true,
			}

			// ------

			_, _, _, err = a.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(err).To(BeNil())

			// Verify that the database entries have been created -----------------------------------------

			var deplToAppMapping db.DeploymentToApplicationMapping
			{
				var appMappings []db.DeploymentToApplicationMapping

				err = dbQueries.ListDeploymentToApplicationMappingByNamespaceAndName(context.Background(), gitopsDepl.Name, gitopsDepl.Namespace, workspaceID, &appMappings)
				Expect(err).To(BeNil())

				Expect(len(appMappings)).To(Equal(1))

				deplToAppMapping = appMappings[0]
			}

			clusterUser := db.ClusterUser{
				User_name: string(workspace.UID),
			}
			err = dbQueries.GetClusterUserByUsername(context.Background(), &clusterUser)
			Expect(err).To(BeNil())

			application := db.Application{
				Application_id: deplToAppMapping.Application_id,
			}

			err = dbQueries.GetApplicationById(context.Background(), &application)
			Expect(err).To(BeNil())

			gitopsEngineInstance := db.GitopsEngineInstance{
				Gitopsengineinstance_id: application.Engine_instance_inst_id,
			}

			err = dbQueries.GetGitopsEngineInstanceById(context.Background(), &gitopsEngineInstance)
			Expect(err).To(BeNil())

			managedEnvironment := db.ManagedEnvironment{
				Managedenvironment_id: application.Managed_environment_id,
			}
			err = dbQueries.GetManagedEnvironmentById(ctx, &managedEnvironment)
			Expect(err).To(BeNil())

			// Delete the GitOpsDepl and verify that the corresponding DB entries are removed -------------

			err = k8sClient.Delete(ctx, gitopsDepl)
			Expect(err).To(BeNil())

			_, _, _, err = a.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(err).To(BeNil())

			// Application should no longer exist
			err = dbQueries.GetApplicationById(ctx, &application)
			Expect(err).ToNot(BeNil())
			Expect(db.IsResultNotFoundError(err)).To(BeTrue())

			// DeploymentToApplicationMapping should be removed, too
			var appMappings []db.DeploymentToApplicationMapping
			err = dbQueries.ListDeploymentToApplicationMappingByNamespaceAndName(context.Background(), gitopsDepl.Name, gitopsDepl.Namespace, workspaceID, &appMappings)
			Expect(err).To(BeNil())
			Expect(len(appMappings)).To(Equal(0))

			// GitopsEngine instance should still be reachable
			err = dbQueries.GetGitopsEngineInstanceById(context.Background(), &gitopsEngineInstance)
			Expect(err).To(BeNil())

			operatorCreated := false
			operatorDeleted := false

			for idx, event := range informer.Events {

				if event.Action == sharedutil.Create && event.ObjectTypeOf() == "Operation" {
					operatorCreated = true
				}
				if event.Action == sharedutil.Delete && event.ObjectTypeOf() == "Operation" {
					operatorDeleted = true
				}

				fmt.Printf("%d) %v\n", idx, event)
			}

			Expect(operatorCreated).To(BeTrue())
			Expect(operatorDeleted).To(BeTrue())

		})

		It("create an invalid deployment and ensure it fails.", func() {
			ctx := context.Background()

			scheme, argocdNamespace, kubesystemNamespace, workspace, err := eventlooptypes.GenericTestSetup()
			Expect(err).To(BeNil())

			gitopsDepl := &managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      strings.Repeat("abc", 100),
					Namespace: workspace.Name,
					UID:       uuid.NewUUID(),
				},
			}

			k8sClientOuter := fake.NewClientBuilder().WithScheme(scheme).WithObjects(gitopsDepl, workspace, argocdNamespace, kubesystemNamespace).Build()
			k8sClient := &sharedutil.ProxyClient{
				InnerClient: k8sClientOuter,
			}

			workspaceID := string(workspace.UID)

			dbQueries, err := db.NewUnsafePostgresDBQueries(false, false)
			Expect(err).To(BeNil())

			a := applicationEventLoopRunner_Action{
				// When the code asks for a new k8s client, give it our fake client
				getK8sClientForGitOpsEngineInstance: func(gitopsEngineInstance *db.GitopsEngineInstance) (client.Client, error) {
					return k8sClient, nil
				},
				eventResourceName:           gitopsDepl.Name,
				eventResourceNamespace:      gitopsDepl.Namespace,
				workspaceClient:             k8sClient,
				log:                         log.FromContext(context.Background()),
				sharedResourceEventLoop:     shared_resource_loop.NewSharedResourceLoop(),
				workspaceID:                 workspaceID,
				testOnlySkipCreateOperation: true,
			}

			// ------

			_, _, _, err = a.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(err).NotTo(BeNil())

			gitopsDeployment := &managedgitopsv1alpha1.GitOpsDeployment{}
			gitopsDeploymentKey := client.ObjectKey{Namespace: gitopsDepl.Namespace, Name: gitopsDepl.Name}
			clientErr := a.workspaceClient.Get(ctx, gitopsDeploymentKey, gitopsDeployment)
			Expect(clientErr).To(BeNil())
			Expect(gitopsDeployment.Status.Conditions).NotTo(BeNil())
		})

	})

	Context("Handle sync run modified", func() {

		It("Ensure the sync run handler can handle a new sync run resource", func() {
			ctx := context.Background()

			scheme, argocdNamespace, kubesystemNamespace, workspace, err := eventlooptypes.GenericTestSetup()
			Expect(err).To(BeNil())

			gitopsDepl := &managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-gitops-depl",
					Namespace: workspace.Name,
					UID:       uuid.NewUUID(),
				},
			}

			gitopsDeplSyncRun := &managedgitopsv1alpha1.GitOpsDeploymentSyncRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-gitops-depl-sync",
					Namespace: workspace.Name,
					UID:       uuid.NewUUID(),
				},
				Spec: managedgitopsv1alpha1.GitOpsDeploymentSyncRunSpec{
					GitopsDeploymentName: gitopsDepl.Name,
					RevisionID:           "HEAD",
				},
			}

			informer := sharedutil.ListEventReceiver{}

			k8sClientOuter := fake.NewClientBuilder().WithScheme(scheme).WithObjects(gitopsDepl, gitopsDeplSyncRun, workspace, argocdNamespace, kubesystemNamespace).Build()
			k8sClient := &sharedutil.ProxyClient{
				InnerClient: k8sClientOuter,
				Informer:    &informer,
			}

			dbQueries, err := db.NewUnsafePostgresDBQueries(true, false)
			Expect(err).To(BeNil())

			opts := zap.Options{
				Development: true,
			}
			ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

			sharedResourceLoop := shared_resource_loop.NewSharedResourceLoop()

			// 1) send a deployment modified event, to ensure the deployment is added to the database, and processed
			a := applicationEventLoopRunner_Action{
				// When the code asks for a new k8s client, give it our fake client
				getK8sClientForGitOpsEngineInstance: func(gitopsEngineInstance *db.GitopsEngineInstance) (client.Client, error) {
					return k8sClient, nil
				},
				eventResourceName:           gitopsDepl.Name,
				eventResourceNamespace:      gitopsDepl.Namespace,
				workspaceClient:             k8sClient,
				log:                         log.FromContext(context.Background()),
				sharedResourceEventLoop:     sharedResourceLoop,
				workspaceID:                 string(workspace.UID),
				testOnlySkipCreateOperation: true,
			}
			_, _, _, err = a.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(err).To(BeNil())

			// 2) add a sync run modified event, to ensure the sync run is added to the database, and processed
			a = applicationEventLoopRunner_Action{
				// When the code asks for a new k8s client, give it our fake client
				getK8sClientForGitOpsEngineInstance: func(gitopsEngineInstance *db.GitopsEngineInstance) (client.Client, error) {
					return k8sClient, nil
				},
				eventResourceName:       gitopsDeplSyncRun.Name,
				eventResourceNamespace:  gitopsDeplSyncRun.Namespace,
				workspaceClient:         k8sClient,
				log:                     log.FromContext(context.Background()),
				sharedResourceEventLoop: sharedResourceLoop,
				workspaceID:             a.workspaceID,
			}
			_, err = a.applicationEventRunner_handleSyncRunModified(ctx, dbQueries)
			Expect(err).To(BeNil())
		})

		It("Ensure the sync run handler fails when an invalid new sync run resource is passed.", func() {
			ctx := context.Background()

			scheme, argocdNamespace, kubesystemNamespace, workspace, err := eventlooptypes.GenericTestSetup()
			Expect(err).To(BeNil())

			gitopsDepl := &managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-gitops-depl",
					Namespace: workspace.Name,
					UID:       uuid.NewUUID(),
				},
			}

			gitopsDeplSyncRun := &managedgitopsv1alpha1.GitOpsDeploymentSyncRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      strings.Repeat("abc", 100),
					Namespace: workspace.Name,
					UID:       uuid.NewUUID(),
				},
				Spec: managedgitopsv1alpha1.GitOpsDeploymentSyncRunSpec{
					GitopsDeploymentName: gitopsDepl.Name,
					RevisionID:           "HEAD",
				},
			}

			k8sClientOuter := fake.NewClientBuilder().WithScheme(scheme).WithObjects(gitopsDepl, gitopsDeplSyncRun, workspace, argocdNamespace, kubesystemNamespace).Build()
			k8sClient := &sharedutil.ProxyClient{
				InnerClient: k8sClientOuter,
			}

			dbQueries, err := db.NewUnsafePostgresDBQueries(true, false)
			Expect(err).To(BeNil())

			opts := zap.Options{
				Development: true,
			}
			ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

			sharedResourceLoop := shared_resource_loop.NewSharedResourceLoop()

			// 1) send a deployment modified event, to ensure the deployment is added to the database, and processed
			a := applicationEventLoopRunner_Action{
				// When the code asks for a new k8s client, give it our fake client
				getK8sClientForGitOpsEngineInstance: func(gitopsEngineInstance *db.GitopsEngineInstance) (client.Client, error) {
					return k8sClient, nil
				},
				eventResourceName:           gitopsDepl.Name,
				eventResourceNamespace:      gitopsDepl.Namespace,
				workspaceClient:             k8sClient,
				log:                         log.FromContext(context.Background()),
				sharedResourceEventLoop:     sharedResourceLoop,
				workspaceID:                 string(workspace.UID),
				testOnlySkipCreateOperation: true,
			}
			_, _, _, err = a.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(err).To(BeNil())

			// 2) add a sync run modified event, to ensure the sync run is added to the database, and processed
			a = applicationEventLoopRunner_Action{
				// When the code asks for a new k8s client, give it our fake client
				getK8sClientForGitOpsEngineInstance: func(gitopsEngineInstance *db.GitopsEngineInstance) (client.Client, error) {
					return k8sClient, nil
				},
				eventResourceName:       gitopsDeplSyncRun.Name,
				eventResourceNamespace:  gitopsDeplSyncRun.Namespace,
				workspaceClient:         k8sClient,
				log:                     log.FromContext(context.Background()),
				sharedResourceEventLoop: sharedResourceLoop,
				workspaceID:             a.workspaceID,
			}
			_, err = a.applicationEventRunner_handleSyncRunModified(ctx, dbQueries)
			Expect(err).NotTo(BeNil())

			gitopsDeployment := &managedgitopsv1alpha1.GitOpsDeployment{}
			gitopsDeploymentKey := client.ObjectKey{Namespace: gitopsDepl.Namespace, Name: gitopsDepl.Name}
			clientErr := a.workspaceClient.Get(ctx, gitopsDeploymentKey, gitopsDeployment)
			Expect(clientErr).To(BeNil())
			Expect(gitopsDeployment.Status.Conditions).NotTo(BeNil())
		})
	})

	Context("Handle deployment status.", func() {
		It("Should update correct status of deployment after calling DeploymentStatusTick handler.", func() {

			ctx := context.Background()

			scheme, argocdNamespace, kubesystemNamespace, workspace, err := eventlooptypes.GenericTestSetup()
			Expect(err).To(BeNil())

			gitopsDepl := &managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-gitops-depl",
					Namespace: workspace.Name,
					UID:       uuid.NewUUID(),
				},
			}

			informer := sharedutil.ListEventReceiver{}

			k8sClientOuter := fake.NewClientBuilder().WithScheme(scheme).WithObjects(gitopsDepl, workspace, argocdNamespace, kubesystemNamespace).Build()
			k8sClient := &sharedutil.ProxyClient{
				InnerClient: k8sClientOuter,
				Informer:    &informer,
			}

			workspaceID := string(workspace.UID)

			dbQueries, err := db.NewUnsafePostgresDBQueries(false, false)
			Expect(err).To(BeNil())

			a := applicationEventLoopRunner_Action{
				// When the code asks for a new k8s client, give it our fake client
				getK8sClientForGitOpsEngineInstance: func(gitopsEngineInstance *db.GitopsEngineInstance) (client.Client, error) {
					return k8sClient, nil
				},
				eventResourceName:           gitopsDepl.Name,
				eventResourceNamespace:      gitopsDepl.Namespace,
				workspaceClient:             k8sClient,
				log:                         log.FromContext(context.Background()),
				sharedResourceEventLoop:     shared_resource_loop.NewSharedResourceLoop(),
				workspaceID:                 workspaceID,
				testOnlySkipCreateOperation: true,
			}

			_, _, _, err = a.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(err).To(BeNil())

			// ----------------------------------------------------------------------------
			By("Get DeploymentToApplicationMapping and Application objects, to be used later")
			// ----------------------------------------------------------------------------
			var deplToAppMapping db.DeploymentToApplicationMapping
			{
				var appMappings []db.DeploymentToApplicationMapping

				err = dbQueries.ListDeploymentToApplicationMappingByNamespaceAndName(context.Background(), gitopsDepl.Name, gitopsDepl.Namespace, workspaceID, &appMappings)
				Expect(err).To(BeNil())

				Expect(len(appMappings)).To(Equal(1))

				deplToAppMapping = appMappings[0]
			}

			applicationState := &db.ApplicationState{Applicationstate_application_id: deplToAppMapping.Application_id}

			// ----------------------------------------------------------------------------
			By("Inserting dummy data into ApplicationState table, as we are not calling the reconciler, which updates the status of application into db.")
			// ----------------------------------------------------------------------------
			applicationState.Health = string(managedgitopsv1alpha1.HeathStatusCodeHealthy)
			applicationState.Message = "Success"
			applicationState.Sync_Status = string(managedgitopsv1alpha1.SyncStatusCodeSynced)
			applicationState.Revision = "abcdefg"

			err = dbQueries.CreateApplicationState(ctx, applicationState)
			Expect(err).To(BeNil())

			// ----------------------------------------------------------------------------
			By("Retrieve latest version of GitObsDeployment and check Health/Synk, before calling applicationEventRunner_handleUpdateDeploymentStatusTick function.")
			// ----------------------------------------------------------------------------

			gitopsDeploymentSecond := &managedgitopsv1alpha1.GitOpsDeployment{}
			gitopsDeploymentKey := client.ObjectKey{Namespace: gitopsDepl.Namespace, Name: gitopsDepl.Name}
			clientErr := a.workspaceClient.Get(ctx, gitopsDeploymentKey, gitopsDeploymentSecond)

			Expect(clientErr).To(BeNil())
			Expect(gitopsDeploymentSecond.Status.Health.Status).To(BeEmpty())
			Expect(gitopsDeploymentSecond.Status.Health.Message).To(BeEmpty())
			Expect(gitopsDeploymentSecond.Status.Sync.Status).To(BeEmpty())
			Expect(gitopsDeploymentSecond.Status.Sync.Revision).To(BeEmpty())

			// ----------------------------------------------------------------------------
			By("Call applicationEventRunner_handleUpdateDeploymentStatusTick function to update Helth/Synk status.")
			// ----------------------------------------------------------------------------

			err = a.applicationEventRunner_handleUpdateDeploymentStatusTick(ctx, string(gitopsDepl.UID), dbQueries)
			Expect(err).To(BeNil())

			// ----------------------------------------------------------------------------
			By("Retrieve latest version of GitObsDeployment and check Health/Synk, after calling applicationEventRunner_handleUpdateDeploymentStatusTick function.")
			// ----------------------------------------------------------------------------
			gitopsDeploymentThird := &managedgitopsv1alpha1.GitOpsDeployment{}
			gitopsDeploymentKeyThird := client.ObjectKey{Namespace: gitopsDepl.Namespace, Name: gitopsDepl.Name}
			clientErr = a.workspaceClient.Get(ctx, gitopsDeploymentKeyThird, gitopsDeploymentThird)

			Expect(clientErr).To(BeNil())
			Expect(gitopsDeploymentThird.Status.Health.Status).To(Equal(managedgitopsv1alpha1.HeathStatusCodeHealthy))
			Expect(gitopsDeploymentThird.Status.Health.Message).To(Equal("Success"))
			Expect(gitopsDeploymentThird.Status.Sync.Status).To(Equal(managedgitopsv1alpha1.SyncStatusCodeSynced))
			Expect(gitopsDeploymentThird.Status.Sync.Revision).To(Equal("abcdefg"))

			// ----------------------------------------------------------------------------
			By("Delete the GitOpsDepl.")
			// ----------------------------------------------------------------------------
			err = k8sClient.Delete(ctx, gitopsDepl)
			Expect(err).To(BeNil())

			_, _, _, err = a.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(err).To(BeNil())
		})

		It("Should not return any error, if deploymentToApplicationMapping doesn't exist for given gitopsdeployment.", func() {

			ctx := context.Background()

			_, _, _, workspace, err := eventlooptypes.GenericTestSetup()
			Expect(err).To(BeNil())

			k8sClientOuter := fake.NewClientBuilder().Build()
			k8sClient := &sharedutil.ProxyClient{
				InnerClient: k8sClientOuter,
			}

			workspaceID := string(workspace.UID)

			dbQueries, err := db.NewUnsafePostgresDBQueries(false, false)
			Expect(err).To(BeNil())

			a := applicationEventLoopRunner_Action{
				getK8sClientForGitOpsEngineInstance: func(gitopsEngineInstance *db.GitopsEngineInstance) (client.Client, error) {
					return k8sClient, nil
				},
				eventResourceName:           "dummy-deployment",
				eventResourceNamespace:      workspace.Namespace,
				workspaceClient:             k8sClient,
				log:                         log.FromContext(context.Background()),
				sharedResourceEventLoop:     shared_resource_loop.NewSharedResourceLoop(),
				workspaceID:                 workspaceID,
				testOnlySkipCreateOperation: true,
			}

			// ----------------------------------------------------------------------------
			By("Call applicationEventRunner_handleUpdateDeploymentStatusTick function to update Helth/Synk status.")
			// ----------------------------------------------------------------------------
			err = a.applicationEventRunner_handleUpdateDeploymentStatusTick(ctx, "dummy-deployment", dbQueries)
			Expect(err).To(BeNil())
		})

		It("Should return error if deploymentToApplicationMapping object is not valid.", func() {

			ctx := context.Background()

			_, _, _, workspace, err := eventlooptypes.GenericTestSetup()
			Expect(err).To(BeNil())

			k8sClientOuter := fake.NewClientBuilder().Build()
			k8sClient := &sharedutil.ProxyClient{
				InnerClient: k8sClientOuter,
			}

			workspaceID := string(workspace.UID)

			dbQueries, err := db.NewUnsafePostgresDBQueries(false, false)
			Expect(err).To(BeNil())

			a := applicationEventLoopRunner_Action{
				getK8sClientForGitOpsEngineInstance: func(gitopsEngineInstance *db.GitopsEngineInstance) (client.Client, error) {
					return k8sClient, nil
				},
				eventResourceName:           "dummy-deployment",
				eventResourceNamespace:      workspace.Namespace,
				workspaceClient:             k8sClient,
				log:                         log.FromContext(context.Background()),
				sharedResourceEventLoop:     shared_resource_loop.NewSharedResourceLoop(),
				workspaceID:                 workspaceID,
				testOnlySkipCreateOperation: true,
			}

			// ----------------------------------------------------------------------------
			By("Call applicationEventRunner_handleUpdateDeploymentStatusTick function to update Helth/Synk status.")
			// ----------------------------------------------------------------------------
			err = a.applicationEventRunner_handleUpdateDeploymentStatusTick(ctx, "", dbQueries)
			Expect(err).NotTo(BeNil())
			Expect(strings.Contains(err.Error(), "field should not be empty string")).To(BeTrue())
		})

		It("Should not return error, if GitOpsDeployment doesn't exist in given namespace.", func() {

			ctx := context.Background()

			scheme, argocdNamespace, kubesystemNamespace, workspace, err := eventlooptypes.GenericTestSetup()
			Expect(err).To(BeNil())

			gitopsDepl := &managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-gitops-depl",
					Namespace: workspace.Name,
					UID:       uuid.NewUUID(),
				},
			}

			k8sClientOuter := fake.NewClientBuilder().WithScheme(scheme).WithObjects(gitopsDepl, workspace, argocdNamespace, kubesystemNamespace).Build()
			k8sClient := &sharedutil.ProxyClient{
				InnerClient: k8sClientOuter,
			}

			workspaceID := string(workspace.UID)

			dbQueries, err := db.NewUnsafePostgresDBQueries(false, false)
			Expect(err).To(BeNil())

			a := applicationEventLoopRunner_Action{
				// When the code asks for a new k8s client, give it our fake client
				getK8sClientForGitOpsEngineInstance: func(gitopsEngineInstance *db.GitopsEngineInstance) (client.Client, error) {
					return k8sClient, nil
				},
				eventResourceName:           gitopsDepl.Name,
				eventResourceNamespace:      gitopsDepl.Namespace,
				workspaceClient:             k8sClient,
				log:                         log.FromContext(context.Background()),
				sharedResourceEventLoop:     shared_resource_loop.NewSharedResourceLoop(),
				workspaceID:                 workspaceID,
				testOnlySkipCreateOperation: true,
			}

			// ----------------------------------------------------------------------------
			By("Create new deployment.")
			// ----------------------------------------------------------------------------
			_, _, _, err = a.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(err).To(BeNil())

			// ----------------------------------------------------------------------------
			By("Delete deployment, but we don't want to delete other DB entries, hence not calling applicationEventRunner_handleDeploymentModified after deleting deployment.")
			// ----------------------------------------------------------------------------
			err = k8sClient.Delete(ctx, gitopsDepl)
			Expect(err).To(BeNil())

			// ----------------------------------------------------------------------------
			By("Call applicationEventRunner_handleUpdateDeploymentStatusTick function to update Helth/Synk status.")
			// ----------------------------------------------------------------------------
			err = a.applicationEventRunner_handleUpdateDeploymentStatusTick(ctx, string(gitopsDepl.UID), dbQueries)
			Expect(err).To(BeNil())

			// ----------------------------------------------------------------------------
			By("Deployment is deleted in previous step, hence delete currespnding db entries.")
			// ----------------------------------------------------------------------------
			_, _, _, err = a.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(err).To(BeNil())
		})

		It("Should not return error, if corresponding ApplicationState doesnt exists for given GitOpsDeployment.", func() {

			ctx := context.Background()

			scheme, argocdNamespace, kubesystemNamespace, workspace, err := eventlooptypes.GenericTestSetup()
			Expect(err).To(BeNil())

			gitopsDepl := &managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-gitops-depl",
					Namespace: workspace.Name,
					UID:       uuid.NewUUID(),
				},
			}

			k8sClientOuter := fake.NewClientBuilder().WithScheme(scheme).WithObjects(gitopsDepl, workspace, argocdNamespace, kubesystemNamespace).Build()
			k8sClient := &sharedutil.ProxyClient{
				InnerClient: k8sClientOuter,
			}

			workspaceID := string(workspace.UID)

			dbQueries, err := db.NewUnsafePostgresDBQueries(false, false)
			Expect(err).To(BeNil())

			a := applicationEventLoopRunner_Action{
				// When the code asks for a new k8s client, give it our fake client
				getK8sClientForGitOpsEngineInstance: func(gitopsEngineInstance *db.GitopsEngineInstance) (client.Client, error) {
					return k8sClient, nil
				},
				eventResourceName:           gitopsDepl.Name,
				eventResourceNamespace:      gitopsDepl.Namespace,
				workspaceClient:             k8sClient,
				log:                         log.FromContext(context.Background()),
				sharedResourceEventLoop:     shared_resource_loop.NewSharedResourceLoop(),
				workspaceID:                 workspaceID,
				testOnlySkipCreateOperation: true,
			}

			_, _, _, err = a.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(err).To(BeNil())

			// ----------------------------------------------------------------------------
			By("Call applicationEventRunner_handleUpdateDeploymentStatusTick function to update Helth/Synk status.")
			// ----------------------------------------------------------------------------
			err = a.applicationEventRunner_handleUpdateDeploymentStatusTick(ctx, string(gitopsDepl.UID), dbQueries)
			Expect(err).To(BeNil())

			// ----------------------------------------------------------------------------
			By("Delete the GitOpsDepl.")
			// ----------------------------------------------------------------------------

			err = k8sClient.Delete(ctx, gitopsDepl)
			Expect(err).To(BeNil())

			_, _, _, err = a.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(err).To(BeNil())
		})

	})
})

var _ = Describe("GitOpsDeployment Conditions", func() {
	var (
		adapter          *gitOpsDeploymentAdapter
		mockCtrl         *gomock.Controller
		mockClient       *mocks.MockClient
		mockStatusWriter *mocks.MockStatusWriter
		gitopsDeployment *managedgitopsv1alpha1.GitOpsDeployment
		mockConditions   *condition.MockConditions
		ctx              context.Context
	)

	BeforeEach(func() {
		gitopsDeployment = testStructs.NewGitOpsDeploymentBuilder().Initialized().GetGitopsDeployment()
		mockCtrl = gomock.NewController(GinkgoT())
		mockClient = mocks.NewMockClient(mockCtrl)
		mockConditions = condition.NewMockConditions(mockCtrl)
		mockStatusWriter = mocks.NewMockStatusWriter(mockCtrl)
	})
	JustBeforeEach(func() {
		adapter = newGitOpsDeploymentAdapter(gitopsDeployment, log.Log.WithName("Test Logger"), mockClient, mockConditions, ctx)
	})
	AfterEach(func() {
		mockCtrl.Finish()
	})

	Context("setGitopsDeploymentCondition()", func() {
		var (
			err           = errors.New("fake reconcile")
			reason        = managedgitopsv1alpha1.GitOpsDeploymentReasonType("ReconcileError")
			conditionType = managedgitopsv1alpha1.GitOpsDeploymentConditionErrorOccurred
		)
		Context("when no conditions defined before and the err is nil", func() {
			BeforeEach(func() {
				mockConditions.EXPECT().HasCondition(gomock.Any(), conditionType).Return(false)
			})
			It("It returns nil ", func() {
				errTemp := adapter.setGitOpsDeploymentCondition(conditionType, reason, nil)
				Expect(errTemp).To(BeNil())
			})
		})
		Context("when the err comes from reconcileHandler", func() {
			It("should update the CR", func() {
				matcher := testStructs.NewGitopsDeploymentMatcher()
				mockClient.EXPECT().Status().Return(mockStatusWriter)
				mockStatusWriter.EXPECT().Update(gomock.Any(), matcher, gomock.Any())
				mockConditions.EXPECT().SetCondition(gomock.Any(), conditionType, managedgitopsv1alpha1.GitOpsConditionStatusTrue, reason, err.Error()).Times(1)
				err := adapter.setGitOpsDeploymentCondition(conditionType, reason, err)
				Expect(err).NotTo(HaveOccurred())
			})
		})
		Context("when the err has been resolved", func() {
			BeforeEach(func() {
				mockConditions.EXPECT().HasCondition(gomock.Any(), conditionType).Return(true)
				mockConditions.EXPECT().FindCondition(gomock.Any(), conditionType).Return(&managedgitopsv1alpha1.GitOpsDeploymentCondition{}, true)
			})
			It("It should update the CR condition status as resolved", func() {
				matcher := testStructs.NewGitopsDeploymentMatcher()
				conditions := &gitopsDeployment.Status.Conditions
				*conditions = append(*conditions, managedgitopsv1alpha1.GitOpsDeploymentCondition{})
				mockClient.EXPECT().Status().Return(mockStatusWriter)
				mockStatusWriter.EXPECT().Update(gomock.Any(), matcher, gomock.Any())
				mockConditions.EXPECT().SetCondition(conditions, conditionType, managedgitopsv1alpha1.GitOpsConditionStatusFalse, managedgitopsv1alpha1.GitOpsDeploymentReasonType("ReconcileErrorResolved"), "").Times(1)
				err := adapter.setGitOpsDeploymentCondition(conditionType, reason, nil)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})
})

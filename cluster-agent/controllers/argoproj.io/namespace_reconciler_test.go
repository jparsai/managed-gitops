package argoprojio

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
	dbutil "github.com/redhat-appstudio/managed-gitops/backend-shared/config/db/util"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/eventlooptypes"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"

	appv1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
)

var _ = Describe("Namespace Reconciler Tests.", func() {
	var reconciler ApplicationReconciler

	Context("Testing for Namespace Reconciler.", func() {
		It("Should consider ArgoCD Application as an orphaned and delete it, if application entry doesnt exists in DB.", func() {
			ctx := context.Background()
			log := log.FromContext(ctx)

			scheme, _, _, _, err := eventlooptypes.GenericTestSetup()
			Expect(err).To(BeNil())

			err = appv1.AddToScheme(scheme)
			Expect(err).To(BeNil())

			// Fake kube client.
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			reconciler := ApplicationReconciler{Client: k8sClient}

			argoApplications := []appv1.Application{
				{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"databaseID": "test-my-application-1"}}},
				{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"databaseID": "test-my-application-2"}}},
				{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"databaseID": "test-my-application-3"}}},
				{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"databaseID": "test-my-application-4"}}},
				{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"databaseID": "test-my-application-5"}}},
			}

			processedApplicationIds := map[string]interface{}{"test-my-application-3": false, "test-my-application-5": false}

			deletedArgoApplications := deleteOrphanedApplications(argoApplications, processedApplicationIds, ctx, reconciler.Client, log)

			Expect(len(deletedArgoApplications)).To(Equal(3))

			deletedApplicationIds := map[string]string{"test-my-application-1": "", "test-my-application-2": "", "test-my-application-4": ""}
			for _, app := range deletedArgoApplications {
				_, ok := deletedApplicationIds[app.Labels["databaseID"]]
				Expect(ok).To(BeTrue())
			}
		})
	})

	Context("Testing for CompareApplications function.", func() {
		It("Should compare applications.", func() {

			applicationFromDB, _, applicationFromArgoCD, err := createDummyApplicationData()
			Expect(err).To(BeNil())

			var ctx context.Context
			log := log.FromContext(ctx)

			result := compareApplications(applicationFromArgoCD, applicationFromDB, log)
			Expect(result).To(BeFalse())

			// Set different value in each field then revert them, otherwise next field wont be compared
			applicationFromArgoCD.APIVersion = "test"
			result = compareApplications(applicationFromArgoCD, applicationFromDB, log)
			Expect(result).To(BeTrue())
			applicationFromArgoCD.APIVersion = applicationFromDB.APIVersion // Revert the value, to compare next field.

			applicationFromArgoCD.Kind = "test"
			result = compareApplications(applicationFromArgoCD, applicationFromDB, log)
			Expect(result).To(BeTrue())
			applicationFromArgoCD.Kind = applicationFromDB.Kind

			applicationFromArgoCD.Name = "test"
			result = compareApplications(applicationFromArgoCD, applicationFromDB, log)
			Expect(result).To(BeTrue())
			applicationFromArgoCD.Name = applicationFromDB.Name

			applicationFromArgoCD.Namespace = "test"
			result = compareApplications(applicationFromArgoCD, applicationFromDB, log)
			Expect(result).To(BeTrue())
			applicationFromArgoCD.Namespace = applicationFromDB.Namespace

			applicationFromArgoCD.Spec.Source.RepoURL = "test"
			result = compareApplications(applicationFromArgoCD, applicationFromDB, log)
			Expect(result).To(BeTrue())
			applicationFromArgoCD.Spec.Source.RepoURL = applicationFromDB.Spec.Source.RepoURL

			applicationFromArgoCD.Spec.Source.Path = "test"
			result = compareApplications(applicationFromArgoCD, applicationFromDB, log)
			Expect(result).To(BeTrue())
			applicationFromArgoCD.Spec.Source.Path = applicationFromDB.Spec.Source.Path

			applicationFromArgoCD.Spec.Source.TargetRevision = "test"
			result = compareApplications(applicationFromArgoCD, applicationFromDB, log)
			Expect(result).To(BeTrue())
			applicationFromArgoCD.Spec.Source.TargetRevision = applicationFromDB.Spec.Source.TargetRevision

			applicationFromArgoCD.Spec.Destination.Server = "test"
			result = compareApplications(applicationFromArgoCD, applicationFromDB, log)
			Expect(result).To(BeTrue())
			applicationFromArgoCD.Spec.Destination.Server = applicationFromDB.Spec.Destination.Server

			applicationFromArgoCD.Spec.Destination.Namespace = "test"
			result = compareApplications(applicationFromArgoCD, applicationFromDB, log)
			Expect(result).To(BeTrue())
			applicationFromArgoCD.Spec.Destination.Namespace = applicationFromDB.Spec.Destination.Namespace

			applicationFromArgoCD.Spec.Destination.Name = "test"
			result = compareApplications(applicationFromArgoCD, applicationFromDB, log)
			Expect(result).To(BeTrue())
			applicationFromArgoCD.Spec.Destination.Name = applicationFromDB.Spec.Destination.Name

			applicationFromArgoCD.Spec.Project = "test"
			result = compareApplications(applicationFromArgoCD, applicationFromDB, log)
			Expect(result).To(BeTrue())
			applicationFromArgoCD.Spec.Project = applicationFromDB.Spec.Project

			applicationFromArgoCD.Spec.SyncPolicy.Automated.Prune = true
			result = compareApplications(applicationFromArgoCD, applicationFromDB, log)
			Expect(result).To(BeTrue())
			applicationFromArgoCD.Spec.SyncPolicy.Automated.Prune = applicationFromDB.Spec.SyncPolicy.Automated.Prune

			applicationFromArgoCD.Spec.SyncPolicy.Automated.SelfHeal = true
			result = compareApplications(applicationFromArgoCD, applicationFromDB, log)
			Expect(result).To(BeTrue())
			applicationFromArgoCD.Spec.SyncPolicy.Automated.SelfHeal = applicationFromDB.Spec.SyncPolicy.Automated.SelfHeal

			applicationFromArgoCD.Spec.SyncPolicy.Automated.AllowEmpty = true
			result = compareApplications(applicationFromArgoCD, applicationFromDB, log)
			Expect(result).To(BeTrue())
			applicationFromArgoCD.Spec.SyncPolicy.Automated.AllowEmpty = applicationFromDB.Spec.SyncPolicy.Automated.AllowEmpty
		})
	})

	Context("Testing CleanK8sOperations function", func() {
		var err error
		var dbQueries db.AllDatabaseQueries
		var ctx context.Context
		var operationList []db.Operation
		var argoCdApp appv1.Application
		var dummyApplicationSpec string
		var applicationput db.Application

		BeforeEach(func() {
			ctx = context.Background()

			err = db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())

			dbQueries, err = db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())

			_, managedEnvironment, _, gitopsEngineInstance, _, err := db.CreateSampleData(dbQueries)
			Expect(err).To(BeNil())

			scheme, argocdNamespace, kubesystemNamespace, workspace, err := eventlooptypes.GenericTestSetup()
			Expect(err).To(BeNil())

			_, dummyApplicationSpec, argoCdApp, err = createDummyApplicationData()
			Expect(err).To(BeNil())

			applicationput = db.Application{
				Application_id:          "test-my-application",
				Name:                    "test-my-application",
				Spec_field:              dummyApplicationSpec,
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			}

			err = dbQueries.CreateApplication(ctx, &applicationput)
			Expect(err).To(BeNil())

			err = appv1.AddToScheme(scheme)
			Expect(err).To(BeNil())

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
				Cache:  dbutil.NewApplicationInfoCache(),
			}

			err = reconciler.Create(ctx, &argoCdApp)
			Expect(err).To(BeNil())

			var speCialClusterUser db.ClusterUser
			err = dbQueries.GetOrCreateSpecialClusterUser(context.Background(), &speCialClusterUser)
			Expect(err).To(BeNil())
		})

		AfterEach(func() {
			for _, operation := range operationList {
				rowsAffected, err := dbQueries.CheckedDeleteOperationById(ctx, operation.Operation_id, operation.Operation_owner_user_id)
				Expect(rowsAffected).Should((Equal(1)))
				Expect(err).To(BeNil())
			}
			// Empty Operation List
			operationList = []db.Operation{}
		})

		It("Should delete Operations from cluster and if operation is completed.", func() {

			ctx := context.Background()
			log := log.FromContext(ctx)

			dbOperationInput := db.Operation{
				Instance_id:   applicationput.Engine_instance_inst_id,
				Resource_id:   applicationput.Application_id,
				Resource_type: db.OperationResourceType_Application,
			}

			_, dbOperation, err := eventlooptypes.CreateOperation(ctx, false, dbOperationInput,
				db.SpecialClusterUserName, dbutil.GetGitOpsEngineSingleInstanceNamespace(), reconciler.DB, reconciler.Client, log)
			Expect(err).To(BeNil())

			dbOperation.State = "Completed"
			err = dbQueries.UpdateOperation(ctx, dbOperation)
			Expect(err).To(BeNil())

			operationList = append(operationList, *dbOperation)

			// Get list of Operations before cleanup.
			listOfK8sOperationFirst := v1alpha1.OperationList{}
			err = reconciler.List(ctx, &listOfK8sOperationFirst)
			Expect(err).To(BeNil())
			Expect(len(listOfK8sOperationFirst.Items)).NotTo(Equal(0))

			// Clean Operations
			cleanK8sOperations(ctx, dbQueries, reconciler.Client, log)

			// Get list of Operations after cleanup.
			listOfK8sOperationSecond := v1alpha1.OperationList{}
			err = reconciler.List(ctx, &listOfK8sOperationSecond)
			Expect(err).To(BeNil())
			Expect(len(listOfK8sOperationSecond.Items)).To(Equal(0))
		})

		It("Should not delete Operations from cluster and if operation is not completed.", func() {

			ctx := context.Background()
			log := log.FromContext(ctx)

			dbOperationInput := db.Operation{
				Instance_id:   applicationput.Engine_instance_inst_id,
				Resource_id:   applicationput.Application_id,
				Resource_type: db.OperationResourceType_Application,
			}

			_, dbOperation, err := eventlooptypes.CreateOperation(ctx, false, dbOperationInput,
				db.SpecialClusterUserName, dbutil.GetGitOpsEngineSingleInstanceNamespace(), reconciler.DB, reconciler.Client, log)
			Expect(err).To(BeNil())

			operationList = append(operationList, *dbOperation)

			// Get list of Operations before cleanup.
			listOfK8sOperationFirst := v1alpha1.OperationList{}
			err = reconciler.List(ctx, &listOfK8sOperationFirst)
			Expect(err).To(BeNil())
			Expect(len(listOfK8sOperationFirst.Items)).NotTo(Equal(0))

			// Clean Operations
			cleanK8sOperations(ctx, dbQueries, reconciler.Client, log)

			// Get list of Operations after cleanup.
			listOfK8sOperationSecond := v1alpha1.OperationList{}
			err = reconciler.List(ctx, &listOfK8sOperationSecond)
			Expect(err).To(BeNil())
			Expect(len(listOfK8sOperationSecond.Items)).NotTo(Equal(0))
		})
	})
})

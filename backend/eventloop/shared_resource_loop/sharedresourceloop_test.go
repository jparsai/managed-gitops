package shared_resource_loop

import (
	"context"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/eventlooptypes"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"testing"
)

// Used to list down resources for deletion which are created while running tests.
type testResources struct {
	clusterAccess           *db.ClusterAccess
	Clusteruser_id          string
	Managedenvironment_id   string
	Gitopsengineinstance_id string
	EngineCluster_id        string
	Clustercredentials_id   []string
}

func TestGetOrCreateClusterUserByNamespaceUID(t *testing.T) {
	// Create a fake K8s client
	ctx := context.Background()

	scheme,
		argocdNamespace,
		kubesystemNamespace,
		workspace := eventlooptypes.GenericTestSetup(t)

	gitopsDepl := &managedgitopsv1alpha1.GitOpsDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-gitops-depl",
			Namespace: workspace.Name,
			UID:       uuid.NewUUID(),
		},
	}

	informer := sharedutil.ListEventReceiver{}

	k8sClientOuter := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(gitopsDepl, workspace, argocdNamespace, kubesystemNamespace).
		Build()

	k8sClient := &sharedutil.ProxyClient{
		InnerClient: k8sClientOuter,
		Informer:    &informer,
	}

	sharedResourceEventLoop := &SharedResourceEventLoop{inputChannel: make(chan sharedResourceLoopMessage)}

	go internalSharedResourceEventLoop(sharedResourceEventLoop.inputChannel)

	// At first assuming there are no existing users, hence creating new.
	usrOld,
		err,
		isNewUser := sharedResourceEventLoop.GetOrCreateClusterUserByNamespaceUID(ctx, k8sClient, *workspace)

	// Check error in Nil and user is not Nil
	assert.Nil(t, err)
	assert.NotNil(t, usrOld)

	// Check new user is created
	assert.True(t, isNewUser)

	// User is created in previous call, then same user should be returned instead of creating new.
	usrNew,
		err,
		isNewUser := sharedResourceEventLoop.GetOrCreateClusterUserByNamespaceUID(ctx, k8sClient, *workspace)

	// Check error in Nil and user is not Nil
	assert.Nil(t, err)
	assert.NotNil(t, usrNew)

	// Check user is not recreated
	assert.False(t, isNewUser)

	// Check old and new users are same
	assert.Equal(t, usrOld, usrNew)

	// Delete user
	resourcesToBeDeleted := testResources{Clusteruser_id: usrNew.Clusteruser_id}

	// Run even in case of failure
	defer tearDownResources(t, ctx, resourcesToBeDeleted)
}

func TestGetOrCreateSharedResources(t *testing.T) {
	// Create a fake K8s client
	ctx := context.Background()

	scheme,
		argocdNamespace,
		kubesystemNamespace,
		workspace := eventlooptypes.GenericTestSetup(t)

	gitopsDepl := &managedgitopsv1alpha1.GitOpsDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-gitops-depl",
			Namespace: workspace.Name,
			UID:       uuid.NewUUID(),
		},
	}

	informer := sharedutil.ListEventReceiver{}

	k8sClientOuter := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(gitopsDepl, workspace, argocdNamespace, kubesystemNamespace).
		Build()

	k8sClient := &sharedutil.ProxyClient{
		InnerClient: k8sClientOuter,
		Informer:    &informer,
	}

	sharedResourceEventLoop := &SharedResourceEventLoop{inputChannel: make(chan sharedResourceLoopMessage)}

	go internalSharedResourceEventLoop(sharedResourceEventLoop.inputChannel)

	// At first assuming there are no existing resources, hence creating new.
	clusterUserOld,
		isNewUser,
		managedEnvOld,
		isNewManagedEnv,
		gitopsEngineInstanceOld,
		isNewInstance,
		clusterAccessOld,
		isNewClusterAccess,
		gitopsEngineClusterOld,
		err := sharedResourceEventLoop.GetOrCreateSharedResources(ctx, k8sClient, *workspace)

	// Check if result is not Nil and error is Nil
	assert.Nil(t, err)
	assert.NotNil(t, clusterUserOld)
	assert.NotNil(t, managedEnvOld)
	assert.NotNil(t, gitopsEngineInstanceOld)
	assert.NotNil(t, clusterAccessOld)

	// Check if new resources are created
	assert.True(t, isNewUser)
	assert.True(t, isNewManagedEnv)
	assert.True(t, isNewInstance)
	assert.True(t, isNewClusterAccess)

	// Resources are created in previous call, then same resources should be returned instead of creating new.
	clusterUserNew,
		isNewUser,
		managedEnvNew,
		isNewManagedEnv,
		gitopsEngineInstanceNew,
		isNewInstance,
		clusterAccessNew,
		isNewClusterAccess,
		_, err := sharedResourceEventLoop.GetOrCreateSharedResources(ctx, k8sClient, *workspace)

	// Check if result is not Nil and error is Nil
	assert.NotNil(t, clusterUserNew)
	assert.NotNil(t, managedEnvNew)
	assert.NotNil(t, gitopsEngineInstanceNew)
	assert.NotNil(t, clusterAccessNew)
	assert.Nil(t, err)

	// Check resources are not recreated
	assert.False(t, isNewUser)
	assert.False(t, isNewManagedEnv)
	assert.False(t, isNewInstance)
	assert.False(t, isNewClusterAccess)

	// Check if old and new resources are same
	assert.Equal(t, clusterUserOld, clusterUserNew)
	assert.Equal(t, managedEnvOld, managedEnvNew)
	assert.Equal(t, gitopsEngineInstanceOld, gitopsEngineInstanceNew)
	assert.Equal(t, clusterAccessOld, clusterAccessNew)

	// Delete resources
	resourcesToBeDeleted := testResources{
		Clusteruser_id:          clusterUserOld.Clusteruser_id,
		Managedenvironment_id:   managedEnvOld.Managedenvironment_id,
		Gitopsengineinstance_id: gitopsEngineInstanceOld.Gitopsengineinstance_id,
		clusterAccess:           clusterAccessOld,
		EngineCluster_id:        gitopsEngineInstanceOld.EngineCluster_id,
		Clustercredentials_id: []string{
			gitopsEngineClusterOld.Clustercredentials_id,
			managedEnvOld.Clustercredentials_id,
		},
	}

	// Run even in case of failure
	defer tearDownResources(t, ctx, resourcesToBeDeleted)
}

func TestGetGitopsEngineInstanceById(t *testing.T) {
	// Create a fake K8s client
	ctx := context.Background()

	scheme,
		argocdNamespace,
		kubesystemNamespace,
		workspace := eventlooptypes.GenericTestSetup(t)

	gitopsDepl := &managedgitopsv1alpha1.GitOpsDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-gitops-depl",
			Namespace: workspace.Name,
			UID:       uuid.NewUUID(),
		},
	}

	informer := sharedutil.ListEventReceiver{}

	k8sClientOuter := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(gitopsDepl, workspace, argocdNamespace, kubesystemNamespace).
		Build()

	k8sClient := &sharedutil.ProxyClient{
		InnerClient: k8sClientOuter,
		Informer:    &informer,
	}

	sharedResourceEventLoop := &SharedResourceEventLoop{inputChannel: make(chan sharedResourceLoopMessage)}

	go internalSharedResourceEventLoop(sharedResourceEventLoop.inputChannel)

	// Negative test, engineInstance is not present, it should return error
	engineInstanceOld, err := sharedResourceEventLoop.GetGitopsEngineInstanceById(ctx, "", k8sClient, *workspace)
	assert.NotNil(t, err)
	assert.True(t, engineInstanceOld.EngineCluster_id == "")

	// Create new engine instance which will be used by "GetGitopsEngineInstanceById" fucntion
	dbq, err := db.NewUnsafePostgresDBQueries(true, true)

	if !assert.NoError(t, err) {
		return
	}
	defer dbq.CloseDatabase()

	clusterCredentials := db.ClusterCredentials{
		Clustercredentials_cred_id: string(uuid.NewUUID()),
	}

	gitopsEngineCluster := db.GitopsEngineCluster{
		Gitopsenginecluster_id: string(uuid.NewUUID()),
		Clustercredentials_id:  clusterCredentials.Clustercredentials_cred_id,
	}

	gitopsEngineInstance := db.GitopsEngineInstance{
		Gitopsengineinstance_id: string(uuid.NewUUID()),
		Namespace_name:          workspace.Namespace,
		Namespace_uid:           string(workspace.UID),
		EngineCluster_id:        gitopsEngineCluster.Gitopsenginecluster_id,
	}

	err = dbq.CreateClusterCredentials(ctx, &clusterCredentials)
	if !assert.NoError(t, err) {
		return
	}

	err = dbq.CreateGitopsEngineCluster(ctx, &gitopsEngineCluster)
	if !assert.NoError(t, err) {
		return
	}

	err = dbq.CreateGitopsEngineInstance(ctx, &gitopsEngineInstance)
	if !assert.NoError(t, err) {
		return
	}

	// Fetch the same engineInstance by ID
	engineInstanceNew, err := sharedResourceEventLoop.GetGitopsEngineInstanceById(ctx, gitopsEngineInstance.Gitopsengineinstance_id, k8sClient, *workspace)

	// Check error is Nil and engineInstance is returned
	assert.Nil(t, err)
	assert.NotNil(t, engineInstanceNew.EngineCluster_id)

	// Delete resources
	resourcesToBeDeleted := testResources{
		Gitopsengineinstance_id: gitopsEngineInstance.Gitopsengineinstance_id,
		EngineCluster_id:        gitopsEngineInstance.EngineCluster_id,
		Clustercredentials_id: []string{
			clusterCredentials.Clustercredentials_cred_id,
		},
	}

	// Run even in case of failure
	defer tearDownResources(t, ctx, resourcesToBeDeleted)
}

func tearDownResources(t *testing.T, ctx context.Context, resourcesToBeDeleted testResources) {

	dbq, err := db.NewUnsafePostgresDBQueries(true, true)

	if !assert.NoError(t, err) {
		return
	}
	defer dbq.CloseDatabase()

	// Delete clusterAccess
	if resourcesToBeDeleted.clusterAccess != nil {
		rowsAffected, err := dbq.DeleteClusterAccessById(ctx,
			resourcesToBeDeleted.clusterAccess.Clusteraccess_user_id,
			resourcesToBeDeleted.clusterAccess.Clusteraccess_managed_environment_id,
			resourcesToBeDeleted.clusterAccess.Clusteraccess_gitops_engine_instance_id)
		assert.Equal(t, rowsAffected, 1)
		assert.NoError(t, err)
	}

	// Delete clusterUser
	if resourcesToBeDeleted.Clusteruser_id != "" {
		rowsAffected, err := dbq.DeleteClusterUserById(ctx, resourcesToBeDeleted.Clusteruser_id)
		assert.Equal(t, rowsAffected, 1)
		assert.NoError(t, err)
	}

	// Delete managedEnv
	if resourcesToBeDeleted.Managedenvironment_id != "" {
		rowsAffected, err := dbq.DeleteManagedEnvironmentById(ctx, resourcesToBeDeleted.Managedenvironment_id)
		assert.Equal(t, rowsAffected, 1)
		assert.NoError(t, err)
	}

	// Delete engineInstance
	if resourcesToBeDeleted.Gitopsengineinstance_id != "" {
		rowsAffected, err := dbq.DeleteGitopsEngineInstanceById(ctx, resourcesToBeDeleted.Gitopsengineinstance_id)
		assert.Equal(t, rowsAffected, 1)
		assert.NoError(t, err)
	}

	// Delete engineCluster
	if resourcesToBeDeleted.EngineCluster_id != "" {
		rowsAffected, err := dbq.DeleteGitopsEngineClusterById(ctx, resourcesToBeDeleted.EngineCluster_id)
		assert.Equal(t, rowsAffected, 1)
		assert.NoError(t, err)
	}

	// Delete clusterCredentials
	if len(resourcesToBeDeleted.Clustercredentials_id) != 0 {
		for _, clustercredentials_id := range resourcesToBeDeleted.Clustercredentials_id {
			rowsAffected, err := dbq.DeleteClusterCredentialsById(ctx, clustercredentials_id)
			assert.Equal(t, rowsAffected, 1)
			assert.NoError(t, err)
		}
	}
}

package eventloop

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/go-logr/logr"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	db "github.com/redhat-appstudio/managed-gitops/backend-shared/db"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	logutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util/log"
	sharedresourceloop "github.com/redhat-appstudio/managed-gitops/backend/eventloop/shared_resource_loop"
)

const (
	repoCredRowBatchSize            = 100              // Number of rows needs to be fetched in each batch.
	repocredReconcilerInterval      = 10 * time.Minute // Interval in Minutes to reconcile Repository Credentials.
	repoCredSleepIntervalsOfBatches = 1 * time.Second  // Interval in Millisecond between each batch.
)

// RepoCredReconciler reconciles RepositoryCredential entries
type RepoCredReconciler struct {
	client.Client
	DB db.DatabaseQueries
}

// This function iterates through each entry of RepositoryCredential table in DB and updates the status of the CR.
func (r *RepoCredReconciler) StartRepoCredReconciler() {
	r.startTimerForNextCycle()
}

func (r *RepoCredReconciler) startTimerForNextCycle() {
	go func() {
		// Timer to trigger Reconciler
		timer := time.NewTimer(time.Duration(repocredReconcilerInterval))
		<-timer.C

		ctx := context.Background()
		log := log.FromContext(ctx).
			WithName(logutil.LogLogger_managed_gitops).
			WithValues("component", "repocred-reconciler")

		_, _ = sharedutil.CatchPanic(func() error {

			// Reconcile RepositoryCredentials here
			reconcileRepositoryCredentials(ctx, r.DB, r.Client, log)

			return nil
		})

		// Kick off the timer again, once the old task runs.
		// This ensures that at least 'repocredReconcilerInterval' time elapses from the end of one run to the beginning of another.
		r.startTimerForNextCycle()
	}()

}

// /////////////
// Reconcile logic for API CR To Database Mapping table and utility functions.
// This will reconcile repository credential entries from ACTDM table and RepoistoryCredential table
// /////////////
func reconcileRepositoryCredentials(ctx context.Context, dbQueries db.DatabaseQueries, client client.Client, l logr.Logger) {

	offSet := 0
	log := l.WithValues("job", "reconcileRepositoryCredentials")

	// Continuously iterate and fetch batches until all entries of ACTDM table are processed.
	for {
		if offSet != 0 {
			time.Sleep(repoCredSleepIntervalsOfBatches)
		}

		var listOfApiCrToDbMapping []db.APICRToDatabaseMapping

		// Fetch ACTDMs table entries in batch size as configured above.​
		if err := dbQueries.GetAPICRToDatabaseMappingBatch(ctx, &listOfApiCrToDbMapping, repoCredRowBatchSize, offSet); err != nil {
			log.Error(err, fmt.Sprintf("Error occurred in ACTDM Reconcile while fetching batch from Offset: %d to %d: ",
				offSet, offSet+repoCredRowBatchSize))
			break
		}

		// Break the loop if no entries are left in table to be processed.
		if len(listOfApiCrToDbMapping) == 0 {
			log.Info("All ACTDM entries are processed by ACTDM Reconciler.")
			break
		}

		// Iterate over batch received above.
		for i := range listOfApiCrToDbMapping {
			apiCrToDbMappingFromDB := listOfApiCrToDbMapping[i] // To avoid "Implicit memory aliasing in for loop." error.

			objectMeta := metav1.ObjectMeta{
				Name:      apiCrToDbMappingFromDB.APIResourceName,
				Namespace: apiCrToDbMappingFromDB.APIResourceNamespace,
			}
			if db.APICRToDatabaseMapping_ResourceType_GitOpsDeploymentRepositoryCredential == apiCrToDbMappingFromDB.APIResourceType {

				// Process if CR is of GitOpsDeploymentRepositoryCredential type.
				reconcileRepositoryCredentialStatus(ctx, client, dbQueries, apiCrToDbMappingFromDB, objectMeta, log)
			}

			log.Info("RepositoryCredential ACTDM Reconcile processed APICRToDatabaseMapping entry: " + apiCrToDbMappingFromDB.APIResourceUID)
		}

		// Skip processed entries in next iteration
		offSet += repoCredRowBatchSize
	}
}

func reconcileRepositoryCredentialStatus(ctx context.Context, apiNamespaceClient client.Client, dbQueries db.DatabaseQueries, apiCrToDbMappingFromDB db.APICRToDatabaseMapping, objectMeta metav1.ObjectMeta, l logr.Logger) {

	gitopsDeploymentRepositoryCredentialCR := managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredential{ObjectMeta: objectMeta}
	log := l.WithValues("job", "reconcileRepositoryCredentialStatus")

	// Check if required CR is present in cluster. If no, skip
	if err := apiNamespaceClient.Get(ctx, types.NamespacedName{Name: gitopsDeploymentRepositoryCredentialCR.GetName(), Namespace: gitopsDeploymentRepositoryCredentialCR.GetNamespace()}, &gitopsDeploymentRepositoryCredentialCR); err != nil {
		log.Info(fmt.Sprintf("could not find GitopsDeploymentRepositoryCredential %s in the cluster. Skipping reconciliation.", gitopsDeploymentRepositoryCredentialCR.GetName()))
		return
	}

	// Sanity test for gitopsDeploymentRepositoryCredentialCR.Spec.Secret to be non-empty value
	if gitopsDeploymentRepositoryCredentialCR.Spec.Secret == "" {
		if err := sharedresourceloop.UpdateGitopsDeploymentRepositoryCredentialStatus(ctx, &gitopsDeploymentRepositoryCredentialCR, apiNamespaceClient, nil, log); err != nil {
			log.Error(err, fmt.Sprintf("error updating status of GitopsDeploymentRepositoryCredential %v", gitopsDeploymentRepositoryCredentialCR))
		}
		return
	}

	secret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind: "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      gitopsDeploymentRepositoryCredentialCR.Spec.Secret,
			Namespace: apiCrToDbMappingFromDB.APIResourceNamespace, // we assume the secret is in the same namespace as the CR
		},
	}

	// Fetch the secret from the cluster
	if err := apiNamespaceClient.Get(ctx, client.ObjectKey{Name: secret.Name, Namespace: secret.Namespace}, secret); err != nil {
		log.Error(err, "Secret not found")
		if err := sharedresourceloop.UpdateGitopsDeploymentRepositoryCredentialStatus(ctx, &gitopsDeploymentRepositoryCredentialCR, apiNamespaceClient, nil, log); err != nil {
			log.Error(err, fmt.Sprintf("error updating status of GitopsDeploymentRepositoryCredential %v", gitopsDeploymentRepositoryCredentialCR))
		}
		return
	}

	// Update the status of GitopsDeploymentRepositoryCredential
	if err := sharedresourceloop.UpdateGitopsDeploymentRepositoryCredentialStatus(ctx, &gitopsDeploymentRepositoryCredentialCR, apiNamespaceClient, secret, log); err != nil {
		log.Error(err, fmt.Sprintf("error updating status of GitopsDeploymentRepositoryCredential %v", gitopsDeploymentRepositoryCredentialCR))
	}

}

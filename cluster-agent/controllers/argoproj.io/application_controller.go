/*
Copyright 2021, 2022

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package argoprojio

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	apierr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	appv1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"github.com/go-logr/logr"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
	cache "github.com/redhat-appstudio/managed-gitops/backend-shared/config/db/util"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	appEventLoop "github.com/redhat-appstudio/managed-gitops/backend/eventloop/application_event_loop"
	"github.com/redhat-appstudio/managed-gitops/backend/util/fauxargocd"
	"github.com/redhat-appstudio/managed-gitops/cluster-agent/controllers"
	"gopkg.in/yaml.v2"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// ApplicationReconciler reconciles a Application object
type ApplicationReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	TaskRetryLoop *sharedutil.TaskRetryLoop
	Cache         *cache.ApplicationInfoCache
	DB            db.DatabaseQueries
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *ApplicationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	log := log.FromContext(ctx).WithValues("name", req.Name, "namespace", req.Namespace)

	defer log.V(sharedutil.LogLevel_Debug).Info("Application Reconcile() complete.")

	// TODO: GITOPSRVCE-68 - PERF - this is single-threaded only

	// 1) Retrieve the Application CR using the request vals
	app := appv1.Application{}
	if err := r.Get(ctx, req.NamespacedName, &app); err != nil {
		if apierr.IsNotFound(err) {
			log.Info("Application deleted '" + req.NamespacedName.String() + "'")
			return ctrl.Result{}, nil
		} else {
			log.Error(err, "Unexpected error on retrieving Application '"+req.NamespacedName.String()+"'")
			return ctrl.Result{}, err
		}
	}

	// 2) Retrieve the Application DB entry, using the 'databaseID' field of the Application.
	// - This is a field we add ourselves to every Application we create, which references the
	//   corresponding an Application table primary key.
	applicationDB := &db.Application{}
	if databaseID, exists := app.Labels["databaseID"]; !exists {
		log.V(sharedutil.LogLevel_Warn).Info("Application CR was missing 'databaseID' label")
		return ctrl.Result{}, nil
	} else {
		applicationDB.Application_id = databaseID
	}
	if _, _, err := r.Cache.GetApplicationById(ctx, applicationDB.Application_id); err != nil {
		if db.IsResultNotFoundError(err) {

			log.V(sharedutil.LogLevel_Warn).Info("Application CR '" + req.NamespacedName.String() + "' missing corresponding database entry: " + applicationDB.Application_id)

			adt := applicationDeleteTask{
				applicationCR: app,
				client:        r.Client,
				log:           log,
			}

			// Add the Application to the task loop, so that it can be deleted.
			r.TaskRetryLoop.AddTaskIfNotPresent(app.Namespace+"/"+app.Name, &adt,
				sharedutil.ExponentialBackoff{Factor: 2, Min: time.Millisecond * 200, Max: time.Second * 10, Jitter: true})

			return ctrl.Result{}, nil
		} else {
			log.Error(err, "Unable to retrieve Application from database: "+applicationDB.Application_id)
			return ctrl.Result{}, err
		}
	}

	log = log.WithValues("applicationID", applicationDB.Application_id)

	// 3) Does there exist an ApplicationState for this Application, already?
	applicationState := &db.ApplicationState{
		Applicationstate_application_id: applicationDB.Application_id,
	}
	if _, _, errGet := r.Cache.GetApplicationStateById(ctx, applicationState.Applicationstate_application_id); errGet != nil {
		if db.IsResultNotFoundError(errGet) {

			// 3a) ApplicationState doesn't exist: so create it

			applicationState.Health = db.TruncateVarchar(string(app.Status.Health.Status), db.ApplicationStateHealthLength)
			applicationState.Message = db.TruncateVarchar(app.Status.Health.Message, db.ApplicationStateMessageLength)
			applicationState.Sync_Status = db.TruncateVarchar(string(app.Status.Sync.Status), db.ApplicationStateSyncStatusLength)
			applicationState.Revision = db.TruncateVarchar(app.Status.Sync.Revision, db.ApplicationStateRevisionLength)
			sanitizeHealthAndStatus(applicationState)

			// Get the list of resources created by deployment and convert it into a compressed YAML string.
			var err error
			applicationState.Resources, err = CompressResourceData(app.Status.Resources)
			if err != nil {
				log.Error(err, "unable to compress resource data into byte array.")
				return ctrl.Result{}, err
			}

			if errCreate := r.Cache.CreateApplicationState(ctx, *applicationState); errCreate != nil {
				log.Error(errCreate, "unexpected error on writing new application state")
				return ctrl.Result{}, errCreate
			}
			// Successfully created ApplicationState
			return ctrl.Result{}, nil
		} else {
			log.Error(errGet, "Unable to retrieve ApplicationState from database: "+applicationDB.Application_id)
			return ctrl.Result{}, errGet
		}
	}

	// 4) ApplicationState already exists, so just update it.

	applicationState.Health = db.TruncateVarchar(string(app.Status.Health.Status), db.ApplicationStateHealthLength)
	applicationState.Message = db.TruncateVarchar(app.Status.Health.Message, db.ApplicationStateMessageLength)
	applicationState.Sync_Status = db.TruncateVarchar(string(app.Status.Sync.Status), db.ApplicationStateSyncStatusLength)
	applicationState.Revision = db.TruncateVarchar(app.Status.Sync.Revision, db.ApplicationStateRevisionLength)
	sanitizeHealthAndStatus(applicationState)

	// Get the list of resources created by deployment and convert it into a compressed YAML string.
	var err error
	applicationState.Resources, err = CompressResourceData(app.Status.Resources)
	if err != nil {
		log.Error(err, "unable to compress resource data into byte array.")
		return ctrl.Result{}, err
	}

	if err := r.Cache.UpdateApplicationState(ctx, *applicationState); err != nil {
		log.Error(err, "unexpected error on updating existing application state")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil

}

var INITIAL_APP_ROW_OFFSET = 0
var APP_ROW_BATCH_SIZE = 50             // Number of rows needs to be fetched in each batch.
var NAME_SPACE_RECONCILER_INTERVAL = 30 //Interval in minutes to reconcile workspace/namespace.

// This function iterates through each Workspace/Namespace present in DB and ensures that the state of resources in Cluster is in Sync with DB.
// Set to True if timer needs to stop after one iteration, for example in case of unit testing.
func (r *ApplicationReconciler) NamespaceReconcile(isDryRun bool) {

	firstIteration := true
	// Timer to trigger reconciler
	ticker := time.NewTicker(time.Duration(NAME_SPACE_RECONCILER_INTERVAL) * time.Minute)

	// First Iteration starts immediately (Needed for test cases),
	// then future iterations will trigger after interval configured for workspace/namespace reconciliation.
	for ; true; <-ticker.C {
		ctx := context.Background()
		log := log.FromContext(ctx)
		batchSize := APP_ROW_BATCH_SIZE
		offSet := INITIAL_APP_ROW_OFFSET

		// If its first iteration then no need to call for cleanup as no operations are created yet.
		if !firstIteration {
			CleanK8sOperations(ctx, r.DB, r.Client, log)
		}

		log.Info("Triggered Namespace Reconciler to keep Argo application in sync with DB.")

		// Iterate until all entries of Application table are not processed.
		for {
			var listOfApplicationsFromDB []db.Application
			var applicationFromDB fauxargocd.FauxApplication

			// Fetch Application table entries in batches as configured above.​
			if err := r.DB.GetApplicationBatch(ctx, &listOfApplicationsFromDB, batchSize, offSet); err != nil {
				log.Error(err, "Error occurred in Namespace Reconciler while fetching batch from Offset: "+strconv.Itoa(offSet)+" to: "+strconv.Itoa(offSet+batchSize))
				break
			}

			// Break the loop if no entries are left in table to be processed.
			if len(listOfApplicationsFromDB) == 0 {
				log.Info("All Application entries are processed by Namespace Reconciler.")
				break
			}

			// Iterate over the received batch.
			for _, applicationRowFromDB := range listOfApplicationsFromDB {

				if err := yaml.Unmarshal([]byte(applicationRowFromDB.Spec_field), &applicationFromDB); err != nil {
					log.Error(err, "Error occurred in Namespace Reconciler while unmarshalling application: "+applicationRowFromDB.Application_id)
					continue
				}

				// Fetch the Application object from k8s
				applicationFromArgoCD := appv1.Application{}
				namespacedName := types.NamespacedName{
					Name:      applicationFromDB.Name,
					Namespace: applicationFromDB.Namespace}

				err := r.Get(ctx, namespacedName, &applicationFromArgoCD)
				if err != nil {
					if strings.Contains(err.Error(), "not found") {
						log.Info("Application " + applicationRowFromDB.Application_id + " not found in ArgoCD, probably user deleted it, " +
							"but It still exists in DB, hence recreating application in ArgoCD.")

						// Assign values to ArgoCD application object from DB object
						applicationFromArgoCD.Kind, applicationFromArgoCD.APIVersion,
							applicationFromArgoCD.Name, applicationFromArgoCD.Namespace,
							applicationFromArgoCD.Spec.Source.RepoURL, applicationFromArgoCD.Spec.Source.Path,
							applicationFromArgoCD.Spec.Source.TargetRevision = applicationFromDB.Kind,
							applicationFromDB.APIVersion, applicationFromDB.Name,
							applicationFromDB.Namespace, applicationFromDB.Spec.Source.RepoURL,
							applicationFromDB.Spec.Source.Path, applicationFromDB.Spec.Source.TargetRevision

						if applicationFromDB.Spec.SyncPolicy != nil {
							applicationFromArgoCD.Spec.SyncPolicy = &appv1.SyncPolicy{
								Automated: &appv1.SyncPolicyAutomated{Prune: applicationFromDB.Spec.SyncPolicy.Automated.Prune},
							}
						}

						// Recreate ArgoCD Application
						if err := r.Create(ctx, &applicationFromArgoCD); err != nil {
							log.Error(err, "Namespace Reconciler failed to create application in ArgoCD.")
							continue
						}
					} else {
						log.Error(err, "Error occurred in Namespace Reconciler while fetching application from cluster: "+applicationRowFromDB.Application_id)
						continue
					}
				}

				// Compare ArgoCD application and Application entry from DB.
				if compareApplications(applicationFromArgoCD, applicationFromDB, log) {
					// No need to do anything, if both objects are same.
					log.Info("Argo application is in Sync with DB, Application:" + applicationRowFromDB.Application_id)
					continue
				} else {
					log.Info("Argo application is not in Sync with DB, Updating ArgoCD App. Application:" + applicationRowFromDB.Application_id)
				}

				// Get Special user created for internal use,
				// because we need ClusterUser for creating Operation and we don't have one.
				// Hence created a dummy Cluster User for internal purpose.
				var specialClusterUser db.ClusterUser
				if err := r.DB.GetOrCreateSpecialClusterUser(ctx, &specialClusterUser); err != nil {
					log.Error(err, "Error occurred in Namespace Reconciler while fetching clusterUser: "+applicationRowFromDB.Application_id)
					continue
				}

				// ArgoCD application and DB entry are not in Sync,
				// ArgoCD should use the state of resources present in the database should
				// Create Operation to inform ArgoCD to get in Sync with database entry.
				dbOperationInput := db.Operation{
					Instance_id:   applicationRowFromDB.Engine_instance_inst_id,
					Resource_id:   applicationRowFromDB.Application_id,
					Resource_type: db.OperationResourceType_Application,
				}

				_, _, err = appEventLoop.CreateOperation(ctx, false, dbOperationInput,
					specialClusterUser.Clusteruser_id, cache.GetGitOpsEngineSingleInstanceNamespace(), r.DB, r.Client, log)
				if err != nil {
					log.Error(err, "Namespace Reconciler is unable to create operation: "+dbOperationInput.ShortString())
					continue
				}

				log.Info("Namespace Reconcile processed application : " + applicationRowFromDB.Application_id)
			}

			// Skip processed entries in next iteration
			offSet += batchSize
		}

		firstIteration = false

		log.Info("NameSpace Reconciler finished an iteration at " + time.Now().String() +
			". Next iteration will be triggered after " + strconv.Itoa(NAME_SPACE_RECONCILER_INTERVAL) + " Minutes")

		// Stop timer and break reconciler loop.
		if isDryRun {
			ticker.Stop()
			break
		}
	}
}

func sanitizeHealthAndStatus(applicationState *db.ApplicationState) {

	if applicationState.Health == "" {
		applicationState.Health = "Unknown"
	}

	if applicationState.Sync_Status == "" {
		applicationState.Sync_Status = "Unknown"
	}

}

type applicationDeleteTask struct {
	applicationCR appv1.Application
	client        client.Client
	log           logr.Logger
}

func (adt *applicationDeleteTask) PerformTask(taskContext context.Context) (bool, error) {

	err := controllers.DeleteArgoCDApplication(context.Background(), adt.applicationCR, adt.client, adt.log)

	if err != nil {
		adt.log.Error(err, "Unable to delete Argo CD Application: "+adt.applicationCR.Name+"/"+adt.applicationCR.Namespace)
	}

	// Don't retry on fail, just return the error
	return false, err
}

// SetupWithManager sets up the controller with the Manager.
func (r *ApplicationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&appv1.Application{}).
		Complete(r)
}

// Convert ResourceStatus Array into String and then compress it into Byte Array​
func CompressResourceData(resources []appv1.ResourceStatus) ([]byte, error) {
	var byteArr []byte
	var buffer bytes.Buffer

	// Convert ResourceStatus object into String.
	resourceStr, err := yaml.Marshal(&resources)
	if err != nil {
		return byteArr, fmt.Errorf("Unable to Marshal resource data. %v", err)
	}

	// Compress string data
	gzipWriter, err := gzip.NewWriterLevel(&buffer, gzip.BestSpeed)
	if err != nil {
		return byteArr, fmt.Errorf("Unable to create Buffer writer. %v", err)
	}

	_, err = gzipWriter.Write([]byte(string(resourceStr)))

	if err != nil {
		return byteArr, fmt.Errorf("Unable to compress resource string. %v", err)
	}

	if err := gzipWriter.Close(); err != nil {
		return byteArr, fmt.Errorf("Unable to close gzip writer connection. %v", err)
	}

	return buffer.Bytes(), nil
}

// Compare Application objects, since both objects are of different types we can not use == operator for comparison.
func compareApplications(applicationFromArgoCD appv1.Application, applicationFromDB fauxargocd.FauxApplication, log logr.Logger) bool {

	if applicationFromArgoCD.APIVersion != applicationFromDB.APIVersion {
		log.Info("APIVersion field in ArgoCD and DB entry is not in Sync.")
		log.Info("APIVersion:= ArgoCD: " + applicationFromArgoCD.APIVersion + "; DB: " + applicationFromDB.APIVersion)
		return false
	}
	if applicationFromArgoCD.Kind != applicationFromDB.Kind {
		log.Info("Kind field in ArgoCD and DB entry is not in Sync.")
		log.Info("Kind:= ArgoCD: " + applicationFromArgoCD.Kind + "; DB: " + applicationFromDB.Kind)
		return false
	}
	if applicationFromArgoCD.Name != applicationFromDB.Name {
		log.Info("Name field in ArgoCD and DB entry is not in Sync.")
		log.Info("Name:= ArgoCD: " + applicationFromArgoCD.Name + "; DB: " + applicationFromDB.Name)
		return false
	}
	if applicationFromArgoCD.Namespace != applicationFromDB.Namespace {
		log.Info("Namespace field in ArgoCD and DB entry is not in Sync.")
		log.Info("Namespace:= ArgoCD: " + applicationFromArgoCD.Namespace + "; DB: " + applicationFromDB.Namespace)
		return false
	}
	if applicationFromArgoCD.Spec.Source.RepoURL != applicationFromDB.Spec.Source.RepoURL {
		log.Info("RepoURL field in ArgoCD and DB entry is not in Sync.")
		log.Info("RepoURL:= ArgoCD: " + applicationFromArgoCD.Spec.Source.RepoURL + "; DB: " + applicationFromDB.Spec.Source.RepoURL)
		return false
	}
	if applicationFromArgoCD.Spec.Source.Path != applicationFromDB.Spec.Source.Path {
		log.Info("Path field in ArgoCD and DB entry is not in Sync.")
		log.Info("Path:= ArgoCD: " + applicationFromArgoCD.Spec.Source.Path + "; DB: " + applicationFromDB.Spec.Source.Path)
		return false
	}
	if applicationFromArgoCD.Spec.Source.TargetRevision != applicationFromDB.Spec.Source.TargetRevision {
		log.Info("TargetRevision field in ArgoCD and DB entry is not in Sync.")
		log.Info("TargetRevision:= ArgoCD: " + applicationFromArgoCD.Spec.Source.TargetRevision + "; DB: " + applicationFromDB.Spec.Source.TargetRevision)
		return false
	}
	if applicationFromArgoCD.Spec.Destination.Server != applicationFromDB.Spec.Destination.Server {
		log.Info("Destination.Server field in ArgoCD and DB entry is not in Sync.")
		log.Info("Destination.Server:= ArgoCD: " + applicationFromArgoCD.Spec.Destination.Server + "; DB: " + applicationFromDB.Spec.Destination.Server)
		return false
	}
	if applicationFromArgoCD.Spec.Destination.Namespace != applicationFromDB.Spec.Destination.Namespace {
		log.Info("Destination.Namespace field in ArgoCD and DB entry is not in Sync.")
		log.Info("Destination.Namespace:= ArgoCD: " + applicationFromArgoCD.Spec.Destination.Namespace + "; DB: " + applicationFromDB.Spec.Destination.Namespace)
		return false
	}
	if applicationFromArgoCD.Spec.Destination.Name != applicationFromDB.Spec.Destination.Name {
		log.Info("Destination.Name field in ArgoCD and DB entry is not in Sync.")
		log.Info("Destination.Name:= ArgoCD: " + applicationFromArgoCD.Spec.Destination.Name + "; DB: " + applicationFromDB.Spec.Destination.Name)
		return false
	}
	if applicationFromArgoCD.Spec.Project != applicationFromDB.Spec.Project {
		log.Info("Project field in ArgoCD and DB entry is not in Sync.")
		log.Info("Project:= ArgoCD: " + applicationFromArgoCD.Spec.Project + "; DB: " + applicationFromDB.Spec.Project)
		return false
	}
	if applicationFromArgoCD.Spec.SyncPolicy.Automated.Prune != applicationFromDB.Spec.SyncPolicy.Automated.Prune {
		log.Info("Prune field in ArgoCD and DB entry is not in Sync.")
		return false
	}
	if applicationFromArgoCD.Spec.SyncPolicy.Automated.SelfHeal != applicationFromDB.Spec.SyncPolicy.Automated.SelfHeal {
		log.Info("SelfHeal field in ArgoCD and DB entry is not in Sync.")
		return false
	}
	if applicationFromArgoCD.Spec.SyncPolicy.Automated.AllowEmpty != applicationFromDB.Spec.SyncPolicy.Automated.AllowEmpty {
		log.Info("AllowEmpty field in ArgoCD and DB entry is not in Sync.")
		return false
	}
	return true
}

func CleanK8sOperations(ctx context.Context, dbq db.DatabaseQueries, client client.Client, log logr.Logger) {
	listOfK8sOperation := v1alpha1.OperationList{}
	err := client.List(ctx, &listOfK8sOperation)
	if err != nil {
		log.Error(err, "Unable to fetch list of k8s Operation from cluster.")
		return
	}

	for _, k8sOperation := range listOfK8sOperation.Items {

		// Skip if Operation was not created by NameSpace Reconciler.
		if k8sOperation.Annotations["source"] != "periodic-cleanup" {
			continue
		}

		// Fetch corresponding DB entry
		dbOperation := db.Operation{
			Operation_id: k8sOperation.Spec.OperationID,
		}
		if err := dbq.GetOperationById(ctx, &dbOperation); err != nil {
			log.Error(err, "Unable to fetch DB Operation entry for "+string(k8sOperation.Spec.OperationID))
			continue
		}

		if dbOperation.State != "Completed" {
			log.Info("K8s Operation is not ready for cleanup : " + string(k8sOperation.UID) + " DbOperation: " + string(k8sOperation.Spec.OperationID))
			continue
		}

		log.Info("Deleting Operation created by NameSpace Reconciler." + string(k8sOperation.UID))

		// Clean the k8s operation now.
		if err := appEventLoop.CleanupOperation(ctx, dbOperation, k8sOperation, cache.GetGitOpsEngineSingleInstanceNamespace(), dbq, client, log); err != nil {
			log.Error(err, "Unable to Delete k8s Operation"+string(k8sOperation.UID)+" for DbOperation: "+string(k8sOperation.Spec.OperationID))
		} else {
			log.Info("Deleted k8s Operation: " + string(k8sOperation.UID) + " for DbOperation: " + string(k8sOperation.Spec.OperationID))
		}
	}
	log.Info("Cleaned all Operations created by NameSpace Reconciler.")
}

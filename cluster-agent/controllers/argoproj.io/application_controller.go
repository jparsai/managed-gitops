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
	"time"

	apierr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	appv1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"github.com/go-logr/logr"
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
var APP_ROW_BATCH_SIZE = 50            // Number of rows needs to be fetched in each batch.
var NAME_SPACE_RECONCILER_INTERVAL = 5 //Interval in minutes to reconcile workspace/namespace.

func (r *ApplicationReconciler) NamespaceReconcile() {

	// Timer to trigger reconciler
	ticker := time.NewTicker(time.Duration(NAME_SPACE_RECONCILER_INTERVAL) * time.Minute)

	// Iterate after interval configured for workspace/namespace reconciliation.
	for range ticker.C {
		batchSize := APP_ROW_BATCH_SIZE
		offSet := INITIAL_APP_ROW_OFFSET
		ctx := context.Background()
		log := log.FromContext(ctx)

		log.Info("Triggered namespace reconcile to keep Argo application in sync with db.")

		// Iterate until all entries of Application table are not processed.
		for {
			var listOfApplicationsFromDB []db.Application
			var applicationFromDB fauxargocd.FauxApplication
			//var applicationFromDBSyncPolicy fauxargocd.SyncPolicy
			//applicationFromDB.Spec.SyncPolicy = &applicationFromDBSyncPolicy

			// Fetch Application table entries in batches as configured above.​
			err := r.DB.GetApplicationBatch(ctx, &listOfApplicationsFromDB, batchSize, offSet)
			if err != nil {
				log.Error(err, "Error occurred while fetching batch from Offset: ", offSet, "to: ", offSet+batchSize)
				break
			}

			// Break the loop if no entries are left in table to be processed.
			if len(listOfApplicationsFromDB) == 0 {
				log.Info("All entries are processed.")
				break
			}

			// Iterate over the received batch.
			for _, applicationsFromDB := range listOfApplicationsFromDB {

				// Convert String to Object
				err = yaml.Unmarshal([]byte(applicationsFromDB.Spec_field), &applicationFromDB)
				if err != nil {
					log.Error(err, "Error occurred while unmarshalling application: ", applicationsFromDB.Application_id)
					continue
				}

				// Fetch the Application object from k8s
				applicationFromArgoCD := appv1.Application{}
				namespacedName := types.NamespacedName{
					Name:      applicationFromDB.Name,
					Namespace: applicationFromDB.Namespace}

				err = r.Get(ctx, namespacedName, &applicationFromArgoCD)
				if err != nil {
					log.Error(err, "Error occurred while fetching application: ", applicationsFromDB.Application_id)
					continue
				}

				// Compare ArgoCD application and Application entry from DB.
				if compareApplications(applicationFromArgoCD, applicationFromDB) {
					// No need to do anything, if both objects are same.
					log.Info("Argo application: ", applicationsFromDB.Application_id, "is in Sync with DB.")
					continue
				}

				// Get Special user created for internal use,
				// because we need ClusterUser for creating Operation and we don't have one.
				// Hence created a dummy Cluster User for internal purpose.
				var specialClusterUser db.ClusterUser
				err = r.DB.GetOrCreateSpecialClusterUser(ctx, &specialClusterUser)
				if err != nil {
					log.Error(err, "Error occurred while fetching clusterUser: ", applicationsFromDB.Application_id)
					continue
				}

				// ArgoCD application and DB entry are not in Sync,
				// ArgoCD should use the state of resources present in the database should
				// Create Operation to inform ArgoCD to get in Sync with database entry.
				dbOperationInput := db.Operation{
					Instance_id:   applicationsFromDB.Engine_instance_inst_id,
					Resource_id:   applicationsFromDB.Application_id,
					Resource_type: db.OperationResourceType_Application,
				}

				k8sOperation, dbOperation, err := appEventLoop.CreateOperation(ctx, true, dbOperationInput,
					specialClusterUser.Clusteruser_id, cache.GetGitOpsEngineSingleInstanceNamespace(), r.DB, r.Client, log)
				if err != nil {
					log.Error(err, "unable to create operation", "operation", dbOperationInput.ShortString())
					continue
				}

				err = appEventLoop.CleanupOperation(ctx, *dbOperation, *k8sOperation, cache.GetGitOpsEngineSingleInstanceNamespace(), r.DB, r.Client, log)
				if err != nil {
					log.Error(err, "unable to cleanup operation", "operation", dbOperationInput.ShortString())
				}

			}

			// Skip processed entries in next iteration
			offSet += batchSize
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
func compareApplications(applicationFromArgoCD appv1.Application, applicationFromDB fauxargocd.FauxApplication) bool {

	if applicationFromArgoCD.APIVersion != applicationFromDB.APIVersion {
		return false
	}
	if applicationFromArgoCD.Kind != applicationFromDB.Kind {
		return false
	}
	if applicationFromArgoCD.Name != applicationFromDB.Name {
		return false
	}
	if applicationFromArgoCD.Namespace != applicationFromDB.Namespace {
		return false
	}
	if applicationFromArgoCD.Spec.Source.RepoURL != applicationFromDB.Spec.Source.RepoURL {
		return false
	}
	if applicationFromArgoCD.Spec.Source.Path != applicationFromDB.Spec.Source.Path {
		return false
	}
	if applicationFromArgoCD.Spec.Source.TargetRevision != applicationFromDB.Spec.Source.TargetRevision {
		return false
	}
	if applicationFromArgoCD.Spec.Destination.Server != applicationFromDB.Spec.Destination.Server {
		return false
	}
	if applicationFromArgoCD.Spec.Destination.Namespace != applicationFromDB.Spec.Destination.Namespace {
		return false
	}
	if applicationFromArgoCD.Spec.Destination.Name != applicationFromDB.Spec.Destination.Name {
		return false
	}
	if applicationFromArgoCD.Spec.Project != applicationFromDB.Spec.Project {
		return false
	}
	if applicationFromArgoCD.Spec.SyncPolicy.Automated.Prune != applicationFromDB.Spec.SyncPolicy.Automated.Prune {
		return false
	}
	if applicationFromArgoCD.Spec.SyncPolicy.Automated.SelfHeal != applicationFromDB.Spec.SyncPolicy.Automated.SelfHeal {
		return false
	}
	if applicationFromArgoCD.Spec.SyncPolicy.Automated.AllowEmpty != applicationFromDB.Spec.SyncPolicy.Automated.AllowEmpty {
		return false
	}
	return true
}

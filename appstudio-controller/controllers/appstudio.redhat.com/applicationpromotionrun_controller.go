/*
Copyright 2022.

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

package appstudioredhatcom

import (
	"context"
	"fmt"
	"strings"
	"time"

	appstudioshared "github.com/redhat-appstudio/managed-gitops/appstudio-shared/apis/appstudio.redhat.com/v1alpha1"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	apibackend "github.com/redhat-appstudio/managed-gitops/backend/apis/managed-gitops/v1alpha1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// ApplicationPromotionRunReconciler reconciles a ApplicationPromotionRun object
type ApplicationPromotionRunReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=appstudio.redhat.com,resources=applicationpromotionruns,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=appstudio.redhat.com,resources=applicationpromotionruns/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=appstudio.redhat.com,resources=applicationpromotionruns/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.0/pkg/reconcile

func (r *ApplicationPromotionRunReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx).WithValues("name", req.Name, "namespace", req.Namespace)
	defer log.V(sharedutil.LogLevel_Debug).Info("Application Promotion Run Reconcile() complete.")

	promotionRun := &appstudioshared.ApplicationPromotionRun{}

	if err := r.Client.Get(ctx, req.NamespacedName, promotionRun); err != nil {
		if apierr.IsNotFound(err) {
			// Nothing more to do!
			return ctrl.Result{}, nil
		} else {
			log.Error(err, "unable to retrieve ApplicationPromotionRun.")
			return ctrl.Result{}, fmt.Errorf("unable to retrieve ApplicationPromotionRun: %v", err)
		}
	}

	if promotionRun.Status.State == appstudioshared.PromotionRunState_Complete {
		// Ignore promotion runs that have completed.
		return ctrl.Result{}, nil
	}

	if err := checkForExistingActivePromotions(ctx, *promotionRun, r.Client); err != nil {
		log.Error(err, "Error occureed while checking for existing active promotions.")

		if promotionRun, err = updateStatusConditions(ctx, r.Client, "Error occureed while checking for existing active promotions.", promotionRun, appstudioshared.PromotionRunConditionErrorOccurred,
			appstudioshared.PromotionRunConditionStatusFalse, appstudioshared.PromotionRunReasonErrorOccurred); err != nil {
			return ctrl.Result{}, fmt.Errorf("unable to update promotionRun %v", err)
		}

		return ctrl.Result{}, nil
	}

	// If this is a automated promotion, ignore it for now
	if promotionRun.Spec.AutomatedPromotion.InitialEnvironment != "" {
		log.Error(fmt.Errorf("Automated promotion are not yet supported."), promotionRun.Name)
		var err error
		if promotionRun, err = updateStatusConditions(ctx, r.Client, "Automated promotion are not yet supported.", promotionRun, appstudioshared.PromotionRunConditionErrorOccurred,
			appstudioshared.PromotionRunConditionStatusFalse, appstudioshared.PromotionRunReasonErrorOccurred); err != nil {
			return ctrl.Result{}, fmt.Errorf("unable to update promotionRun %v", err)
		}
		return ctrl.Result{}, nil
	}

	if promotionRun.Spec.ManualPromotion.TargetEnvironment == "" {
		log.Error(fmt.Errorf("Target environment has invalid value."), promotionRun.Name)

		var err error
		if promotionRun, err = updateStatusConditions(ctx, r.Client, "Target environment has invalid value.", promotionRun, appstudioshared.PromotionRunConditionErrorOccurred,
			appstudioshared.PromotionRunConditionStatusFalse, appstudioshared.PromotionRunReasonErrorOccurred); err != nil {
			return ctrl.Result{}, fmt.Errorf("unable to update promotionRun %v", err)
		}

		return ctrl.Result{}, nil
	}

	// 1) Locate the binding that this PromotionRun is targetting
	binding, err := locateTargetManualBinding(ctx, *promotionRun, r.Client)
	if err != nil {
		return ctrl.Result{}, nil
	}

	if promotionRun.Status.State != appstudioshared.PromotionRunState_Active {
		promotionRun.Status.State = appstudioshared.PromotionRunState_Active
		if err := r.Client.Status().Update(ctx, promotionRun); err != nil {
			log.Error(err, "unable to update PromotionRun state")
			return ctrl.Result{}, fmt.Errorf("unable to update PromotionRun state: %v", err)
		}
		log.V(sharedutil.LogLevel_Debug).Info("updated PromotionRun state" + promotionRun.Name)
	}

	// Verify: activebindings should not have a value which differs from the value specified in promotionrun.spec
	if len(promotionRun.Status.ActiveBindings) > 0 {
		for _, existingActiveBinding := range promotionRun.Status.ActiveBindings {
			if existingActiveBinding != binding.Name {
				message := "The binding changed after the PromotionRun first start. " +
					"The .spec fields of the PromotionRun are immutable, and should not be changed " +
					"after being created. old-binding: " + existingActiveBinding + ", new-binding: " + binding.Name

				if promotionRun, err = updateStatusConditions(ctx, r.Client, message, promotionRun, appstudioshared.PromotionRunConditionErrorOccurred,
					appstudioshared.PromotionRunConditionStatusFalse, appstudioshared.PromotionRunReasonErrorOccurred); err != nil {
					return ctrl.Result{}, fmt.Errorf("unable to update promotionRun %v", err)
				}
				return ctrl.Result{}, nil
			}
		}
	}

	// Verify that the snapshot refered in binding.spec.snapshot actually exists, if not, throw error

	applicationSnapshot := appstudioshared.ApplicationSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      binding.Spec.Snapshot,
			Namespace: binding.Namespace,
		},
	}

	if err := r.Client.Get(ctx, client.ObjectKeyFromObject(&applicationSnapshot), &applicationSnapshot); err != nil {
		if apierr.IsNotFound(err) {
			log.Error(err, "Snapshot refered in Binding does not exist.")
			return ctrl.Result{}, fmt.Errorf("Snapshot refered in Binding does not exist.")
		} else {
			log.Error(err, "unable to retrieve ApplicationSnapshot.")
			return ctrl.Result{}, fmt.Errorf("unable to retrieve ApplicationSnapshot: %v", err)
		}
	}

	// 2) Set the Binding to target the expected snapshot, if not already done
	if binding.Spec.Snapshot != promotionRun.Spec.Snapshot || len(promotionRun.Status.ActiveBindings) == 0 {
		binding.Spec.Snapshot = promotionRun.Spec.Snapshot
		if err := r.Client.Update(ctx, &binding); err != nil {
			return ctrl.Result{}, fmt.Errorf("unable to update Binding '%s' snapshot: %v", binding.Name, err)
		}
		log.Info("Updating Binding: " + binding.Name + " to target the Snapshot: " + promotionRun.Spec.Snapshot)

		// Set the time when of first reconcilation on a perticular PromotionRun if not set already.
		if promotionRun.Status.PromotionStartTime.IsZero() {
			promotionRun.Status.PromotionStartTime = metav1.Now()
		}

		promotionRun.Status.ActiveBindings = []string{binding.Name}
		if err := r.Client.Status().Update(ctx, promotionRun); err != nil {
			return ctrl.Result{}, fmt.Errorf("unable to update PromotionRun active binding: %v", err)
		}
		return ctrl.Result{}, nil
	}

	// 3) Wait for the environment binding to create all of the expected GitOpsDeployments
	if len(binding.Status.GitOpsDeployments) != len(binding.Spec.Components) {
		promotionRun.Status.State = appstudioshared.PromotionRunState_Waiting

		if promotionRun, err = updateStatusEnvironmentStatus(ctx, r.Client, "Waiting for the environment binding to create all of the expected GitOpsDeployments.",
			promotionRun, appstudioshared.ApplicationPromotionRunEnvironmentStatus_InProgress); err != nil {
			return ctrl.Result{}, fmt.Errorf("unable to update promotionRun %v", err)
		}
		return ctrl.Result{}, nil
	}

	gitopsRepositoryCommitsWithSnapshot := []string{} /* we need a mechanism to tell which gitops repository revision corresponds to which snapshot*/

	// 4) Wait for all the GitOpsDeployments of the binding to have the expected state
	waitingGitOpsDeployments := []string{}

	for _, gitopsDeploymentName := range binding.Status.GitOpsDeployments {

		gitopsDeployment := &apibackend.GitOpsDeployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      gitopsDeploymentName.GitOpsDeployment,
				Namespace: binding.Namespace,
			},
		}
		if err := r.Client.Get(ctx, client.ObjectKeyFromObject(gitopsDeployment), gitopsDeployment); err != nil {
			return ctrl.Result{}, fmt.Errorf("unable to retrieve gitopsdeployment '%s', %v", gitopsDeployment.Name, err)
		}

		// Must have status of Synced/Healthy
		if gitopsDeployment.Status.Sync.Status == apibackend.SyncStatusCodeSynced && gitopsDeployment.Status.Health.Status != apibackend.HeathStatusCodeHealthy {
			promotionRun.Status.State = appstudioshared.PromotionRunState_Waiting

			if promotionRun, err = updateStatusEnvironmentStatus(ctx, r.Client, "waiting for GitOpsDeployments to get in Sync/Healthy.",
				promotionRun, appstudioshared.ApplicationPromotionRunEnvironmentStatus_InProgress); err != nil {
				return ctrl.Result{}, fmt.Errorf("unable to update promotionRun %v", err)
			}

			waitingGitOpsDeployments = append(waitingGitOpsDeployments, gitopsDeployment.Name)
			continue
		}

		// Argo CD must have deployed at least one of the commits that include the Snapshot container images
		match := false
		for _, snapshotCommit := range gitopsRepositoryCommitsWithSnapshot {
			if gitopsDeployment.Status.Sync.Revision == snapshotCommit {
				match = true
				break
			}
		}
		if !match {
			waitingGitOpsDeployments = append(waitingGitOpsDeployments, gitopsDeployment.Name)
			continue
		}
	}

	// Check time limit set for PromotionRun reconcilation, fail if the conditions aren't met in the given timeframe.
	if err := r.Client.Get(ctx, client.ObjectKeyFromObject(promotionRun), promotionRun); err != nil {
		return ctrl.Result{}, fmt.Errorf("unable to retrieve promotionRun '%s', %v", promotionRun.Name, err)
	}

	if !promotionRun.Status.PromotionStartTime.IsZero() && (metav1.Now().Sub(promotionRun.Status.PromotionStartTime.Time).Minutes() > 10) {
		promotionRun.Status.CompletionResult = appstudioshared.PromotionRunCompleteResult_Failure
		promotionRun.Status.State = appstudioshared.PromotionRunState_Complete

		if promotionRun, err = updateStatusEnvironmentStatus(ctx, r.Client, "Promotion Failed. Could not be completed in 10 Minutes.",
			promotionRun, appstudioshared.ApplicationPromotionRunEnvironmentStatus_Failed); err != nil {
			return ctrl.Result{}, fmt.Errorf("unable to update promotionRun %v", err)
		}
	}

	if len(waitingGitOpsDeployments) > 0 {
		fmt.Println("Waiting for GitOpsDeployments to have expected commit/sync/health:", waitingGitOpsDeployments)
		promotionRun.Status.State = appstudioshared.PromotionRunState_Waiting

		if promotionRun, err = updateStatusEnvironmentStatus(ctx, r.Client, "Waiting for following GitOpsDeployments to be Synced/Healthy "+strings.Join(waitingGitOpsDeployments[:], ", "),
			promotionRun, appstudioshared.ApplicationPromotionRunEnvironmentStatus_InProgress); err != nil {
			return ctrl.Result{}, fmt.Errorf("unable to update promotionRun %v", err)
		}
		return ctrl.Result{RequeueAfter: time.Second * 15}, nil
	}

	// All the GitOpsDeployments are synced/healthy, and they are synced with a commit that includes the target snapshot.
	promotionRun.Status.ActiveBindings = []string{}
	promotionRun.Status.CompletionResult = appstudioshared.PromotionRunCompleteResult_Success
	promotionRun.Status.State = appstudioshared.PromotionRunState_Complete
	promotionRun.Status.ActiveBindings = []string{binding.Name}

	if promotionRun, err = updateStatusEnvironmentStatus(ctx, r.Client, "All GitOpsDeployments are Synced/Healthy",
		promotionRun, appstudioshared.ApplicationPromotionRunEnvironmentStatus_Success); err != nil {
		return ctrl.Result{}, fmt.Errorf("unable to update promotionRun %v", err)
	}

	return ctrl.Result{}, nil
}

// checkForExistingActivePromotions ensures that there are no other promotions that are active on this application.
// - only one promotion should be active at a time on a single Application
func checkForExistingActivePromotions(ctx context.Context, reconciledPromotionRun appstudioshared.ApplicationPromotionRun, k8sClient client.Client) error {

	otherPromotionRuns := &appstudioshared.ApplicationPromotionRunList{}
	if err := k8sClient.List(ctx, otherPromotionRuns); err != nil {
		// log me
		return err
	}

	for _, otherPromotionRun := range otherPromotionRuns.Items {
		if otherPromotionRun.Status.State == appstudioshared.PromotionRunState_Complete {
			// Ignore completed promotions (these are no longer active)
			continue
		}

		if otherPromotionRun.Spec.Application != reconciledPromotionRun.Spec.Application {
			// Ignore promotions with applications that don't match the one we are reconciling
			continue
		}

		if otherPromotionRun.ObjectMeta.CreationTimestamp.Before(&reconciledPromotionRun.CreationTimestamp) {
			return fmt.Errorf("another PromotionRun is already active. Only one waiting/active PromotionRun should exist per Application: %s", otherPromotionRun.Name)
		}

	}

	return nil
}

func locateTargetManualBinding(ctx context.Context, promotionRun appstudioshared.ApplicationPromotionRun, k8sClient client.Client) (appstudioshared.ApplicationSnapshotEnvironmentBinding, error) {

	// Locate the corresponding binding

	bindingList := appstudioshared.ApplicationSnapshotEnvironmentBindingList{}
	if err := k8sClient.List(ctx, &bindingList); err != nil {
		return appstudioshared.ApplicationSnapshotEnvironmentBinding{}, fmt.Errorf("unable to list bindings: %v", err)
	}

	for _, binding := range bindingList.Items {

		if binding.Spec.Application == promotionRun.Spec.Application && binding.Spec.Environment == promotionRun.Spec.ManualPromotion.TargetEnvironment {
			return binding, nil
		}
	}

	return appstudioshared.ApplicationSnapshotEnvironmentBinding{},
		fmt.Errorf("unable to locate binding with application '%s' and target environment '%s'",
			promotionRun.Spec.Application, promotionRun.Spec.ManualPromotion.TargetEnvironment)

}

// SetupWithManager sets up the controller with the Manager.
func (r *ApplicationPromotionRunReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&appstudioshared.ApplicationPromotionRun{}).
		Owns(&appstudioshared.ApplicationSnapshotEnvironmentBinding{}).
		Complete(r)
}

func updateStatusEnvironmentStatus(ctx context.Context, client client.Client, displayStatus string, promotionRun *appstudioshared.ApplicationPromotionRun,
	status appstudioshared.PromotionRunEnvironmentStatusField) (*appstudioshared.ApplicationPromotionRun, error) {

	targetEnvIndex, targetEnvStep, isEnvStatusExsists := 0, 0, false

	// Check if EnvironmentStatus for given Environment is already present.
	for i, envStatus := range promotionRun.Status.EnvironmentStatus {
		// Find the Index in array having status for given environment.
		if envStatus.EnvironmentName == promotionRun.Spec.ManualPromotion.TargetEnvironment {
			targetEnvIndex = i
			isEnvStatusExsists = true
			break
		}
	}

	// If given environment is not present already then create new else update existing one.
	if !isEnvStatusExsists {
		// Find the max Step and Index available
		for _, j := range promotionRun.Status.EnvironmentStatus {
			if j.Step > targetEnvIndex {
				targetEnvStep = j.Step
			}
		}

		promotionRun.Status.EnvironmentStatus = append(promotionRun.Status.EnvironmentStatus,
			appstudioshared.PromotionRunEnvironmentStatus{
				Step:            targetEnvStep + 1,
				EnvironmentName: promotionRun.Spec.ManualPromotion.TargetEnvironment,
				DisplayStatus:   displayStatus,
				Status:          status,
			})
	} else {
		// Status for given environment exists, just update it.
		promotionRun.Status.EnvironmentStatus[targetEnvIndex].DisplayStatus = displayStatus
		promotionRun.Status.EnvironmentStatus[targetEnvIndex].Status = status
	}

	if err := client.Status().Update(ctx, promotionRun); err != nil {
		return promotionRun, err
	}
	return promotionRun, nil
}

func updateStatusConditions(ctx context.Context, client client.Client, message string,
	promotionRun *appstudioshared.ApplicationPromotionRun, conditionType appstudioshared.PromotionRunConditionType,
	status appstudioshared.PromotionRunConditionStatus, reason appstudioshared.PromotionRunReasonType) (*appstudioshared.ApplicationPromotionRun, error) {

	now := metav1.Now()
	promotionRun.Status.Conditions = append(promotionRun.Status.Conditions,
		appstudioshared.PromotionRunCondition{
			Type:               conditionType,
			Message:            message,
			LastProbeTime:      now,
			LastTransitionTime: &now,
			Status:             status,
			Reason:             reason,
		})

	if err := client.Status().Update(ctx, promotionRun); err != nil {
		return promotionRun, err
	}

	return promotionRun, nil
}

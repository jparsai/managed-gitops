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

package v1alpha1

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var environmentlog = logf.Log.WithName("environment-resource")

func (r *Environment) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

//+kubebuilder:webhook:path=/mutate-appstudio-redhat-com-redhat-com-v1alpha1-environment,mutating=true,failurePolicy=fail,sideEffects=None,groups=appstudio.redhat.com.redhat.com,resources=environments,verbs=create;update,versions=v1alpha1,name=menvironment.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &Environment{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *Environment) Default() {
	environmentlog.Info("default", "name", r.Name)

	// TODO(user): fill in your defaulting logic.
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:path=/validate-appstudio-redhat-com-redhat-com-v1alpha1-environment,mutating=false,failurePolicy=fail,sideEffects=None,groups=appstudio.redhat.com.redhat.com,resources=environments,verbs=create;update,versions=v1alpha1,name=venvironment.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &Environment{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *Environment) ValidateCreate() error {
	environmentlog.Info("validate create", "name", r.Name)

	fmt.Println("1....#############################")
	fmt.Println("1....#############################")
	fmt.Println("1....#############################")

	// TODO(user): fill in your validation logic upon object creation.
	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *Environment) ValidateUpdate(old runtime.Object) error {
	environmentlog.Info("validate update", "name", r.Name)

	fmt.Println("2....#############################")
	fmt.Println("2....#############################")
	fmt.Println("2....#############################")

	// TODO(user): fill in your validation logic upon object update.
	return nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *Environment) ValidateDelete() error {
	environmentlog.Info("validate delete", "name", r.Name)

	fmt.Println("3....#############################")
	fmt.Println("3....#############################")
	fmt.Println("3....#############################")

	// TODO(user): fill in your validation logic upon object deletion.
	return nil
}

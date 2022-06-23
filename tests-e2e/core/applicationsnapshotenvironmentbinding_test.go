package core

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/k8s"

	appstudiosharedv1 "github.com/redhat-appstudio/managed-gitops/appstudio-shared/apis/appstudio.redhat.com/v1alpha1"
	bindingFixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/binding"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("ApplicationSnapshotEnvironmentBinding Reconciler E2E tests", func() {

	Context("Testing ApplicationSnapshotEnvironmentBinding Reconciler.", func() {

		It("should update status of Binding.", func() {
			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			// Create Binding Object
			binding := buildApplicationSnapshotEnvironmentBindingResource(
				"appa-staging-binding",
				"new-demo-app",
				"staging",
				"component-a",
				"my-snapshot",
				3,
			)
			err := k8s.Create(&binding)
			Expect(err).To(Succeed())

			err = k8s.Get(&binding)
			Expect(err).To(BeNil())

			fmt.Println("11..binding === ", binding.Status)

			status := appstudiosharedv1.ApplicationSnapshotEnvironmentBindingStatus{
				Components: []appstudiosharedv1.ComponentStatus{
					{
						Name: "component-a",
						GitOpsRepository: appstudiosharedv1.BindingComponentGitOpsRepository{
							URL:                "https://github.com/redhat-appstudio/gitops-repository-template",
							Branch:             "main",
							Path:               "components/componentA/overlays/staging",
							GeneratedResources: []string{},
						},
					},
				},
			}
			binding.Status = status
			k8s.UpdateStatus(&binding)

			Eventually(binding, "2m", "1s").Should(
				SatisfyAll(
					bindingFixture.HaveStatusComponents(status.Components),
				),
			)
		})
	})
})

func buildApplicationSnapshotEnvironmentBindingResource(name, appName, envName, componentName, snapShotName string, replica int) appstudiosharedv1.ApplicationSnapshotEnvironmentBinding {
	// Create ApplicationSnapshotEnvironmentBinding CR.
	binding := appstudiosharedv1.ApplicationSnapshotEnvironmentBinding{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ApplicationSnapshotEnvironmentBinding",
			APIVersion: "appstudio.redhat.com/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: fixture.GitOpsServiceE2ENamespace,
		},
		Spec: appstudiosharedv1.ApplicationSnapshotEnvironmentBindingSpec{
			Application: appName,
			Environment: envName,
			Snapshot:    snapShotName,
			Components: []appstudiosharedv1.BindingComponent{
				{
					Name: componentName,
					Configuration: appstudiosharedv1.BindingComponentConfiguration{
						Replicas: replica,
					},
				},
			},
		},
	}
	return binding
}

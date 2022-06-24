package core

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/k8s"
	"k8s.io/apimachinery/pkg/util/uuid"

	appstudiosharedv1 "github.com/redhat-appstudio/managed-gitops/appstudio-shared/apis/appstudio.redhat.com/v1alpha1"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend/apis/managed-gitops/v1alpha1"
	bindingFixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/binding"
	gitopsDeplFixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/gitopsdeployment"

	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("ApplicationSnapshotEnvironmentBinding Reconciler E2E tests", func() {

	Context("Testing ApplicationSnapshotEnvironmentBinding Reconciler.", func() {

		It("Should update Status of Binding and create GitOpsDeployment.", func() {
			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			// Create Binding in Cluster
			binding := buildApplicationSnapshotEnvironmentBindingResource("appa-staging-binding", "new-demo-app",
				"staging", "component-a", "my-snapshot", 3)
			err := k8s.Create(&binding)
			Expect(err).To(Succeed())

			// Update the Status field ob Binding, because it is not updated while creating object
			status := buildApplicationSnapshotEnvironmentBindingStatus(binding.Spec.Components[0].Name, "https://github.com/redhat-appstudio/gitops-repository-template",
				"main", "components/componentA/overlays/staging")

			err = k8s.Get(&binding)
			Expect(err).To(Succeed())
			binding.Status = status
			err = k8s.UpdateStatus(&binding)
			Expect(err).To(Succeed())

			// Check Binding Status has Component.
			Eventually(binding, "2m", "1s").Should(bindingFixture.HaveStatusComponents(status.Components))

			// Check GitOpsDeployment is having meta data as given in Binding
			gitOpsDeployment := buildGitOpsDeploymentObjectMeta(binding.Name+"-"+binding.Spec.Application+"-"+binding.Spec.Environment+"-"+binding.Spec.Components[0].Name, binding.Namespace)

			Eventually(gitOpsDeployment, "2m", "1s").Should(gitopsDeplFixture.HaveSource(managedgitopsv1alpha1.ApplicationSource{
				RepoURL:        binding.Status.Components[0].GitOpsRepository.URL,
				Path:           binding.Status.Components[0].GitOpsRepository.Path,
				TargetRevision: binding.Status.Components[0].GitOpsRepository.Branch,
			}))
		})

		It("Should update GitOpsDeployment, if Binding metadata is updated.", func() {
			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			// Create Binding in Cluster
			binding := buildApplicationSnapshotEnvironmentBindingResource("appa-staging-binding", "new-demo-app",
				"staging", "component-a", "my-snapshot", 3)
			err := k8s.Create(&binding)
			Expect(err).To(Succeed())

			// Update the Status field ob Binding, because it is not updated while creating object
			status := buildApplicationSnapshotEnvironmentBindingStatus(binding.Spec.Components[0].Name, "https://github.com/redhat-appstudio/gitops-repository-template",
				"main", "components/componentA/overlays/staging")

			err = k8s.Get(&binding)
			Expect(err).To(Succeed())
			binding.Status = status
			err = k8s.UpdateStatus(&binding)
			Expect(err).To(Succeed())

			// Check Binding Status has Component.
			Eventually(binding, "2m", "1s").Should(bindingFixture.HaveStatusComponents(status.Components))

			// Check GitOpsDeployment is having meta data as given in Binding
			gitOpsDeployment := buildGitOpsDeploymentObjectMeta(binding.Name+"-"+binding.Spec.Application+"-"+binding.Spec.Environment+"-"+binding.Spec.Components[0].Name, binding.Namespace)

			Eventually(gitOpsDeployment, "2m", "1s").Should(gitopsDeplFixture.HaveSource(managedgitopsv1alpha1.ApplicationSource{
				RepoURL:        binding.Status.Components[0].GitOpsRepository.URL,
				Path:           binding.Status.Components[0].GitOpsRepository.Path,
				TargetRevision: binding.Status.Components[0].GitOpsRepository.Branch,
			}))

			err = k8s.Get(&binding)
			Expect(err).To(Succeed())
			binding.Status.Components[0].GitOpsRepository.Path = "components/componentA/overlays/dev"
			statusTemp := binding.Status
			err = k8s.UpdateStatus(&binding)
			Expect(err).To(Succeed())

			// Check Binding Status has Component.
			Eventually(binding, "2m", "1s").Should(bindingFixture.HaveStatusComponents(statusTemp.Components))

			// Check GitOpsDeployment is having meta data as given in Binding
			gitOpsDeploymentSecond := buildGitOpsDeploymentObjectMeta(binding.Name+"-"+binding.Spec.Application+"-"+binding.Spec.Environment+"-"+binding.Spec.Components[0].Name, binding.Namespace)

			Eventually(gitOpsDeploymentSecond, "2m", "1s").Should(gitopsDeplFixture.HaveSource(managedgitopsv1alpha1.ApplicationSource{
				RepoURL:        binding.Status.Components[0].GitOpsRepository.URL,
				Path:           binding.Status.Components[0].GitOpsRepository.Path,
				TargetRevision: binding.Status.Components[0].GitOpsRepository.Branch,
			}))
		})

		It("Should recreate GitOpsDeployment, if it is deleted.", func() {
			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			// Create Binding in Cluster
			binding := buildApplicationSnapshotEnvironmentBindingResource("appa-staging-binding", "new-demo-app",
				"staging", "component-a", "my-snapshot", 3)
			err := k8s.Create(&binding)
			Expect(err).To(Succeed())

			// Update the Status field ob Binding, because it is not updated while creating object
			status := buildApplicationSnapshotEnvironmentBindingStatus(binding.Spec.Components[0].Name, "https://github.com/redhat-appstudio/gitops-repository-template",
				"main", "components/componentA/overlays/staging")

			err = k8s.Get(&binding)
			Expect(err).To(Succeed())
			binding.Status = status
			err = k8s.UpdateStatus(&binding)
			Expect(err).To(Succeed())

			// Check Binding Status has Component.
			Eventually(binding, "2m", "1s").Should(bindingFixture.HaveStatusComponents(status.Components))

			// Check GitOpsDeployment is having meta data as given in Binding
			gitOpsDeployment := buildGitOpsDeploymentObjectMeta(binding.Name+"-"+binding.Spec.Application+"-"+binding.Spec.Environment+"-"+binding.Spec.Components[0].Name, binding.Namespace)

			Eventually(gitOpsDeployment, "2m", "1s").Should(gitopsDeplFixture.HaveSource(managedgitopsv1alpha1.ApplicationSource{
				RepoURL:        binding.Status.Components[0].GitOpsRepository.URL,
				Path:           binding.Status.Components[0].GitOpsRepository.Path,
				TargetRevision: binding.Status.Components[0].GitOpsRepository.Branch,
			}))

			err = k8s.Delete(&gitOpsDeployment)
			Expect(err).To(Succeed())

			err = k8s.Get(&gitOpsDeployment)
			Expect(err).NotTo(Succeed())
			Expect(apierr.IsNotFound(err)).To(BeTrue())

			// Update any value in Binding just to trigger Reconciler.
			err = k8s.Get(&binding)
			Expect(err).To(Succeed())
			binding.Spec.Components[0].Configuration.Replicas = 2
			err = k8s.Update(&binding)
			Expect(err).To(Succeed())

			gitOpsDeploymentSecond := buildGitOpsDeploymentObjectMeta(binding.Name+"-"+binding.Spec.Application+"-"+binding.Spec.Environment+"-"+binding.Spec.Components[0].Name, binding.Namespace)
			err = k8s.Get(&gitOpsDeploymentSecond)
			Expect(err).To(Succeed())

			Eventually(gitOpsDeploymentSecond, "2m", "1s").Should(gitopsDeplFixture.HaveSource(managedgitopsv1alpha1.ApplicationSource{
				RepoURL:        binding.Status.Components[0].GitOpsRepository.URL,
				Path:           binding.Status.Components[0].GitOpsRepository.Path,
				TargetRevision: binding.Status.Components[0].GitOpsRepository.Branch,
			}))
		})

		It("Should keep GitOpsDeployment in Sync with Binding.", func() {
			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			// Create Binding in Cluster
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

			// Update the status in Cluster, because Status field is not updated while creating object
			err = k8s.Get(&binding)
			Expect(err).To(Succeed())
			status := buildApplicationSnapshotEnvironmentBindingStatus(
				binding.Spec.Components[0].Name,
				"https://github.com/redhat-appstudio/gitops-repository-template",
				"main",
				"components/componentA/overlays/staging")
			binding.Status = status
			err = k8s.UpdateStatus(&binding)
			Expect(err).To(Succeed())

			// Check Binding Status has Component.
			Eventually(binding, "2m", "1s").Should(bindingFixture.HaveStatusComponents(status.Components))

			gitOpsDeployment := buildGitOpsDeploymentObjectMeta(
				binding.Name+"-"+
					binding.Spec.Application+"-"+
					binding.Spec.Environment+"-"+
					binding.Spec.Components[0].Name,
				binding.Namespace)

			// Fetch GitOpsDeployment
			err = k8s.Get(&gitOpsDeployment)
			Expect(err).To(Succeed())

			sourcetemp := gitOpsDeployment.Spec.Source

			gitOpsDeployment.Spec.Source.Path = "components/componentA/overlays/dev"
			err = k8s.Update(&gitOpsDeployment)
			Expect(err).To(Succeed())

			// Update any value in Binding just to trigger Reconciler.
			err = k8s.Get(&binding)
			Expect(err).To(Succeed())
			binding.Spec.Components[0].Configuration.Replicas = 2
			err = k8s.Update(&binding)
			Expect(err).To(Succeed())

			// Reconciler should revert the GitOpsDeployment in Cluster.
			Eventually(gitOpsDeployment, "2m", "1s").Should(gitopsDeplFixture.HaveSource(sourcetemp))
		})

		It("Should use short name for GitOpsDeployment, if Name field length is more than max length.", func() {
			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			// Create Binding object.
			binding := buildApplicationSnapshotEnvironmentBindingResource(
				"appa-staging-binding",
				"new-demo-app",
				"staging",
				"component-a",
				"my-snapshot",
				3,
			)

			// Create Binding in Cluster
			binding.Spec.Application = strings.Repeat("abcde", 45)
			err := k8s.Create(&binding)
			Expect(err).To(Succeed())

			// Update the status in Cluster, because Status field is not updated while creating object
			err = k8s.Get(&binding)
			Expect(err).To(Succeed())

			status := buildApplicationSnapshotEnvironmentBindingStatus(
				binding.Spec.Components[0].Name,
				"https://github.com/redhat-appstudio/gitops-repository-template",
				"main",
				"components/componentA/overlays/staging")
			binding.Status = status
			err = k8s.UpdateStatus(&binding)
			Expect(err).To(Succeed())

			gitOpsDeployment := buildGitOpsDeploymentObjectMeta(
				binding.Name+"-"+
					binding.Spec.Application+"-"+
					binding.Spec.Environment+"-"+
					binding.Spec.Components[0].Name,
				binding.Namespace)

			err = k8s.Get(&gitOpsDeployment)
			Expect(apierr.IsNotFound(err)).To(BeTrue())

			gitOpsDeployment.Name = binding.Name + "-" + binding.Spec.Components[0].Name
			err = k8s.Get(&gitOpsDeployment)
			Expect(err).To(Succeed())

			// Check GitOpsDeployment is having meta data as given in Binding.
			Eventually(gitOpsDeployment, "2m", "1s").Should(gitopsDeplFixture.HaveSource(managedgitopsv1alpha1.ApplicationSource{
				RepoURL:        status.Components[0].GitOpsRepository.URL,
				Path:           status.Components[0].GitOpsRepository.Path,
				TargetRevision: status.Components[0].GitOpsRepository.Branch,
			}))
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

func buildApplicationSnapshotEnvironmentBindingStatus(name, url, branch, path string) appstudiosharedv1.ApplicationSnapshotEnvironmentBindingStatus {

	// Create ApplicationSnapshotEnvironmentBindingStatus object.
	status := appstudiosharedv1.ApplicationSnapshotEnvironmentBindingStatus{
		Components: []appstudiosharedv1.ComponentStatus{
			{
				Name: name,
				GitOpsRepository: appstudiosharedv1.BindingComponentGitOpsRepository{
					URL:                url,
					Branch:             branch,
					Path:               path,
					GeneratedResources: []string{},
				},
			},
		},
	}
	return status
}

func buildGitOpsDeploymentObjectMeta(name, namespace string) managedgitopsv1alpha1.GitOpsDeployment {
	gitOpsDeployment := managedgitopsv1alpha1.GitOpsDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	return gitOpsDeployment
}

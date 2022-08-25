package core

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appstudiocontroller "github.com/redhat-appstudio/managed-gitops/appstudio-controller/controllers/appstudio.redhat.com"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/k8s"

	appstudiosharedv1 "github.com/redhat-appstudio/managed-gitops/appstudio-shared/apis/appstudio.redhat.com/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("GitOpsDeployment E2E tests", func() {
	Context("Create a new GitOpsDeployment", func() {
		It("should be healthy and have synced status, and resources should be deployed", func() {
			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			environmentStage := buildEnvironmentResource("staging", "Staging Environment", "staging", appstudiosharedv1.EnvironmentType_POC)
			err := k8s.Create(&environmentStage)
			Expect(err).To(Succeed())

			environmentProd := buildEnvironmentResource("prod", "Production Environment", "prod", appstudiosharedv1.EnvironmentType_POC)
			err = k8s.Create(&environmentProd)
			Expect(err).To(Succeed())

			applicationSnapshot := buildApplicationSnapshotResource("my-snapshot", "new-demo-app", "Staging Snapshot", "Staging Snapshot", "component-a", "quay.io/jgwest-redhat/sample-workload:latest")
			err = k8s.Create(&applicationSnapshot)
			Expect(err).To(Succeed())

			bindingStage := buildApplicationSnapshotEnvironmentBindingResource("appa-staging-binding", "new-demo-app", "staging", "my-snapshot", 3, []string{"component-a", "component-b"})
			err = k8s.Create(&bindingStage)
			Expect(err).To(Succeed())

			// Update Status field
			err = k8s.Get(&bindingStage)
			Expect(err).To(Succeed())
			bindingStage.Status = buildApplicationSnapshotEnvironmentBindingStatus(bindingStage.Spec.Components, "https://github.com/redhat-appstudio/gitops-repository-template", "main", []string{"components/componentA/overlays/staging", "components/componentB/overlays/staging"})
			err = k8s.UpdateStatus(&bindingStage)
			Expect(err).To(Succeed())

			bindingProd := buildApplicationSnapshotEnvironmentBindingResource("appa-prod-binding", "new-demo-app", "prod", "my-snapshot", 3, []string{"component-a", "component-b"})
			err = k8s.Create(&bindingProd)
			Expect(err).To(Succeed())

			// Update Status field
			err = k8s.Get(&bindingProd)
			Expect(err).To(Succeed())
			bindingProd.Status = buildApplicationSnapshotEnvironmentBindingStatus(bindingProd.Spec.Components, "https://github.com/redhat-appstudio/gitops-repository-template", "main", []string{"components/componentA/overlays/staging", "components/componentB/overlays/staging"})
			err = k8s.UpdateStatus(&bindingProd)
			Expect(err).To(Succeed())

			//====================================================
			By("Verify that Status.GitOpsDeployments field of Binding is having Component and GitOpsDeployment name.")

			time.Sleep(2 * time.Minute)
			gitOpsDeploymentNameStage := appstudiocontroller.GenerateBindingGitOpsDeploymentName(bindingStage, bindingStage.Spec.Components[0].Name)
			fmt.Println("gitOpsDeploymentNameStage == ", gitOpsDeploymentNameStage)

			/*expectedGitOpsDeploymentStage := []appstudiosharedv1.BindingStatusGitOpsDeployment{
				{ComponentName: bindingStage.Spec.Components[0].Name, GitOpsDeployment: gitOpsDeploymentNameStage},
			}
			Eventually(bindingStage, "2m", "1s").Should(bindingFixture.HaveStatusGitOpsDeployments(expectedGitOpsDeploymentStage))*/

			gitOpsDeploymentNameProd := appstudiocontroller.GenerateBindingGitOpsDeploymentName(bindingProd, bindingProd.Spec.Components[0].Name)
			fmt.Println("gitOpsDeploymentNameProd == ", gitOpsDeploymentNameProd)

			/*expectedGitOpsDeploymentProd := []appstudiosharedv1.BindingStatusGitOpsDeployment{
				{ComponentName: bindingProd.Spec.Components[0].Name, GitOpsDeployment: gitOpsDeploymentNameProd},
			}
			Eventually(bindingProd, "2m", "1s").Should(bindingFixture.HaveStatusGitOpsDeployments(expectedGitOpsDeploymentProd))*/

			gitOpsDeploymentStage := v1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      gitOpsDeploymentNameStage,
					Namespace: bindingStage.Namespace,
				},
			}
			err = k8s.Get(&gitOpsDeploymentStage)
			Expect(err).To(Succeed())

			gitOpsDeploymentProd := v1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      gitOpsDeploymentNameProd,
					Namespace: bindingProd.Namespace,
				},
			}
			err = k8s.Get(&gitOpsDeploymentProd)
			Expect(err).To(Succeed())

			promotionRun := buildPromotionRunResource("new-demo-app-manual-promotion", "new-demo-app", "my-snapshot", "prod")
			err = k8s.Create(&promotionRun)
			Expect(err).To(Succeed())

			err = k8s.Get(&promotionRun)
			fmt.Println("promotionRun.Status = ", promotionRun.Status)
			fmt.Println("promotionRun.Status.ActiveBindings = ", promotionRun.Status.ActiveBindings)
			fmt.Println("promotionRun.Status.EnvironmentStatus = ", promotionRun.Status.EnvironmentStatus)
			fmt.Println("promotionRun.Status.PromotionStartTime = ", promotionRun.Status.PromotionStartTime)
			fmt.Println("promotionRun.Status.CompletionResult = ", promotionRun.Status.CompletionResult)
			fmt.Println("promotionRun.Status.Conditions = ", promotionRun.Status.Conditions)
			fmt.Println("promotionRun.Status.State = ", promotionRun.Status.State)

			Expect(err).To(Succeed())

			time.Sleep(1 * time.Minute)
			err = k8s.Get(&promotionRun)
			fmt.Println("promotionRun.Status = ", promotionRun.Status)
			fmt.Println("promotionRun.Status.ActiveBindings = ", promotionRun.Status.ActiveBindings)
			fmt.Println("promotionRun.Status.EnvironmentStatus = ", promotionRun.Status.EnvironmentStatus)
			fmt.Println("promotionRun.Status.PromotionStartTime = ", promotionRun.Status.PromotionStartTime)
			fmt.Println("promotionRun.Status.CompletionResult = ", promotionRun.Status.CompletionResult)
			fmt.Println("promotionRun.Status.Conditions = ", promotionRun.Status.Conditions)
			fmt.Println("promotionRun.Status.State = ", promotionRun.Status.State)

			Expect(err).To(Succeed())
		})
	})
})

func buildEnvironmentResource(name, displayName, parentEnvironment string, envType appstudiosharedv1.EnvironmentType) appstudiosharedv1.Environment {
	environment := appstudiosharedv1.Environment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "appstudio.redhat.com/v1alpha1",
			Kind:       "Environment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: fixture.GitOpsServiceE2ENamespace,
		},
		Spec: appstudiosharedv1.EnvironmentSpec{
			DisplayName:        displayName,
			Type:               envType,
			DeploymentStrategy: appstudiosharedv1.DeploymentStrategy_AppStudioAutomated,
			ParentEnvironment:  parentEnvironment,
			Tags:               []string{name},
			Configuration: appstudiosharedv1.EnvironmentConfiguration{
				Env: []appstudiosharedv1.EnvVarPair{},
			},
		},
	}
	/*

	   metadata:
	     name: staging
	   spec:
	     type: poc
	     displayName: “Production for Team A”
	     deploymentStrategy: AppStudioAutomated
	     parentEnvironment: staging
	     tags:
	       - staging
	     configuration:
	       env:
	         - name: My_STG_ENV
	           value: "100"


	*/

	return environment
}

func buildApplicationSnapshotResource(name, appName, displayName, displayDescription, componentName, containerImage string) appstudiosharedv1.ApplicationSnapshot {
	applicationSnapshot := appstudiosharedv1.ApplicationSnapshot{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "appstudio.redhat.com/v1alpha1",
			Kind:       "ApplicationSnapshot",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: fixture.GitOpsServiceE2ENamespace,
		},
		Spec: appstudiosharedv1.ApplicationSnapshotSpec{
			Application:        appName,
			DisplayName:        displayName,
			DisplayDescription: displayDescription,
			Components: []appstudiosharedv1.ApplicationSnapshotComponent{
				{
					Name:           componentName,
					ContainerImage: containerImage,
				},
			},
		},
	}
	return applicationSnapshot
}

func buildPromotionRunResource(name, appName, snapshotName, targetEnvironment string) appstudiosharedv1.ApplicationPromotionRun {

	promotionRun := appstudiosharedv1.ApplicationPromotionRun{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "appstudio.redhat.com/v1alpha1",
			Kind:       "ApplicationPromotionRun",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: fixture.GitOpsServiceE2ENamespace,
		},
		Spec: appstudiosharedv1.ApplicationPromotionRunSpec{
			Snapshot:    snapshotName,
			Application: appName,
			ManualPromotion: appstudiosharedv1.ManualPromotionConfiguration{
				TargetEnvironment: targetEnvironment,
			},
		},
	}
	return promotionRun
}

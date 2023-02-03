package core

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redhat-appstudio/application-api/api/v1alpha1"
	appstudiosharedv1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/k8s"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Webhook Tests.", func() {

	Context("Testing Snapshot Webhook.", func() {
		var err error
		var ctx context.Context
		var k8sClient client.Client
		var snapshot v1alpha1.Snapshot

		BeforeEach(func() {
			Expect(fixture.EnsureCleanSlate()).To(Succeed())
			ctx = context.Background()

			k8sClient, err = fixture.GetE2ETestUserWorkspaceKubeClient()
			Expect(err).To(Succeed())

			By("Create Snapshot CR.")

			snapshot = buildSnapshotResource("my-snapshot", "new-demo-app", "Staging Snapshot", "Staging Snapshot", "component-a", "quay.io/jgwest-redhat/sample-workload:latest")
			err = k8s.Create(&snapshot, k8sClient)
			Expect(err).To(Succeed())

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&snapshot), &snapshot)
			Expect(err).To(Succeed())
		})

		FIt("Should not allow to change Spec.Application field.", func() {

			By("Change field and update Snapshot CR.")

			snapshot.Spec.Application = "new-name"
			err = k8sClient.Update(ctx, &snapshot)
			fmt.Println("err == ", err)
			Expect(err).NotTo(Succeed())
		})

		FIt("Should not allow to change Spec.Components.Name field.", func() {

			By("Change field and update Snapshot CR.")

			snapshot.Spec.Components[0].Name = "new-name"
			err = k8sClient.Update(ctx, &snapshot)
			fmt.Println("err == ", err)
			Expect(err).NotTo(Succeed())
		})

		FIt("Should not allow to change Spec.Components.ContainerImage field.", func() {

			By("Change field and update Snapshot CR.")

			snapshot.Spec.Components[0].ContainerImage = "new-image-name"
			err = k8sClient.Update(ctx, &snapshot)
			fmt.Println("err == ", err)
			Expect(err).NotTo(Succeed())
		})
	})

	Context("Testing Snapshot Webhook.", func() {
		var err error
		var ctx context.Context
		var k8sClient client.Client
		var environment appstudiosharedv1.Environment
		var binding appstudiosharedv1.SnapshotEnvironmentBinding

		BeforeEach(func() {
			Expect(fixture.EnsureCleanSlate()).To(Succeed())
			ctx = context.Background()

			k8sClient, err = fixture.GetE2ETestUserWorkspaceKubeClient()
			Expect(err).To(Succeed())

			By("creating the Environment CR")

			environment = appstudiosharedv1.Environment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "staging",
					Namespace: fixture.GitOpsServiceE2ENamespace,
				},
				Spec: appstudiosharedv1.EnvironmentSpec{
					Type:               appstudiosharedv1.EnvironmentType_POC,
					DisplayName:        "my-environment",
					DeploymentStrategy: appstudiosharedv1.DeploymentStrategy_AppStudioAutomated,
					ParentEnvironment:  "",
					Tags:               []string{},
					Configuration: appstudiosharedv1.EnvironmentConfiguration{
						Env: []appstudiosharedv1.EnvVarPair{},
					},
				},
			}

			err = k8s.Create(&environment, k8sClient)
			Expect(err).To(Succeed())

			binding = buildSnapshotEnvironmentBindingResource("appa-staging-binding", "new-demo-app", "staging", "my-snapshot", 3, []string{"component-a", "component-b"})
			binding.Labels = make(map[string]string)
			err = k8s.Create(&binding, k8sClient)
			Expect(err).To(Succeed())

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&binding), &binding)
			Expect(err).To(Succeed())
		})

		FIt("Should not allow to change Spec.Application field.", func() {

			By("Change field and update Snapshot CR.")

			binding.Spec.Application = "new-name"
			err = k8sClient.Update(ctx, &binding)

			fmt.Println("err == ", err)
			Expect(err).NotTo(Succeed())
		})

		FIt("Should not allow to change Spec.Environment field.", func() {

			By("Change field and update Snapshot CR.")

			binding.Spec.Environment = "new-env"
			err = k8sClient.Update(ctx, &binding)

			fmt.Println("err == ", err)
			Expect(err).NotTo(Succeed())
		})

		FIt("Should not allow to change Spec.Environment field.", func() {

			By("Change field and update Snapshot CR.")

			binding.Labels = make(map[string]string)
			err = k8sClient.Update(ctx, &binding)

			fmt.Println("err == ", err)
			Expect(err).NotTo(Succeed())
		})

	})
})

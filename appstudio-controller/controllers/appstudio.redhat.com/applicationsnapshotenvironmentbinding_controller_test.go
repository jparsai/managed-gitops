package appstudioredhatcom

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appstudiosharedv1 "github.com/redhat-appstudio/managed-gitops/appstudio-shared/apis/appstudio.redhat.com/v1alpha1"
	apibackend "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/tests"
)

var _ = Describe("ApplicationSnapshotEnvironmentBinding Reconciler Tests", func() {

	Context("Testing ApplicationSnapshotEnvironmentBindingReconciler.", func() {
		var ctx context.Context
		var request reconcile.Request
		var binding *appstudiosharedv1.ApplicationSnapshotEnvironmentBinding
		var bindingReconciler ApplicationSnapshotEnvironmentBindingReconciler

		var environment appstudiosharedv1.Environment

		BeforeEach(func() {
			ctx = context.Background()

			scheme,
				argocdNamespace,
				kubesystemNamespace,
				apiNamespace,
				err := tests.GenericTestSetup()
			Expect(err).To(BeNil())

			// Create fake client
			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(apiNamespace, argocdNamespace, kubesystemNamespace).
				Build()

			err = appstudiosharedv1.AddToScheme(scheme)
			Expect(err).To(BeNil())

			// Create placeholder environment
			environment = appstudiosharedv1.Environment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "staging",
					Namespace: apiNamespace.Name,
				},
				Spec: appstudiosharedv1.EnvironmentSpec{
					DisplayName:        "my-environment",
					Type:               appstudiosharedv1.EnvironmentType_POC,
					DeploymentStrategy: appstudiosharedv1.DeploymentStrategy_AppStudioAutomated,
					ParentEnvironment:  "",
					Tags:               []string{},
					Configuration:      appstudiosharedv1.EnvironmentConfiguration{},
				},
			}
			err = k8sClient.Create(ctx, &environment)
			Expect(err).To(BeNil())

			bindingReconciler = ApplicationSnapshotEnvironmentBindingReconciler{Client: k8sClient, Scheme: scheme}

			// Create ApplicationSnapshotEnvironmentBinding CR.
			binding = &appstudiosharedv1.ApplicationSnapshotEnvironmentBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "appa-staging-binding",
					Namespace: apiNamespace.Name,
					Labels: map[string]string{
						"appstudio.application": "new-demo-app",
						"appstudio.environment": "staging",
					},
				},
				Spec: appstudiosharedv1.ApplicationSnapshotEnvironmentBindingSpec{
					Application: "new-demo-app",
					Environment: "staging",
					Snapshot:    "my-snapshot",
					Components: []appstudiosharedv1.BindingComponent{
						{
							Name: "component-a",
							Configuration: appstudiosharedv1.BindingComponentConfiguration{
								Env: []appstudiosharedv1.EnvVarPair{
									{Name: "My_STG_ENV", Value: "1000"},
								},
								Replicas: 3,
							},
						},
					},
				},
				Status: appstudiosharedv1.ApplicationSnapshotEnvironmentBindingStatus{
					Components: []appstudiosharedv1.ComponentStatus{
						{
							Name: "component-a",
							GitOpsRepository: appstudiosharedv1.BindingComponentGitOpsRepository{
								URL:    "https://github.com/redhat-appstudio/gitops-repository-template",
								Branch: "main",
								Path:   "components/componentA/overlays/staging",
							},
						},
					},
				},
			}

			// Create request object for Reconciler
			request = newRequest(apiNamespace.Name, binding.Name)
		})

		It("Should set the status field of Binding.", func() {
			// Create ApplicationSnapshotEnvironmentBinding CR in cluster.
			err := bindingReconciler.Create(ctx, binding)
			Expect(err).To(BeNil())

			// Check status field before calling Reconciler
			bindingFirst := &appstudiosharedv1.ApplicationSnapshotEnvironmentBinding{}
			err = bindingReconciler.Client.Get(ctx, request.NamespacedName, bindingFirst)
			Expect(err).To(BeNil())
			Expect(len(bindingFirst.Status.GitOpsDeployments)).To(Equal(0))

			// Trigger Reconciler
			_, err = bindingReconciler.Reconcile(ctx, request)
			Expect(err).To(BeNil())

			// Check status field after calling Reconciler
			bindingSecond := &appstudiosharedv1.ApplicationSnapshotEnvironmentBinding{}
			err = bindingReconciler.Client.Get(ctx, request.NamespacedName, bindingSecond)

			Expect(err).To(BeNil())
			Expect(len(bindingSecond.Status.GitOpsDeployments)).NotTo(Equal(0))
			Expect(bindingSecond.Status.GitOpsDeployments[0].ComponentName).To(Equal("component-a"))
			Expect(bindingSecond.Status.GitOpsDeployments[0].GitOpsDeployment).
				To(Equal(binding.Name + "-" +
					binding.Spec.Application + "-" +
					binding.Spec.Environment + "-" +
					binding.Spec.Components[0].Name))
		})

		It("Should not update GitOpsDeployment if same Binding is created again.", func() {

			// Create ApplicationSnapshotEnvironmentBinding CR in cluster.
			err := bindingReconciler.Create(ctx, binding)
			Expect(err).To(BeNil())

			// Trigger Reconciler
			_, err = bindingReconciler.Reconcile(ctx, request)
			Expect(err).To(BeNil())

			// Fetch GitOpsDeployment object before calling Reconciler
			gitopsDeploymentKey := client.ObjectKey{
				Namespace: binding.Namespace,
				Name:      GenerateBindingGitOpsDeploymentName(*binding, binding.Spec.Components[0].Name),
			}

			gitopsDeploymentFirst := &apibackend.GitOpsDeployment{}
			err = bindingReconciler.Get(ctx, gitopsDeploymentKey, gitopsDeploymentFirst)
			Expect(err).To(BeNil())

			// Trigger Reconciler again
			_, err = bindingReconciler.Reconcile(ctx, request)
			Expect(err).To(BeNil())

			// Fetch GitOpsDeployment object after calling Reconciler
			gitopsDeploymentSecond := &apibackend.GitOpsDeployment{}
			err = bindingReconciler.Get(ctx, gitopsDeploymentKey, gitopsDeploymentSecond)
			Expect(err).To(BeNil())

			// Reconciler should not do any change in GitOpsDeployment object.
			Expect(gitopsDeploymentFirst).To(Equal(gitopsDeploymentSecond))
		})

		It("Should revert GitOpsDeploymentObject if it's spec is different than Binding Component.", func() {
			// Create ApplicationSnapshotEnvironmentBinding CR in cluster.
			err := bindingReconciler.Create(ctx, binding)
			Expect(err).To(BeNil())

			// Trigger Reconciler
			_, err = bindingReconciler.Reconcile(ctx, request)
			Expect(err).To(BeNil())

			gitopsDeploymentKey := client.ObjectKey{
				Namespace: binding.Namespace,
				Name:      GenerateBindingGitOpsDeploymentName(*binding, binding.Spec.Components[0].Name),
			}

			// Fetch GitOpsDeployment object
			gitopsDeployment := &apibackend.GitOpsDeployment{}
			err = bindingReconciler.Get(ctx, gitopsDeploymentKey, gitopsDeployment)
			Expect(err).To(BeNil())

			// GitOpsDeployment object spec should be same as Binding Component.
			Expect(gitopsDeployment.Spec.Source.Path).To(Equal(binding.Status.Components[0].GitOpsRepository.Path))

			// Update GitOpsDeploymentObject in cluster.
			gitopsDeployment.Spec.Source.Path = "components/componentA/overlays/dev"
			err = bindingReconciler.Update(ctx, gitopsDeployment)
			Expect(err).To(BeNil())

			// Trigger Reconciler again
			_, err = bindingReconciler.Reconcile(ctx, request)
			Expect(err).To(BeNil())

			// Fetch GitOpsDeployment object after calling Reconciler
			err = bindingReconciler.Get(ctx, gitopsDeploymentKey, gitopsDeployment)
			Expect(err).To(BeNil())

			// Reconciler should revert GitOpsDeployment object, so it will be same as old object
			Expect(gitopsDeployment.Spec.Source.Path).To(Equal(binding.Status.Components[0].GitOpsRepository.Path))
		})

		It("Should use short name for GitOpsDeployment object.", func() {
			// Update application name to exceed the limit
			binding.Spec.Application = strings.Repeat("abcde", 45)
			request = newRequest(binding.Namespace, binding.Name)

			// Create ApplicationSnapshotEnvironmentBinding CR in cluster.
			err := bindingReconciler.Create(ctx, binding)
			Expect(err).To(BeNil())

			// Trigger Reconciler
			_, err = bindingReconciler.Reconcile(ctx, request)
			Expect(err).To(BeNil())

			// Check status field after calling Reconciler
			binding := &appstudiosharedv1.ApplicationSnapshotEnvironmentBinding{}
			err = bindingReconciler.Client.Get(ctx, request.NamespacedName, binding)

			Expect(err).To(BeNil())
			Expect(len(binding.Status.GitOpsDeployments)).NotTo(Equal(0))
			Expect(binding.Status.GitOpsDeployments[0].ComponentName).To(Equal("component-a"))

			// GitOpsDeployment should have short name
			Expect(binding.Status.GitOpsDeployments[0].GitOpsDeployment).
				To(Equal(binding.Name + "-" + binding.Spec.Components[0].Name))
		})

		It("Should use short name with hash value for GitOpsDeployment, if combination of Binding name and Component name is still longer than 250 characters.", func() {
			compName := strings.Repeat("abcde", 50)

			// Update application name to exceed the limit
			binding.Status.Components[0].Name = compName
			request = newRequest(binding.Namespace, binding.Name)

			// Create ApplicationSnapshotEnvironmentBinding CR in cluster.
			err := bindingReconciler.Create(ctx, binding)
			Expect(err).To(BeNil())

			// Trigger Reconciler
			_, err = bindingReconciler.Reconcile(ctx, request)
			Expect(err).To(BeNil())

			// Check status field after calling Reconciler
			binding := &appstudiosharedv1.ApplicationSnapshotEnvironmentBinding{}
			err = bindingReconciler.Client.Get(ctx, request.NamespacedName, binding)

			Expect(err).To(BeNil())
			Expect(len(binding.Status.GitOpsDeployments)).NotTo(Equal(0))

			// 210 (First 210 characters of combination of Binding name and Component name) + 1 ("-")+32 (length of UUID) = 243 (Total length)
			Expect(len(binding.Status.GitOpsDeployments[0].GitOpsDeployment)).To(Equal(243))

			// Get the short name with hash value.
			hasher := md5.New()
			hasher.Write([]byte(binding.Name + "-" + binding.Spec.Application + "-" + binding.Spec.Environment + "-" + compName))
			hashValue := hex.EncodeToString(hasher.Sum(nil))
			expectedName := (binding.Name + "-" + compName)[0:210] + "-" + hashValue

			Expect(binding.Status.GitOpsDeployments[0].GitOpsDeployment).
				To(Equal(expectedName))
		})

		It("Should not return error if Status.Components is not available in Binding object.", func() {
			binding.Status.Components = []appstudiosharedv1.ComponentStatus{}
			// Create ApplicationSnapshotEnvironmentBinding CR in cluster.
			err := bindingReconciler.Create(ctx, binding)
			Expect(err).To(BeNil())

			// Trigger Reconciler
			_, err = bindingReconciler.Reconcile(ctx, request)
			Expect(err).To(BeNil())
			// Expect(err.Error()).To(Equal("ApplicationSnapshotEventBinding Component status is required to generate GitOps deployment, waiting for the Application Service controller to finish reconciling binding appa-staging-binding"))

			// TODO: GITOPSRVCE-182: Once GITOPSRVCE-182 is implemented, check for the above message in the condition.

		})

		It("Should return error if Status.GitOpsRepoConditions Status is set to False in Binding object.", func() {
			binding.Status.GitOpsRepoConditions = []metav1.Condition{
				{
					Status: metav1.ConditionFalse,
				},
			}

			// Create ApplicationSnapshotEnvironmentBinding CR in cluster.
			err := bindingReconciler.Create(ctx, binding)
			Expect(err).To(BeNil())

			// Trigger Reconciler
			_, err = bindingReconciler.Reconcile(ctx, request)
			Expect(err).To(BeNil())
		})

		It("should not return an error if there are duplicate components in binding.Status.Components", func() {

			By("creating an ApplicationSnapshotEnvironmentBinding with duplicate component names")

			binding.Status.Components = []appstudiosharedv1.ComponentStatus{
				{
					Name: "componentA",
					GitOpsRepository: appstudiosharedv1.BindingComponentGitOpsRepository{
						URL:                "https://url",
						Branch:             "branch",
						Path:               "path",
						GeneratedResources: []string{},
					},
				},
				{
					Name: "componentA",
					GitOpsRepository: appstudiosharedv1.BindingComponentGitOpsRepository{
						URL:                "https://url2",
						Branch:             "branch2",
						Path:               "path2",
						GeneratedResources: []string{},
					},
				},
			}

			// Create ApplicationSnapshotEnvironmentBinding CR in cluster.
			err := bindingReconciler.Create(ctx, binding)
			Expect(err).To(BeNil())

			// Trigger Reconciler
			_, err = bindingReconciler.Reconcile(ctx, request)
			Expect(err).To(BeNil())

			// Expect(strings.Contains(err.Error(), errDuplicateKeysFound)).To(BeTrue(),
			// 	"duplicate components should be detected by the reconciler")
			// TODO: GITOPSRVCE-182: Once GITOPSRVCE-182 is implemented, check for the above message in the condition.

		})

		It("should verify that if the Environment contains configuration information, that it is included in the generate GitOpsDeployment", func() {

			By("creating an Environment with valid configuration fields")
			environment.Spec.UnstableConfigurationFields = &appstudiosharedv1.UnstableEnvironmentConfiguration{
				KubernetesClusterCredentials: appstudiosharedv1.KubernetesClusterCredentials{
					TargetNamespace:          "my-target-namespace",
					APIURL:                   "my-api-url",
					ClusterCredentialsSecret: "secret",
				},
			}
			err := bindingReconciler.Client.Update(ctx, &environment)
			Expect(err).To(BeNil())

			By("creating default Binding")
			err = bindingReconciler.Client.Create(ctx, binding)
			Expect(err).To(BeNil())

			By("calling Reconcile")
			request = newRequest(binding.Namespace, binding.Name)
			_, err = bindingReconciler.Reconcile(ctx, request)
			Expect(err).To(BeNil())

			By("ensuring that the GitOpsDeployment was created using values from ConfigurationFields")

			gitopsDeploymentKey := client.ObjectKey{
				Namespace: binding.Namespace,
				Name:      GenerateBindingGitOpsDeploymentName(*binding, binding.Spec.Components[0].Name),
			}
			gitopsDeployment := &apibackend.GitOpsDeployment{}
			err = bindingReconciler.Get(ctx, gitopsDeploymentKey, gitopsDeployment)
			Expect(err).To(BeNil())

			Expect(gitopsDeployment.Spec.Destination.Namespace).To(Equal(environment.Spec.UnstableConfigurationFields.TargetNamespace))

			By("removing the field from Environment, and ensuring the GitOpsDeployment is updated")
			environment.Spec.UnstableConfigurationFields = nil
			err = bindingReconciler.Client.Update(ctx, &environment)
			Expect(err).To(BeNil())

			By("reconciling again")
			_, err = bindingReconciler.Reconcile(ctx, request)
			Expect(err).To(BeNil())

			err = bindingReconciler.Get(ctx, gitopsDeploymentKey, gitopsDeployment)
			Expect(err).To(BeNil())
			Expect(gitopsDeployment.Spec.Destination.Namespace).To(Equal(""))
			Expect(gitopsDeployment.Spec.Destination.Environment).To(Equal(""))

			By("testing with a missing TargetNamespace, which should return an error")
			environment.Spec.UnstableConfigurationFields = &appstudiosharedv1.UnstableEnvironmentConfiguration{
				KubernetesClusterCredentials: appstudiosharedv1.KubernetesClusterCredentials{
					APIURL:                   "my-api-url",
					ClusterCredentialsSecret: "secret",
				},
			}
			err = bindingReconciler.Client.Update(ctx, &environment)
			Expect(err).To(BeNil())

			By("reconciling again, and expecting an TargetNamespace missing error")
			_, err = bindingReconciler.Reconcile(ctx, request)
			Expect(err).ToNot(BeNil())
			Expect(strings.Contains(err.Error(), errMissingTargetNamespace)).To(BeTrue())

		})
	})
})

// newRequest contains the information necessary to reconcile a Kubernetes object.
func newRequest(namespace, name string) reconcile.Request {
	return reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		},
	}
}

package appstudioredhatcom_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appstudiocontrollers "github.com/redhat-appstudio/managed-gitops/appstudio-controller/controllers/appstudio.redhat.com"
	appstudiosharedv1 "github.com/redhat-appstudio/managed-gitops/appstudio-shared/apis/appstudio.redhat.com/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/eventlooptypes"
)

var _ = Describe("Application Snapshot Environment Binding Reconciler Tests", func() {

	Context("Testing ApplicationSnapshotEnvironmentBindingReconciler.", func() {

		It("Should update status of Binding.", func() {
			scheme,
				argocdNamespace,
				kubesystemNamespace,
				workspace,
				err := eventlooptypes.GenericTestSetup()
			Expect(err).To(BeNil())

			// Register scheme
			err = appstudiosharedv1.AddToScheme(scheme)
			Expect(err).To(BeNil())

			// Create fake client
			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(workspace, argocdNamespace, kubesystemNamespace).
				Build()

			bindingReconciler := appstudiocontrollers.ApplicationSnapshotEnvironmentBindingReconciler{
				Client: k8sClient, Scheme: scheme}

			// Create ApplicationSnapshotEnvironmentBinding in cluster
			binding := &appstudiosharedv1.ApplicationSnapshotEnvironmentBinding{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ApplicationSnapshotEnvironmentBinding",
					APIVersion: "appstudio.redhat.com/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "appa-staging-binding",
					Namespace: kubesystemNamespace.Name,
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
							GitOpsRepository: appstudiosharedv1.BindingComponentGitOpsRepository{
								URL:    "https://github.com/redhat-appstudio/gitops-repository-template",
								Branch: "main",
								Path:   "components/componentA/overlays/staging",
							},
						},
					},
				},
			}

			ctx := context.Background()
			err = bindingReconciler.Create(ctx, binding)
			Expect(err).To(BeNil())

			// Create request object for Reconciler
			request := newRequest(kubesystemNamespace.Name, binding.Name)

			// Check status before calling Reconciler
			bindingFirst := &appstudiosharedv1.ApplicationSnapshotEnvironmentBinding{}
			err = bindingReconciler.Client.Get(ctx, request.NamespacedName, bindingFirst)
			Expect(err).To(BeNil())
			Expect(len(bindingFirst.Status.GitOpsDeployments)).To(Equal(0))

			_, err = bindingReconciler.Reconcile(ctx, request)
			Expect(err).To(BeNil())

			// Check status after calling Reconciler
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

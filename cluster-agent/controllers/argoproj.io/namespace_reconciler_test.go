package argoprojio

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/eventlooptypes"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"

	appv1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Namespace Reconciler Tests.", func() {
	Context("Testing for Namespace Reconciler.", func() {
		It("Should consider ArgoCD Application as an orphaned and delete it, if application entry doesnt exists in DB.", func() {
			ctx := context.Background()
			log := log.FromContext(ctx)

			scheme, _, _, _, err := eventlooptypes.GenericTestSetup()
			Expect(err).To(BeNil())

			err = appv1.AddToScheme(scheme)
			Expect(err).To(BeNil())

			// Fake kube client.
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			reconciler := ApplicationReconciler{Client: k8sClient}

			var argoApplications []appv1.Application
			argoApplications = append(argoApplications, appv1.Application{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"databaseID": "test-my-application-1"}}})
			argoApplications = append(argoApplications, appv1.Application{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"databaseID": "test-my-application-2"}}})
			argoApplications = append(argoApplications, appv1.Application{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"databaseID": "test-my-application-3"}}})
			argoApplications = append(argoApplications, appv1.Application{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"databaseID": "test-my-application-4"}}})
			argoApplications = append(argoApplications, appv1.Application{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"databaseID": "test-my-application-5"}}})

			processedApplicationIds := map[string]bool{"test-my-application-3": false, "test-my-application-5": false}

			deletedArgoApplications := deleteOrphanedApplications(argoApplications, processedApplicationIds, ctx, reconciler.Client, log)

			Expect(len(deletedArgoApplications)).To(Equal(3))

			deletedApplicationIds := map[string]string{"test-my-application-1": "", "test-my-application-2": "", "test-my-application-4": ""}
			for _, app := range deletedArgoApplications {
				_, ok := deletedApplicationIds[app.Labels["databaseID"]]
				Expect(ok).To(BeTrue())
			}
		})
	})
})

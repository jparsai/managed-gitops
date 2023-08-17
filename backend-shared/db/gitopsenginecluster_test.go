package db_test

import (
	"context"
	"strings"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	db "github.com/redhat-appstudio/managed-gitops/backend-shared/db"
)

var _ = Describe("Gitopsenginecluster Test", func() {
	It("Should Create, Get and Delete a GitopsEngineCluster", func() {
		err := db.SetupForTestingDBGinkgo()
		Expect(err).ToNot(HaveOccurred())

		ctx := context.Background()
		dbq, err := db.NewUnsafePostgresDBQueries(true, true)
		Expect(err).ToNot(HaveOccurred())
		defer dbq.CloseDatabase()

		clusterCredentials := db.ClusterCredentials{
			Clustercredentials_cred_id:  "test-cluster-creds-test-1",
			Host:                        "host",
			Kube_config:                 "kube-config",
			Kube_config_context:         "kube-config-context",
			Serviceaccount_bearer_token: "serviceaccount_bearer_token",
			Serviceaccount_ns:           "Serviceaccount_ns",
		}

		gitopsEngineClusterput := db.GitopsEngineCluster{
			Gitopsenginecluster_id: "test-fake-cluster-1",
			Clustercredentials_id:  clusterCredentials.Clustercredentials_cred_id,
		}

		err = dbq.CreateClusterCredentials(ctx, &clusterCredentials)
		Expect(err).ToNot(HaveOccurred())

		err = dbq.CreateGitopsEngineCluster(ctx, &gitopsEngineClusterput)
		Expect(err).ToNot(HaveOccurred())

		gitopsEngineClusterget := db.GitopsEngineCluster{
			Gitopsenginecluster_id: gitopsEngineClusterput.Gitopsenginecluster_id,
		}

		err = dbq.GetGitopsEngineClusterById(ctx, &gitopsEngineClusterget)
		Expect(err).ToNot(HaveOccurred())
		Expect(gitopsEngineClusterput).Should(Equal(gitopsEngineClusterget))

		rowsAffected, err := dbq.DeleteGitopsEngineClusterById(ctx, gitopsEngineClusterput.Gitopsenginecluster_id)
		Expect(err).ToNot(HaveOccurred())
		Expect(rowsAffected).Should(Equal(1))

		err = dbq.GetGitopsEngineClusterById(ctx, &gitopsEngineClusterget)
		Expect(true).To(Equal(db.IsResultNotFoundError(err)))

		gitopsEngineClusterget = db.GitopsEngineCluster{
			Gitopsenginecluster_id: "does-not-exist"}
		err = dbq.GetGitopsEngineClusterById(ctx, &gitopsEngineClusterget)
		Expect(true).To(Equal(db.IsResultNotFoundError(err)))

		gitopsEngineClusterput.Clustercredentials_id = strings.Repeat("abc", 100)
		err = dbq.CreateGitopsEngineCluster(ctx, &gitopsEngineClusterput)
		Expect(true).To(Equal(db.IsMaxLengthError(err)))

	})

	It("Should Get ClusterCredentials in batch.", func() {

		err := db.SetupForTestingDBGinkgo()
		Expect(err).ToNot(HaveOccurred())

		ctx := context.Background()
		dbq, err := db.NewUnsafePostgresDBQueries(true, true)
		Expect(err).ToNot(HaveOccurred())
		defer dbq.CloseDatabase()

		clusterCredentials := db.ClusterCredentials{
			Clustercredentials_cred_id:  "test-cluster-creds-test-1",
			Host:                        "host",
			Kube_config:                 "kube-config",
			Kube_config_context:         "kube-config-context",
			Serviceaccount_bearer_token: "serviceaccount_bearer_token",
			Serviceaccount_ns:           "Serviceaccount_ns",
		}
		err = dbq.CreateClusterCredentials(ctx, &clusterCredentials)
		Expect(err).ToNot(HaveOccurred())

		By("Create multiple GitopsEngineCluster entries.")

		gitopsEngineCluster := db.GitopsEngineCluster{
			Gitopsenginecluster_id: "test-" + uuid.NewString(),
			Clustercredentials_id:  clusterCredentials.Clustercredentials_cred_id,
		}
		err = dbq.CreateGitopsEngineCluster(ctx, &gitopsEngineCluster)
		Expect(err).ToNot(HaveOccurred())

		gitopsEngineCluster.Gitopsenginecluster_id = "test-" + uuid.NewString()
		err = dbq.CreateGitopsEngineCluster(ctx, &gitopsEngineCluster)
		Expect(err).ToNot(HaveOccurred())

		gitopsEngineCluster.Gitopsenginecluster_id = "test-" + uuid.NewString()
		err = dbq.CreateGitopsEngineCluster(ctx, &gitopsEngineCluster)
		Expect(err).ToNot(HaveOccurred())

		gitopsEngineCluster.Gitopsenginecluster_id = "test-" + uuid.NewString()
		err = dbq.CreateGitopsEngineCluster(ctx, &gitopsEngineCluster)
		Expect(err).ToNot(HaveOccurred())

		gitopsEngineCluster.Gitopsenginecluster_id = "test-" + uuid.NewString()
		err = dbq.CreateGitopsEngineCluster(ctx, &gitopsEngineCluster)
		Expect(err).ToNot(HaveOccurred())

		By("Get data in batch.")

		var listOfGitopsEngineClusterFromDB []db.GitopsEngineCluster
		err = dbq.GetGitopsEngineClusterBatch(ctx, &listOfGitopsEngineClusterFromDB, 2, 0)
		Expect(err).ToNot(HaveOccurred())
		Expect(listOfGitopsEngineClusterFromDB).To(HaveLen(2))

		err = dbq.GetGitopsEngineClusterBatch(ctx, &listOfGitopsEngineClusterFromDB, 3, 1)
		Expect(err).ToNot(HaveOccurred())
		Expect(listOfGitopsEngineClusterFromDB).To(HaveLen(3))
	})
})

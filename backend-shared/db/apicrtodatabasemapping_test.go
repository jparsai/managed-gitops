package db_test

import (
	"context"
	"strings"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	db "github.com/redhat-appstudio/managed-gitops/backend-shared/db"
)

var _ = Describe("Apicrtodatabasemapping Tests", func() {
	Context("Tests all the DB functions for Apicrtodatabasemapping", func() {
		It("Should execute all Apicrtodatabasemapping Functions", func() {
			err := db.SetupForTestingDBGinkgo()
			Expect(err).ToNot(HaveOccurred())

			item := db.APICRToDatabaseMapping{
				APIResourceType:      db.APICRToDatabaseMapping_ResourceType_GitOpsDeploymentSyncRun,
				APIResourceUID:       "test-k8s-uid",
				APIResourceName:      "test-k8s-name",
				APIResourceNamespace: "test-k8s-namespace",
				NamespaceUID:         "test-namespace-uid",
				DBRelationType:       db.APICRToDatabaseMapping_DBRelationType_SyncOperation,
				DBRelationKey:        "test-key",
			}
			ctx := context.Background()
			dbq, err := db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).ToNot(HaveOccurred())
			defer dbq.CloseDatabase()
			err = dbq.CreateAPICRToDatabaseMapping(ctx, &item)
			Expect(err).ToNot(HaveOccurred())

			fetchRow := db.APICRToDatabaseMapping{
				APIResourceType: item.APIResourceType,
				APIResourceUID:  item.APIResourceUID,
				DBRelationKey:   item.DBRelationKey,
				DBRelationType:  item.DBRelationType,
			}

			err = dbq.GetDatabaseMappingForAPICR(ctx, &fetchRow)
			Expect(err).ToNot(HaveOccurred())
			Expect(fetchRow).Should(Equal(item))

			var items []db.APICRToDatabaseMapping

			err = dbq.ListAPICRToDatabaseMappingByAPINamespaceAndName(ctx, item.APIResourceType, item.APIResourceName, item.APIResourceNamespace, item.NamespaceUID, item.DBRelationType, &items)
			Expect(err).ToNot(HaveOccurred())
			Expect(items[0]).Should(Equal(item))

			rowsAffected, err := dbq.DeleteAPICRToDatabaseMapping(ctx, &fetchRow)
			Expect(err).ToNot(HaveOccurred())
			Expect(rowsAffected).To(Equal((1)))
			fetchRow = db.APICRToDatabaseMapping{
				APIResourceType: item.APIResourceType,
				APIResourceUID:  item.APIResourceUID,
				DBRelationKey:   item.DBRelationKey,
				DBRelationType:  item.DBRelationType,
			}
			err = dbq.GetDatabaseMappingForAPICR(ctx, &fetchRow)
			Expect(db.IsResultNotFoundError(err)).To(BeTrue())

			// Set the invalid value
			item.APIResourceName = strings.Repeat("abc", 100)
			err = dbq.CreateAPICRToDatabaseMapping(ctx, &item)
			Expect(db.IsMaxLengthError(err)).To(BeTrue())

		})

		It("Should Get APICRToDatabaseMapping in batch.", func() {
			err := db.SetupForTestingDBGinkgo()
			Expect(err).ToNot(HaveOccurred())

			ctx := context.Background()
			dbq, err := db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).ToNot(HaveOccurred())

			defer dbq.CloseDatabase()

			By("Create multiple APICRToDatabaseMapping entries.")

			apiCRToDatabaseMapping := db.APICRToDatabaseMapping{
				APIResourceType:      db.APICRToDatabaseMapping_ResourceType_GitOpsDeploymentSyncRun,
				APIResourceUID:       "test-k8s-uid" + uuid.NewString(),
				APIResourceName:      "test-k8s-name",
				APIResourceNamespace: "test-k8s-namespace",
				NamespaceUID:         "test-namespace-uid",
				DBRelationType:       db.APICRToDatabaseMapping_DBRelationType_SyncOperation,
				DBRelationKey:        uuid.NewString(),
			}
			err = dbq.CreateAPICRToDatabaseMapping(ctx, &apiCRToDatabaseMapping)
			Expect(err).ToNot(HaveOccurred())

			apiCRToDatabaseMapping.APIResourceUID, apiCRToDatabaseMapping.DBRelationKey = "test-k8s-uid"+uuid.NewString(), uuid.NewString()
			err = dbq.CreateAPICRToDatabaseMapping(ctx, &apiCRToDatabaseMapping)
			Expect(err).ToNot(HaveOccurred())

			apiCRToDatabaseMapping.APIResourceUID, apiCRToDatabaseMapping.DBRelationKey = "test-k8s-uid"+uuid.NewString(), uuid.NewString()
			err = dbq.CreateAPICRToDatabaseMapping(ctx, &apiCRToDatabaseMapping)
			Expect(err).ToNot(HaveOccurred())

			apiCRToDatabaseMapping.APIResourceUID, apiCRToDatabaseMapping.DBRelationKey = "test-k8s-uid"+uuid.NewString(), uuid.NewString()
			err = dbq.CreateAPICRToDatabaseMapping(ctx, &apiCRToDatabaseMapping)
			Expect(err).ToNot(HaveOccurred())

			apiCRToDatabaseMapping.APIResourceUID, apiCRToDatabaseMapping.DBRelationKey = "test-k8s-uid"+uuid.NewString(), uuid.NewString()
			err = dbq.CreateAPICRToDatabaseMapping(ctx, &apiCRToDatabaseMapping)
			Expect(err).ToNot(HaveOccurred())

			By("Get data in batch.")

			var listOfAPICRToDatabaseMappingFromDB []db.APICRToDatabaseMapping
			err = dbq.GetAPICRToDatabaseMappingBatch(ctx, &listOfAPICRToDatabaseMappingFromDB, 2, 0)
			Expect(err).ToNot(HaveOccurred())
			Expect(listOfAPICRToDatabaseMappingFromDB).To(HaveLen(2))

			err = dbq.GetAPICRToDatabaseMappingBatch(ctx, &listOfAPICRToDatabaseMappingFromDB, 3, 1)
			Expect(err).ToNot(HaveOccurred())
			Expect(listOfAPICRToDatabaseMappingFromDB).To(HaveLen(3))
		})
	})
})

package db

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestIsEmptyValues(t *testing.T) {

	for _, c := range []struct {
		// name is human-readable test name
		name          string
		params        []interface{}
		expectedError bool
	}{
		{
			name:          "valid",
			params:        []interface{}{"key", "value"},
			expectedError: false,
		},
		{
			name:          "empty key w/ string value",
			params:        []interface{}{"", "value"},
			expectedError: true,
		},
		{
			name:          "empty value w/ string key",
			params:        []interface{}{"key", ""},
			expectedError: true,
		},
		{
			name:          "empty string value, w/ string key",
			params:        []interface{}{"key", ""},
			expectedError: true,
		},
		{
			name:          "empty interface value, w/ string key",
			params:        []interface{}{"key", nil},
			expectedError: true,
		},
		{
			name:          "empty interface key, w/ string value",
			params:        []interface{}{nil, ""},
			expectedError: true,
		},
		{
			name:          "multiple valid string values",
			params:        []interface{}{"key", "value", "key2", "value2"},
			expectedError: false,
		},
		{
			name:          "multiple string values, one invalid",
			params:        []interface{}{"key", "value", "key2", ""},
			expectedError: true,
		},
		{
			name:          "multiple mixed values, one invalid",
			params:        []interface{}{"key", nil, "key2", ""},
			expectedError: true,
		},
		{
			name:          "multiple mixed values, one invalid, different order",
			params:        []interface{}{"key", "hi", "key2", nil},
			expectedError: true,
		},

		{
			name:          "invalid number of params",
			params:        []interface{}{"key", "value", "key2"},
			expectedError: true,
		},
		{
			name:          "no params",
			params:        []interface{}{},
			expectedError: true,
		},
	} {
		t.Run(c.name, func(t *testing.T) {

			res := isEmptyValues(c.name, c.params...)
			assert.True(t, (res != nil) == c.expectedError)

		})
	}
}

var _ = Describe("Utils Test", func() {
	Context("Utils test", func() {
		It("Should return error for Clusteruser_id", func() {
			user := &ClusterUser{
				Clusteruser_id: strings.Repeat("abc", 17),
				User_name:      "user-name",
			}
			err := ValidateFieldLength(user)
			Expect(err).NotTo(BeNil())
		})

		It("Should return error for User_name", func() {
			user := &ClusterUser{
				Clusteruser_id: "user-id",
				User_name:      strings.Repeat("abc", 86),
			}
			err := ValidateFieldLength(user)
			Expect(err).NotTo(BeNil())
		})

		It("Should pass", func() {
			user := &ClusterUser{
				Clusteruser_id: "user-id",
				User_name:      "user-name",
			}
			err := ValidateFieldLength(user)
			Expect(err).To(BeNil())
		})

		It("Should return error for User_name", func() {
			gitopsEngineCluster := &GitopsEngineCluster{
				Gitopsenginecluster_id: "user-id",
				SeqID:                  123,
			}
			err := ValidateFieldLength(gitopsEngineCluster)
			Expect(err).To(BeNil())
		})

		It("Should return error for User_name", func() {
			gitopsEngineCluster := &GitopsEngineCluster{
				Gitopsenginecluster_id: strings.Repeat("abc", 17),
				SeqID:                  123,
			}
			err := ValidateFieldLength(gitopsEngineCluster)
			Expect(err).NotTo(BeNil())
		})
	})
})

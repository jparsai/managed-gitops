package core

import (
	"testing"
	"time"

	"github.com/onsi/ginkgo/v2/config"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestCore(t *testing.T) {
	// A test is "slow" if it takes longer than a minute
	config.DefaultReporterConfig.SlowSpecThreshold = time.Hour.Seconds() * 60
	RegisterFailHandler(Fail)
	RunSpecs(t, "Core Suite")
}

package containerjfr_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestContainerjfr(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Containerjfr Suite")
}

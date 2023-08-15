package controllers_test

import (
	"github.com/cryostatio/cryostat-operator/internal/controllers"
	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("CryostatController", func() {
	c := &controllerTest{
		clusterScoped:   false,
		constructorFunc: newCryostatController,
	}

	c.commonTests()
})

func newCryostatController(config *controllers.ReconcilerConfig) controllers.CommonReconciler {
	return controllers.NewCryostatReconciler(config)
}

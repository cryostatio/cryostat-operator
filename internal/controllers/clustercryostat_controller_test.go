// Copyright The Cryostat Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package controllers_test

import (
	"github.com/cryostatio/cryostat-operator/internal/controllers"
	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("ClusterCryostatController", func() {
	c := controllerTest{
		clusterScoped:   true,
		constructorFunc: newClusterCryostatController,
	}

	c.commonTests()
})

func newClusterCryostatController(config *controllers.ReconcilerConfig) (controllers.CommonReconciler, error) {
	return controllers.NewClusterCryostatReconciler(config)
}

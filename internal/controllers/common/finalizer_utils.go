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

package common

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// AddFinalizer adds the provided finalizer to the object and updates it in the cluster
func AddFinalizer(ctx context.Context, client client.Client, obj controllerutil.Object, finalizer string) error {
	log.Info("adding finalizer to object", "namespace", obj.GetNamespace(), "name", obj.GetName())
	controllerutil.AddFinalizer(obj, finalizer)
	err := client.Update(ctx, obj)
	if err != nil {
		log.Error(err, "failed to add finalizer to object", "namespace", obj.GetNamespace(),
			"name", obj.GetName())
		return err
	}
	return nil
}

// RemoveFinalizer removes the provided finalizer from the object and updates it in the cluster
func RemoveFinalizer(ctx context.Context, client client.Client, obj controllerutil.Object, finalizer string) error {
	log.Info("removing finalizer from object", "namespace", obj.GetNamespace(), "name", obj.GetName())
	controllerutil.RemoveFinalizer(obj, finalizer)
	err := client.Update(ctx, obj)
	if err != nil {
		log.Error(err, "failed to remove finalizer from object", "namespace", obj.GetNamespace(),
			"name", obj.GetName())
		return err
	}

	return nil
}

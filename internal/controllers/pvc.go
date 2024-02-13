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

package controllers

import (
	"context"
	"fmt"

	operatorv1beta2 "github.com/cryostatio/cryostat-operator/api/v1beta2"
	"github.com/cryostatio/cryostat-operator/internal/controllers/common"
	"github.com/cryostatio/cryostat-operator/internal/controllers/model"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// Event type to inform users of invalid PVC specs
const eventPersistentVolumeClaimInvalidType = "PersistentVolumeClaimInvalid"

func (r *Reconciler) reconcilePVC(ctx context.Context, cr *model.CryostatInstance) error {
	emptyDir := cr.Spec.StorageOptions != nil && cr.Spec.StorageOptions.EmptyDir != nil && cr.Spec.StorageOptions.EmptyDir.Enabled
	if emptyDir {
		// If user requested an emptyDir volume, then do nothing.
		// Don't delete the PVC to prevent accidental data loss
		// depending on the reclaim policy.
		return nil
	}
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name,
			Namespace: cr.InstallNamespace,
		},
	}

	// Look up PVC configuration, applying defaults where needed
	config := configurePVC(cr)

	err := r.createOrUpdatePVC(ctx, pvc, cr.Object, config)
	if err != nil {
		// If the API server says the PVC is invalid, emit a warning event
		// to inform the user.
		if kerrors.IsInvalid(err) {
			r.EventRecorder.Event(cr.Object, corev1.EventTypeWarning, eventPersistentVolumeClaimInvalidType, err.Error())
		}
		return err
	}
	return nil
}

func (r *Reconciler) createOrUpdatePVC(ctx context.Context, pvc *corev1.PersistentVolumeClaim,
	owner metav1.Object, config *operatorv1beta2.PersistentVolumeClaimConfig) error {
	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, pvc, func() error {
		// Merge labels and annotations to prevent overriding any set by Kubernetes
		common.MergeLabelsAndAnnotations(&pvc.ObjectMeta, config.Labels, config.Annotations)

		// Set the Cryostat CR as controller
		if err := controllerutil.SetControllerReference(owner, pvc, r.Scheme); err != nil {
			return err
		}

		// Several PVC spec fields are modified by Kubernetes controllers (e.g. VolumeName, StorageClassName).
		// Resetting those modifications to the values from our CR results in an infinite loop of
		// modifications between our controller and the Kubernetes controllers.
		// To avoid this, only set the PVC spec fields (except resources) during creation.
		// In most cases, updates to these fields are invalid anyways.
		if pvc.CreationTimestamp.IsZero() {
			pvc.Spec = *config.Spec
		} else {
			// Resource requests can be expanded, and in rare cases shrunken. Let the modification proceed,
			// and if not admitted, let the user know with a warning Event.
			requestedStorage := config.Spec.Resources.Requests.Storage()
			pvc.Spec.Resources.Requests[corev1.ResourceStorage] = *requestedStorage
		}
		return nil
	})
	if err != nil {
		return err
	}
	r.Log.Info(fmt.Sprintf("Persistent Volume Claim %s", op), "name", pvc.Name, "namespace", pvc.Namespace)
	return nil
}

func configurePVC(cr *model.CryostatInstance) *operatorv1beta2.PersistentVolumeClaimConfig {
	// Check for PVC config within CR
	var config *operatorv1beta2.PersistentVolumeClaimConfig
	if cr.Spec.StorageOptions == nil || cr.Spec.StorageOptions.PVC == nil {
		config = &operatorv1beta2.PersistentVolumeClaimConfig{}
	} else {
		config = cr.Spec.StorageOptions.PVC.DeepCopy()
	}

	if config.Labels == nil {
		config.Labels = map[string]string{}
	}
	if config.Annotations == nil {
		config.Annotations = map[string]string{}
	}
	if config.Spec == nil {
		config.Spec = &corev1.PersistentVolumeClaimSpec{}
	}

	// Add "app" label. This will override any user-specified "app" label.
	config.Labels["app"] = cr.Name

	// Apply any applicable spec defaults. Don't apply a default storage class name, since nil
	// may be intentionally specified.
	if config.Spec.Resources.Requests == nil {
		config.Spec.Resources.Requests = corev1.ResourceList{
			corev1.ResourceStorage: *resource.NewQuantity(500*1024*1024, resource.BinarySI),
		}
	}
	if config.Spec.AccessModes == nil {
		config.Spec.AccessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}
	}

	return config
}

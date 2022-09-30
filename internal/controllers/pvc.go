// Copyright The Cryostat Authors
//
// The Universal Permissive License (UPL), Version 1.0
//
// Subject to the condition set forth below, permission is hereby granted to any
// person obtaining a copy of this software, associated documentation and/or data
// (collectively the "Software"), free of charge and under any and all copyright
// rights in the Software, and any and all patent rights owned or freely
// licensable by each licensor hereunder covering either (i) the unmodified
// Software as contributed to or provided by such licensor, or (ii) the Larger
// Works (as defined below), to deal in both
//
// (a) the Software, and
// (b) any piece of software and/or hardware listed in the lrgrwrks.txt file if
// one is included with the Software (each a "Larger Work" to which the Software
// is contributed by such licensors),
//
// without restriction, including without limitation the rights to copy, create
// derivative works of, display, perform, and distribute the Software and make,
// use, sell, offer for sale, import, export, have made, and have sold the
// Software and the Larger Work(s), and to sublicense the foregoing rights on
// either these or other terms.
//
// This license is subject to the following condition:
// The above copyright notice and either this complete permission notice or at
// a minimum a reference to the UPL must be included in all copies or
// substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package controllers

import (
	"context"
	"fmt"

	operatorv1beta1 "github.com/cryostatio/cryostat-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// Event type to inform users of invalid PVC specs
const eventPersistentVolumeClaimInvalidType = "PersistentVolumeClaimInvalid"

func (r *CryostatReconciler) reconcilePVC(ctx context.Context, cr *operatorv1beta1.Cryostat) error {
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
			Namespace: cr.Namespace,
		},
	}

	// Look up PVC configuration, applying defaults where needed
	config := configurePVC(cr)

	err := r.createOrUpdatePVC(ctx, pvc, cr, config)
	if err != nil {
		// If the API server says the PVC is invalid, emit a warning event
		// to inform the user.
		if kerrors.IsInvalid(err) {
			r.EventRecorder.Event(cr, corev1.EventTypeWarning, eventPersistentVolumeClaimInvalidType, err.Error())
		}
		return err
	}
	return nil
}

func (r *CryostatReconciler) createOrUpdatePVC(ctx context.Context, pvc *corev1.PersistentVolumeClaim,
	owner metav1.Object, config *operatorv1beta1.PersistentVolumeClaimConfig) error {
	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, pvc, func() error {
		// Update labels and annotations
		pvc.Labels = config.Labels
		pvc.Annotations = config.Annotations
		// Set the Cryostat CR as controller
		if err := controllerutil.SetControllerReference(owner, pvc, r.Scheme); err != nil {
			return err
		}
		// PVC admission control is complex and can depend on StorageClass implementation, see:
		// https://kubernetes.io/docs/reference/access-authn-authz/admission-controllers/#persistentvolumeclaimresize
		// Apply the PVC spec as requested, and send an Event if the creation/update fails.
		pvc.Spec = *config.Spec
		return nil
	})
	if err != nil {
		return err
	}
	r.Log.Info(fmt.Sprintf("Persistent Volume Claim %s", op), "name", pvc.Name, "namespace", pvc.Namespace)
	return nil
}

func configurePVC(cr *operatorv1beta1.Cryostat) *operatorv1beta1.PersistentVolumeClaimConfig {
	// Check for PVC config within CR
	var config *operatorv1beta1.PersistentVolumeClaimConfig
	if cr.Spec.StorageOptions == nil || cr.Spec.StorageOptions.PVC == nil {
		config = &operatorv1beta1.PersistentVolumeClaimConfig{}
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

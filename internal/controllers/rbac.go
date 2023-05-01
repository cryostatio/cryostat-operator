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
	"encoding/json"
	"errors"
	"fmt"

	"github.com/cryostatio/cryostat-operator/internal/controllers/common"
	"github.com/cryostatio/cryostat-operator/internal/controllers/model"
	"github.com/google/go-cmp/cmp"
	oauthv1 "github.com/openshift/api/oauth/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *Reconciler) reconcileRBAC(ctx context.Context, cr *model.CryostatInstance) error {
	err := r.reconcileServiceAccount(ctx, cr)
	if err != nil {
		return err
	}
	err = r.reconcileRole(ctx, cr)
	if err != nil {
		return err
	}
	err = r.reconcileRoleBinding(ctx, cr)
	if err != nil {
		return err
	}
	err = r.reconcileClusterRoleBinding(ctx, cr)
	if err != nil {
		return err
	}
	return nil
}

func (r *Reconciler) finalizeRBAC(ctx context.Context, cr *model.CryostatInstance) error {
	return r.deleteClusterRoleBinding(ctx, newClusterRoleBinding(cr))
}

func newServiceAccount(cr *model.CryostatInstance) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name,
			Namespace: cr.InstallNamespace,
		},
	}
}

func (r *Reconciler) reconcileServiceAccount(ctx context.Context, cr *model.CryostatInstance) error {
	sa := newServiceAccount(cr)
	labels := map[string]string{
		"app": cr.Name,
	}
	annotations := map[string]string{}
	// If running on OpenShift, set the route reference as an annotation.
	// This will tell OpenShift's OAuth to redirect to the route when
	// this Service Account is used as an OAuth client.
	if r.IsOpenShift {
		oAuthRedirectReference := &oauthv1.OAuthRedirectReference{
			Reference: oauthv1.RedirectReference{
				Kind: "Route",
				Name: newCoreRoute(cr).Name,
			},
		}

		ref, err := json.Marshal(oAuthRedirectReference)
		if err != nil {
			return err
		}

		annotations["serviceaccounts.openshift.io/oauth-redirectreference.route"] = string(ref)
	}
	return r.createOrUpdateServiceAccount(ctx, sa, cr.Object, labels, annotations)
}

func newRole(cr *model.CryostatInstance) *rbacv1.Role {
	return &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name,
			Namespace: cr.InstallNamespace,
		},
	}
}

func (r *Reconciler) reconcileRole(ctx context.Context, cr *model.CryostatInstance) error {
	// Replaced by a cluster role, clean up any legacy role
	return r.cleanUpRole(ctx, cr, newRole(cr))
}

func newRoleBinding(cr *model.CryostatInstance, namespace string) *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name,
			Namespace: namespace,
		},
	}
}

func (r *Reconciler) reconcileRoleBinding(ctx context.Context, cr *model.CryostatInstance) error {
	sa := newServiceAccount(cr)
	subjects := []rbacv1.Subject{
		{
			Kind:      rbacv1.ServiceAccountKind,
			Name:      sa.Name,
			Namespace: sa.Namespace,
		},
	}

	// Create a RoleBinding in each target namespace
	for _, ns := range cr.TargetNamespaces {
		binding := newRoleBinding(cr, ns)
		roleRef := &rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "cryostat-operator-cryostat-namespaced",
		}
		err := r.createOrUpdateRoleBinding(ctx, binding, cr.Object, subjects, roleRef)
		if err != nil {
			return err
		}
	}
	// Delete any RoleBindings in target namespaces that are no longer requested
	for _, ns := range toDelete(cr) {
		binding := newRoleBinding(cr, ns)
		err := r.deleteRoleBinding(ctx, binding)
		if err != nil {
			return err
		}
	}

	return nil
}

func newClusterRoleBinding(cr *model.CryostatInstance) *rbacv1.ClusterRoleBinding {
	return &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: common.ClusterUniqueName(cr.Object.GetObjectKind().GroupVersionKind().Kind,
				cr.Name, cr.InstallNamespace),
		},
	}
}

const clusterRoleName = "cryostat-operator-cryostat"

func (r *Reconciler) reconcileClusterRoleBinding(ctx context.Context, cr *model.CryostatInstance) error {
	binding := newClusterRoleBinding(cr)

	sa := newServiceAccount(cr)
	subjects := []rbacv1.Subject{
		{
			Kind:      rbacv1.ServiceAccountKind,
			Name:      sa.Name,
			Namespace: sa.Namespace,
		},
	}

	roleRef := &rbacv1.RoleRef{
		APIGroup: "rbac.authorization.k8s.io",
		Kind:     "ClusterRole",
		Name:     clusterRoleName,
	}

	return r.createOrUpdateClusterRoleBinding(ctx, binding, cr.Object, subjects, roleRef)
}

func (r *Reconciler) createOrUpdateServiceAccount(ctx context.Context, sa *corev1.ServiceAccount,
	owner metav1.Object, labels map[string]string, annotations map[string]string) error {
	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, sa, func() error {
		// TODO just replace the labels and annotations we manage, once we allow the user to configure
		// ServiceAccount annotations/labels in the CR, we can simply overwrite them all

		// Update labels and annotations managed by the operator
		for key, val := range labels {
			metav1.SetMetaDataLabel(&sa.ObjectMeta, key, val)
		}
		for key, val := range annotations {
			metav1.SetMetaDataAnnotation(&sa.ObjectMeta, key, val)
		}

		// Set the Cryostat CR as controller
		if err := controllerutil.SetControllerReference(owner, sa, r.Scheme); err != nil {
			return err
		}
		// AutomountServiceAccountToken specified in Pod, which takes precedence
		// Secrets, ImagePullSecrets are modified by Kubernetes/OpenShift
		return nil
	})
	if err != nil {
		return err
	}
	r.Log.Info(fmt.Sprintf("Service Account %s", op), "name", sa.Name, "namespace", sa.Namespace)
	return nil
}

func (r *Reconciler) cleanUpRole(ctx context.Context, cr *model.CryostatInstance, role *rbacv1.Role) error {
	err := r.Client.Get(ctx, types.NamespacedName{Name: role.Name, Namespace: role.Namespace}, role)
	if err != nil && !kerrors.IsNotFound(err) {
		r.Log.Error(err, "Could not look up role", "name", role.Name, "namespace", role.Namespace)
		return err
	} else if metav1.IsControlledBy(role, cr.Object) {
		err := r.Client.Delete(ctx, role)
		if err != nil {
			r.Log.Info("Failed to delete role", "name", role.Name, "namespace", role.Namespace)
		}
		r.Log.Info("Role deleted", "name", role.Name, "namespace", role.Namespace)
	}
	return nil
}

func (r *Reconciler) createOrUpdateRoleBinding(ctx context.Context, binding *rbacv1.RoleBinding,
	owner metav1.Object, subjects []rbacv1.Subject, roleRef *rbacv1.RoleRef) error {
	bindingCopy := binding.DeepCopy()
	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, binding, func() error {
		// Update the list of Subjects
		binding.Subjects = subjects
		// Update the Role reference
		roleRef, err := getRoleRef(binding, &binding.RoleRef, roleRef)
		if err != nil {
			return err
		}
		binding.RoleRef = *roleRef

		// Set the Cryostat CR as controller
		if err := controllerutil.SetControllerReference(owner, binding, r.Scheme); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		if err == errRoleRefModified {
			return r.recreateRoleBinding(ctx, bindingCopy, owner, subjects, roleRef)
		}
		return err
	}
	r.Log.Info(fmt.Sprintf("Role Binding %s", op), "name", binding.Name, "namespace", binding.Namespace)
	return nil
}

var errRoleRefModified error = errors.New("role binding roleRef has been modified")

func getRoleRef(binding metav1.Object, oldRef *rbacv1.RoleRef, newRef *rbacv1.RoleRef) (*rbacv1.RoleRef, error) {
	// The RoleRef field is immutable. In order to update this field, we need to
	// delete and re-create the role binding.
	// See: https://kubernetes.io/docs/reference/access-authn-authz/rbac/#clusterrolebinding-example
	creationTimestamp := binding.GetCreationTimestamp()
	if creationTimestamp.IsZero() {
		return newRef, nil
	} else if !cmp.Equal(oldRef, newRef) {
		// Return error so role binding can be recreated
		return nil, errRoleRefModified
	}
	return oldRef, nil
}

func (r *Reconciler) recreateRoleBinding(ctx context.Context, binding *rbacv1.RoleBinding, owner metav1.Object,
	subjects []rbacv1.Subject, roleRef *rbacv1.RoleRef) error {
	// Delete and recreate role binding
	err := r.deleteRoleBinding(ctx, binding)
	if err != nil {
		return err
	}
	return r.createOrUpdateRoleBinding(ctx, binding, owner, subjects, roleRef)
}

func (r *Reconciler) deleteRoleBinding(ctx context.Context, binding *rbacv1.RoleBinding) error {
	err := r.Client.Delete(ctx, binding)
	if err != nil {
		if kerrors.IsNotFound(err) {
			r.Log.Info("No role binding to delete", "name", binding.Name, "namespace", binding.Namespace)
			return nil
		}
		r.Log.Error(err, "Could not delete role binding", "name", binding.Name, "namespace", binding.Namespace)
		return err
	}
	r.Log.Info("Role Binding deleted", "name", binding.Name, "namespace", binding.Namespace)
	return nil
}

func (r *Reconciler) createOrUpdateClusterRoleBinding(ctx context.Context, binding *rbacv1.ClusterRoleBinding,
	owner metav1.Object, subjects []rbacv1.Subject, roleRef *rbacv1.RoleRef) error {
	bindingCopy := binding.DeepCopy()
	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, binding, func() error {
		// Update the list of Subjects
		binding.Subjects = subjects
		// Update the Role reference
		roleRef, err := getRoleRef(binding, &binding.RoleRef, roleRef)
		if err != nil {
			return err
		}
		binding.RoleRef = *roleRef

		// ClusterRoleBinding can't be owned by namespaced CR, clean up using finalizer
		return nil
	})
	if err != nil {
		if err == errRoleRefModified {
			return r.recreateClusterRoleBinding(ctx, bindingCopy, owner, subjects, roleRef)
		}
		return err
	}
	r.Log.Info(fmt.Sprintf("Cluster Role Binding %s", op), "name", binding.Name)
	return nil
}

func (r *Reconciler) recreateClusterRoleBinding(ctx context.Context, binding *rbacv1.ClusterRoleBinding, owner metav1.Object,
	subjects []rbacv1.Subject, roleRef *rbacv1.RoleRef) error {
	// Delete and recreate role binding
	err := r.deleteClusterRoleBinding(ctx, binding)
	if err != nil {
		return err
	}
	return r.createOrUpdateClusterRoleBinding(ctx, binding, owner, subjects, roleRef)
}

func (r *Reconciler) deleteClusterRoleBinding(ctx context.Context, clusterBinding *rbacv1.ClusterRoleBinding) error {
	err := r.Delete(ctx, clusterBinding)
	if err != nil {
		if kerrors.IsNotFound(err) {
			r.Log.Info("ClusterRoleBinding not found, proceeding with deletion", "bindingName", clusterBinding.Name)
			return nil
		}
		r.Log.Error(err, "failed to delete ClusterRoleBinding", "bindingName", clusterBinding.Name)
		return err
	}
	r.Log.Info("deleted ClusterRoleBinding", "bindingName", clusterBinding.Name)
	return nil
}

func toDelete(cr *model.CryostatInstance) []string {
	toDelete := []string{}
	for _, ns := range *cr.TargetNamespaceStatus {
		if !containsNamespace(cr.TargetNamespaces, ns) {
			toDelete = append(toDelete, ns)
		}
	}
	return toDelete
}

func containsNamespace(namespaces []string, namespace string) bool {
	for _, ns := range namespaces {
		if ns == namespace {
			return true
		}
	}
	return false
}

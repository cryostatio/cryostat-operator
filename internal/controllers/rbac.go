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
	"fmt"

	operatorv1beta1 "github.com/cryostatio/cryostat-operator/api/v1beta1"
	"github.com/cryostatio/cryostat-operator/internal/controllers/common"
	oauthv1 "github.com/openshift/api/oauth/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *CryostatReconciler) reconcileRBAC(ctx context.Context, cr *operatorv1beta1.Cryostat) error {
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

func (r *CryostatReconciler) finalizeRBAC(ctx context.Context, cr *operatorv1beta1.Cryostat) error {
	return r.deleteClusterRoleBinding(ctx, cr)
}

func newServiceAccount(cr *operatorv1beta1.Cryostat) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name,
			Namespace: cr.Namespace,
		},
	}
}

func (r *CryostatReconciler) reconcileServiceAccount(ctx context.Context, cr *operatorv1beta1.Cryostat) error {
	sa := newServiceAccount(cr)
	labels := map[string]string{
		"app": "cryostat",
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
	return r.createOrUpdateServiceAccount(ctx, sa, cr, labels, annotations)
}

func newRole(cr *operatorv1beta1.Cryostat) *rbacv1.Role {
	return &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name,
			Namespace: cr.Namespace,
		},
	}
}

func (r *CryostatReconciler) reconcileRole(ctx context.Context, cr *operatorv1beta1.Cryostat) error {
	role := newRole(cr)
	rules := []rbacv1.PolicyRule{
		{
			Verbs:     []string{"get", "list", "watch"},
			APIGroups: []string{""},
			Resources: []string{"endpoints"},
		},
		{
			Verbs:     []string{"get"},
			APIGroups: []string{""},
			Resources: []string{"pods", "replicationcontrollers"},
		},
		{
			Verbs:     []string{"get"},
			APIGroups: []string{"apps"},
			Resources: []string{"replicasets", "deployments", "daemonsets", "statefulsets"},
		},
		{
			Verbs:     []string{"get"},
			APIGroups: []string{"apps.openshift.io"},
			Resources: []string{"deploymentconfigs"},
		},
		{
			Verbs:     []string{"get", "list"},
			APIGroups: []string{"route.openshift.io"},
			Resources: []string{"routes"},
		},
	}
	return r.createOrUpdateRole(ctx, role, cr, rules)
}

func (r *CryostatReconciler) reconcileRoleBinding(ctx context.Context, cr *operatorv1beta1.Cryostat) error {
	binding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name,
			Namespace: cr.Namespace,
		},
	}

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
		Kind:     "Role",
		Name:     newRole(cr).Name,
	}

	return r.createOrUpdateRoleBinding(ctx, binding, cr, subjects, roleRef)
}

func newClusterRoleBinding(cr *operatorv1beta1.Cryostat) *rbacv1.ClusterRoleBinding {
	return &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: common.ClusterUniqueName(cr),
		},
	}
}

const clusterRoleName = "cryostat-operator-cryostat"

func (r *CryostatReconciler) reconcileClusterRoleBinding(ctx context.Context, cr *operatorv1beta1.Cryostat) error {
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

	return r.createOrUpdateClusterRoleBinding(ctx, binding, cr, subjects, roleRef)
}

func (r *CryostatReconciler) createOrUpdateServiceAccount(ctx context.Context, sa *corev1.ServiceAccount,
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

func (r *CryostatReconciler) createOrUpdateRole(ctx context.Context, role *rbacv1.Role,
	owner metav1.Object, rules []rbacv1.PolicyRule) error {
	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, role, func() error {
		// Update the list of PolicyRules
		role.Rules = rules

		// Set the Cryostat CR as controller
		if err := controllerutil.SetControllerReference(owner, role, r.Scheme); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}
	r.Log.Info(fmt.Sprintf("Role %s", op), "name", role.Name, "namespace", role.Namespace)
	return nil
}

func (r *CryostatReconciler) createOrUpdateRoleBinding(ctx context.Context, binding *rbacv1.RoleBinding,
	owner metav1.Object, subjects []rbacv1.Subject, roleRef *rbacv1.RoleRef) error {
	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, binding, func() error {
		// Update the list of Subjects
		binding.Subjects = subjects
		// Update the Role reference
		binding.RoleRef = *roleRef

		// Set the Cryostat CR as controller
		if err := controllerutil.SetControllerReference(owner, binding, r.Scheme); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}
	r.Log.Info(fmt.Sprintf("Role Binding %s", op), "name", binding.Name, "namespace", binding.Namespace)
	return nil
}

func (r *CryostatReconciler) createOrUpdateClusterRoleBinding(ctx context.Context, binding *rbacv1.ClusterRoleBinding,
	owner metav1.Object, subjects []rbacv1.Subject, roleRef *rbacv1.RoleRef) error {
	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, binding, func() error {
		// Update the list of Subjects
		binding.Subjects = subjects
		// Update the Role reference
		binding.RoleRef = *roleRef

		// ClusterRoleBinding can't be owned by namespaced CR, clean up using finalizer
		return nil
	})
	if err != nil {
		return err
	}
	r.Log.Info(fmt.Sprintf("Cluster Role Binding %s", op), "name", binding.Name)
	return nil
}

func (r *CryostatReconciler) deleteClusterRoleBinding(ctx context.Context, cr *operatorv1beta1.Cryostat) error {
	clusterBinding := newClusterRoleBinding(cr)
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

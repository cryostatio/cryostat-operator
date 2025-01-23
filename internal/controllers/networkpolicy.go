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

	resources "github.com/cryostatio/cryostat-operator/internal/controllers/common/resource_definitions"
	"github.com/cryostatio/cryostat-operator/internal/controllers/constants"
	"github.com/cryostatio/cryostat-operator/internal/controllers/model"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var AllNamespacesSelector = networkingv1.NetworkPolicyPeer{
	NamespaceSelector: &metav1.LabelSelector{},
}

var RouteSelector = networkingv1.NetworkPolicyPeer{
	NamespaceSelector: &metav1.LabelSelector{
		MatchLabels: map[string]string{
			"policy-group.network.openshift.io/ingress": "",
		},
	},
}

func installationNamespaceSelector(cr *model.CryostatInstance) *metav1.LabelSelector {
	return namespaceOriginSelector(cr.InstallNamespace)
}

func namespaceOriginSelector(namespace string) *metav1.LabelSelector {
	return &metav1.LabelSelector{
		MatchLabels: map[string]string{
			"kubernetes.io/metadata.name": namespace,
		},
	}
}

func (r *Reconciler) reconcileCoreNetworkPolicy(ctx context.Context, cr *model.CryostatInstance) error {
	networkPolicy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-internal-ingress", cr.Name),
			Namespace: cr.InstallNamespace,
		},
	}

	if cr.Spec.NetworkPolicies != nil && cr.Spec.NetworkPolicies.CoreConfig != nil && cr.Spec.NetworkPolicies.CoreConfig.Disabled != nil && *cr.Spec.NetworkPolicies.CoreConfig.Disabled {
		return r.deletePolicy(ctx, networkPolicy)
	}

	return r.createOrUpdatePolicy(ctx, networkPolicy, cr.Object, func() error {
		networkPolicy.Spec = networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: resources.CorePodLabels(cr),
			},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				// allow ingress to the authproxy/cryostat HTTP(S) port from any namespace or from the Route
				{
					From: []networkingv1.NetworkPolicyPeer{
						AllNamespacesSelector,
						RouteSelector,
					},
					Ports: []networkingv1.NetworkPolicyPort{
						{
							Port: &intstr.IntOrString{IntVal: constants.AuthProxyHttpContainerPort},
						},
					},
				},
				// allow ingress to the agent gateway from the target namespaces
				{
					From: []networkingv1.NetworkPolicyPeer{
						{
							NamespaceSelector: &metav1.LabelSelector{
								MatchExpressions: []metav1.LabelSelectorRequirement{
									{
										Key:      "kubernetes.io/metadata.name",
										Operator: metav1.LabelSelectorOpIn,
										Values:   cr.Spec.TargetNamespaces,
									},
								},
							},
						},
					},
					Ports: []networkingv1.NetworkPolicyPort{
						networkingv1.NetworkPolicyPort{
							Port: &intstr.IntOrString{IntVal: constants.AgentProxyContainerPort},
						},
					},
				},
			},
		}
		return nil
	})
}

func (r *Reconciler) reconcileDatabaseNetworkPolicy(ctx context.Context, cr *model.CryostatInstance) error {
	networkPolicy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-db-internal-ingress", cr.Name),
			Namespace: cr.InstallNamespace,
		},
	}
	if cr.Spec.NetworkPolicies != nil && cr.Spec.NetworkPolicies.DatabaseConfig != nil && cr.Spec.NetworkPolicies.DatabaseConfig.Disabled != nil && *cr.Spec.NetworkPolicies.DatabaseConfig.Disabled {
		return r.deletePolicy(ctx, networkPolicy)
	}

	return r.createOrUpdatePolicy(ctx, networkPolicy, cr.Object, func() error {
		networkPolicy.Spec = networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: resources.DatabasePodLabels(cr),
			},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{
					From: []networkingv1.NetworkPolicyPeer{
						{
							NamespaceSelector: installationNamespaceSelector(cr),
							PodSelector: &metav1.LabelSelector{
								MatchLabels: resources.CorePodLabels(cr),
							},
						},
					},
					Ports: []networkingv1.NetworkPolicyPort{
						{
							Port: &intstr.IntOrString{IntVal: constants.DatabasePort},
						},
					},
				},
			},
		}
		return nil
	})
}

func (r *Reconciler) reconcileStorageNetworkPolicy(ctx context.Context, cr *model.CryostatInstance, serviceSpecs *resources.ServiceSpecs) error {
	networkPolicy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-storage-internal-ingress", cr.Name),
			Namespace: cr.InstallNamespace,
		},
	}

	if cr.Spec.NetworkPolicies != nil && cr.Spec.NetworkPolicies.StorageConfig != nil && cr.Spec.NetworkPolicies.StorageConfig.Disabled != nil && *cr.Spec.NetworkPolicies.StorageConfig.Disabled {
		return r.deletePolicy(ctx, networkPolicy)
	}

	return r.createOrUpdatePolicy(ctx, networkPolicy, cr.Object, func() error {
		networkPolicy.Spec = networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: resources.StoragePodLabels(cr),
			},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{
					From: []networkingv1.NetworkPolicyPeer{
						{
							NamespaceSelector: installationNamespaceSelector(cr),
							PodSelector: &metav1.LabelSelector{
								MatchLabels: resources.CorePodLabels(cr),
							},
						},
					},
					Ports: []networkingv1.NetworkPolicyPort{
						{
							Port: &intstr.IntOrString{IntVal: constants.StoragePort},
						},
					},
				},
			},
		}
		return nil
	})
}

func (r *Reconciler) reconcileReportsNetworkPolicy(ctx context.Context, cr *model.CryostatInstance) error {
	networkPolicy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-reports-internal-ingress", cr.Name),
			Namespace: cr.InstallNamespace,
		},
	}

	if cr.Spec.NetworkPolicies != nil && cr.Spec.NetworkPolicies.ReportsConfig != nil && cr.Spec.NetworkPolicies.ReportsConfig.Disabled != nil && *cr.Spec.NetworkPolicies.ReportsConfig.Disabled {
		return r.deletePolicy(ctx, networkPolicy)
	}

	return r.createOrUpdatePolicy(ctx, networkPolicy, cr.Object, func() error {
		networkPolicy.Spec = networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: resources.ReportsPodLabels(cr),
			},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{
					From: []networkingv1.NetworkPolicyPeer{
						{
							NamespaceSelector: installationNamespaceSelector(cr),
							PodSelector: &metav1.LabelSelector{
								MatchLabels: resources.CorePodLabels(cr),
							},
						},
					},
					Ports: []networkingv1.NetworkPolicyPort{
						{
							Port: &intstr.IntOrString{IntVal: constants.ReportsContainerPort},
						},
					},
				},
			},
		}
		return nil
	})
}

func (r *Reconciler) createOrUpdatePolicy(ctx context.Context, networkPolicy *networkingv1.NetworkPolicy, owner metav1.Object,
	delegate controllerutil.MutateFn) error {
	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, networkPolicy, func() error {
		// Set the Cryostat CR as controller
		if owner != nil {
			if err := controllerutil.SetControllerReference(owner, networkPolicy, r.Scheme); err != nil {
				return err
			}
		}
		// Call the delegate for specific mutations
		return delegate()
	})
	if err != nil {
		return err
	}
	r.Log.Info(fmt.Sprintf("Network policy %s", op), "name", networkPolicy.Name, "namespace", networkPolicy.Namespace)
	return nil
}

func (r *Reconciler) deletePolicy(ctx context.Context, networkPolicy *networkingv1.NetworkPolicy) error {
	err := r.Client.Delete(ctx, networkPolicy)
	if err != nil && !errors.IsNotFound(err) {
		r.Log.Error(err, "Could not delete network policy", "name", networkPolicy.Name, "namespace", networkPolicy.Namespace)
		return err
	}
	r.Log.Info("Network policy deleted", "name", networkPolicy.Name, "namespace", networkPolicy.Namespace)
	return nil
}

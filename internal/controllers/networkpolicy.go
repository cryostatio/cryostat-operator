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
	"slices"

	resources "github.com/cryostatio/cryostat-operator/internal/controllers/common/resource_definitions"
	"github.com/cryostatio/cryostat-operator/internal/controllers/constants"
	"github.com/cryostatio/cryostat-operator/internal/controllers/model"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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

const NAMESPACE_NAME_LABEL = "kubernetes.io/metadata.name"

func namespaceOriginSelector(namespace string) *metav1.LabelSelector {
	return &metav1.LabelSelector{
		MatchLabels: map[string]string{
			NAMESPACE_NAME_LABEL: namespace,
		},
	}
}

func (r *Reconciler) reconcileCoreNetworkPolicy(ctx context.Context, cr *model.CryostatInstance) error {
	var err error

	ingressPolicy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-internal-ingress", cr.Name),
			Namespace: cr.InstallNamespace,
		},
	}
	ingressDisabled := cr.Spec.NetworkPolicies != nil && cr.Spec.NetworkPolicies.CoreConfig != nil && cr.Spec.NetworkPolicies.CoreConfig.IngressDisabled != nil && *cr.Spec.NetworkPolicies.CoreConfig.IngressDisabled
	if ingressDisabled {
		err = r.deletePolicy(ctx, ingressPolicy)
	}
	if err != nil {
		return err
	}

	egressPolicy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-internal-egress", cr.Name),
			Namespace: cr.InstallNamespace,
		},
	}
	egressEnabled := cr.Spec.NetworkPolicies != nil && cr.Spec.NetworkPolicies.CoreConfig != nil && cr.Spec.NetworkPolicies.CoreConfig.EgressEnabled != nil && *cr.Spec.NetworkPolicies.CoreConfig.EgressEnabled
	if !egressEnabled {
		err = r.deletePolicy(ctx, egressPolicy)
	}
	if err != nil {
		return err
	}

	if !ingressDisabled {
		err = r.createOrUpdatePolicy(ctx, ingressPolicy, cr.Object, func() error {
			ingressPolicy.Spec = networkingv1.NetworkPolicySpec{
				PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
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
											Key:      NAMESPACE_NAME_LABEL,
											Operator: metav1.LabelSelectorOpIn,
											Values:   cr.Spec.TargetNamespaces,
										},
									},
								},
							},
						},
						Ports: []networkingv1.NetworkPolicyPort{
							{
								Port: &intstr.IntOrString{IntVal: constants.AgentProxyContainerPort},
							},
						},
					},
				},
			}
			return nil
		})
	}
	if err != nil {
		return err
	}

	if egressEnabled {
		egressDestinations := []networkingv1.NetworkPolicyPeer{}
		egressNamespaces := []string{
			// allow outgoing connections to Pods in infrastructure namespaces for discovery and auth
			"default",
			"kube-system",
			// allow outgoing connections to Pods in the InstallNamespace, so Cryostat can connect to its database, storage, etc.
			cr.InstallNamespace,
		}
		if r.IsOpenShift {
			egressNamespaces = append(egressNamespaces, "openshift")
		}
		for _, ns := range cr.TargetNamespaces {
			// allow outgoing connections to any Pod in the TargetNamespaces
			egressNamespaces = append(egressNamespaces, ns)
		}
		slices.Sort(egressNamespaces)
		egressDestinations = append(egressDestinations, networkingv1.NetworkPolicyPeer{
			NamespaceSelector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      NAMESPACE_NAME_LABEL,
						Operator: metav1.LabelSelectorOpIn,
						Values:   slices.Compact(egressNamespaces),
					},
				},
			},
		})
		k8sApiEndpoint := corev1.Endpoints{}
		err = r.Client.Get(ctx, types.NamespacedName{Namespace: "default", Name: "kubernetes"}, &k8sApiEndpoint)
		if err != nil {
			return err
		}
		if len(k8sApiEndpoint.Subsets) > 0 && len(k8sApiEndpoint.Subsets[0].Addresses) > 0 {
			// allow outgoing connections to the Kubernetes API server
			egressDestinations = append(egressDestinations,
				networkingv1.NetworkPolicyPeer{
					IPBlock: &networkingv1.IPBlock{
						CIDR: fmt.Sprintf("%s/32", k8sApiEndpoint.Subsets[0].Addresses[0].IP),
					},
				},
			)
		} else {
			return fmt.Errorf("Endpoints 'kubernetes' had no .Subsets or subset .Addresses")
		}
		err = r.createOrUpdatePolicy(ctx, egressPolicy, cr.Object, func() error {
			egressPolicy.Spec = networkingv1.NetworkPolicySpec{
				PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
				PodSelector: metav1.LabelSelector{
					MatchLabels: resources.CorePodLabels(cr),
				},
				Egress: []networkingv1.NetworkPolicyEgressRule{
					{
						To: egressDestinations,
					},
				},
			}
			return nil
		})
	}
	return err
}

func (r *Reconciler) reconcileDatabaseNetworkPolicy(ctx context.Context, cr *model.CryostatInstance) error {
	ingressPolicy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-db-internal-ingress", cr.Name),
			Namespace: cr.InstallNamespace,
		},
	}
	if cr.Spec.NetworkPolicies != nil && cr.Spec.NetworkPolicies.DatabaseConfig != nil && cr.Spec.NetworkPolicies.DatabaseConfig.IngressDisabled != nil && *cr.Spec.NetworkPolicies.DatabaseConfig.IngressDisabled {
		return r.deletePolicy(ctx, ingressPolicy)
	}

	return r.createOrUpdatePolicy(ctx, ingressPolicy, cr.Object, func() error {
		ingressPolicy.Spec = networkingv1.NetworkPolicySpec{
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
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

func (r *Reconciler) reconcileStorageNetworkPolicy(ctx context.Context, cr *model.CryostatInstance) error {
	ingressPolicy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-storage-internal-ingress", cr.Name),
			Namespace: cr.InstallNamespace,
		},
	}
	if cr.Spec.NetworkPolicies != nil && cr.Spec.NetworkPolicies.StorageConfig != nil && cr.Spec.NetworkPolicies.StorageConfig.IngressDisabled != nil && *cr.Spec.NetworkPolicies.StorageConfig.IngressDisabled {
		return r.deletePolicy(ctx, ingressPolicy)
	}

	return r.createOrUpdatePolicy(ctx, ingressPolicy, cr.Object, func() error {
		ingressPolicy.Spec = networkingv1.NetworkPolicySpec{
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
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
	ingressPolicy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-reports-internal-ingress", cr.Name),
			Namespace: cr.InstallNamespace,
		},
	}
	if cr.Spec.NetworkPolicies != nil && cr.Spec.NetworkPolicies.ReportsConfig != nil && cr.Spec.NetworkPolicies.ReportsConfig.IngressDisabled != nil && *cr.Spec.NetworkPolicies.ReportsConfig.IngressDisabled {
		return r.deletePolicy(ctx, ingressPolicy)
	}

	return r.createOrUpdatePolicy(ctx, ingressPolicy, cr.Object, func() error {
		ingressPolicy.Spec = networkingv1.NetworkPolicySpec{
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
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

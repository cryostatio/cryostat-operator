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
	"regexp"

	"github.com/cryostatio/cryostat-operator/internal/controllers/common"
	"github.com/cryostatio/cryostat-operator/internal/controllers/model"
	"github.com/go-logr/logr"
	configv1 "github.com/openshift/api/config/v1"
	consolev1 "github.com/openshift/api/console/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *Reconciler) reconcileOpenShift(ctx context.Context, cr *model.CryostatInstance) error {
	if !r.IsOpenShift {
		return nil
	}
	err := r.reconcileConsoleLink(ctx, cr)
	if err != nil {
		return err
	}
	// Remove CORS modifications from previous operator versions
	return r.deleteCorsAllowedOrigins(ctx, cr)
}

func (r *Reconciler) finalizeOpenShift(ctx context.Context, cr *model.CryostatInstance) error {
	if !r.IsOpenShift {
		return nil
	}
	reqLogger := r.Log.WithValues("Request.Namespace", cr.InstallNamespace, "Request.Name", cr.Name)
	err := r.deleteConsoleLink(ctx, r.newConsoleLink(cr), reqLogger)
	if err != nil {
		return err
	}
	return r.deleteCorsAllowedOrigins(ctx, cr)
}

func (r *Reconciler) newConsoleLink(cr *model.CryostatInstance) *consolev1.ConsoleLink {
	// Cluster scoped, so use a unique name to avoid conflicts
	return &consolev1.ConsoleLink{
		ObjectMeta: metav1.ObjectMeta{
			Name: common.ClusterUniqueName(r.gvk, cr.Name, cr.InstallNamespace),
		},
	}
}

func (r *Reconciler) reconcileConsoleLink(ctx context.Context, cr *model.CryostatInstance) error {
	reqLogger := r.Log.WithValues("Request.Namespace", cr.InstallNamespace, "Request.Name", cr.Name)
	link := r.newConsoleLink(cr)

	url := cr.Status.ApplicationURL
	if len(url) == 0 {
		// Nothing to link to, so remove the link if it exists
		err := r.deleteConsoleLink(ctx, link, reqLogger)
		if err != nil {
			return err
		}
		return nil
	}
	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, link, func() error {
		link.Spec.Link = consolev1.Link{
			Text: AppName,
			Href: url,
		}
		link.Spec.Location = consolev1.NamespaceDashboard
		link.Spec.NamespaceDashboard = &consolev1.NamespaceDashboardSpec{
			Namespaces: []string{cr.InstallNamespace},
		}
		return nil
	})
	if err != nil {
		return err
	}
	reqLogger.Info(fmt.Sprintf("Console Link %s", op), "name", link.Name)
	return nil
}

func (r *Reconciler) deleteConsoleLink(ctx context.Context, link *consolev1.ConsoleLink, logger logr.Logger) error {
	err := r.Client.Delete(ctx, link)
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Info("ConsoleLink not found, proceeding with deletion", "name", link.Name)
			return nil
		}
		logger.Error(err, "failed to delete ConsoleLink", "name", link.Name)
		return err
	}
	logger.Info("deleted ConsoleLink", "name", link.Name)
	return nil
}

func (r *Reconciler) deleteCorsAllowedOrigins(ctx context.Context, cr *model.CryostatInstance) error {
	reqLogger := r.Log.WithValues("Request.Namespace", cr.InstallNamespace, "Request.Name", cr.Name)

	allowedOrigin := cr.Status.ApplicationURL
	if len(allowedOrigin) == 0 {
		reqLogger.Info("No Route to remove from APIServer config")
		return nil
	}

	apiServer := &configv1.APIServer{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: apiServerName}, apiServer)
	if err != nil {
		reqLogger.Error(err, "Failed to get APIServer config")
		return err
	}

	allowedOriginAsRegex := regexp.QuoteMeta(allowedOrigin)

	for i, origin := range apiServer.Spec.AdditionalCORSAllowedOrigins {
		if origin == allowedOriginAsRegex {
			apiServer.Spec.AdditionalCORSAllowedOrigins = append(
				apiServer.Spec.AdditionalCORSAllowedOrigins[:i],
				apiServer.Spec.AdditionalCORSAllowedOrigins[i+1:]...)
			err = r.Client.Update(ctx, apiServer)
			if err != nil {
				reqLogger.Error(err, "Failed to remove Cryostat origin from APIServer CORS allowed origins")
				return err
			}

			reqLogger.Info("Removed from APIServer CORS allowed origins")
			return nil
		}
	}
	return nil
}

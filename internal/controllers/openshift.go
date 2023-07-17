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
	err := r.deleteConsoleLink(ctx, newConsoleLink(cr), reqLogger)
	if err != nil {
		return err
	}
	return r.deleteCorsAllowedOrigins(ctx, cr)
}

func newConsoleLink(cr *model.CryostatInstance) *consolev1.ConsoleLink {
	// Cluster scoped, so use a unique name to avoid conflicts
	return &consolev1.ConsoleLink{
		ObjectMeta: metav1.ObjectMeta{
			Name: common.ClusterUniqueName(cr.Object.GetObjectKind().GroupVersionKind().Kind,
				cr.Name, cr.InstallNamespace),
		},
	}
}

func (r *Reconciler) reconcileConsoleLink(ctx context.Context, cr *model.CryostatInstance) error {
	reqLogger := r.Log.WithValues("Request.Namespace", cr.InstallNamespace, "Request.Name", cr.Name)
	link := newConsoleLink(cr)

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

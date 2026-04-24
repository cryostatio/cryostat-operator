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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// ControllerBuilder wraps controller-runtime's builder.Builder
// as an interface to aid testing.
type ControllerBuilder interface {
	For(object client.Object, opts ...builder.ForOption) ControllerBuilder
	Owns(object client.Object, opts ...builder.OwnsOption) ControllerBuilder
	Watches(object client.Object, eventHandler handler.EventHandler, opts ...builder.WatchesOption) ControllerBuilder
	Complete(r reconcile.Reconciler) error
	EnqueueRequestsFromMapFunc(fn handler.MapFunc) handler.EventHandler
	WithPredicates(predicates ...predicate.Predicate) builder.Predicates
}

type ctrlBuilder struct {
	impl *builder.Builder
}

var _ ControllerBuilder = (*ctrlBuilder)(nil)

// NewControllerBuilder returns a new ControllerBuilder for the provided manager.
func NewControllerBuilder(mgr ctrl.Manager) ControllerBuilder {
	return &ctrlBuilder{
		impl: ctrl.NewControllerManagedBy(mgr),
	}
}

// For wraps the [builder.Builder.For] method
func (b *ctrlBuilder) For(object client.Object, opts ...builder.ForOption) ControllerBuilder {
	b.impl = b.impl.For(object, opts...)
	return b
}

// Owns wraps the [builder.Builder.Owns] method
func (b *ctrlBuilder) Owns(object client.Object, opts ...builder.OwnsOption) ControllerBuilder {
	b.impl = b.impl.Owns(object, opts...)
	return b
}

// Watches wraps the [builder.Builder.Watches] method
func (b *ctrlBuilder) Watches(object client.Object, eventHandler handler.EventHandler, opts ...builder.WatchesOption) ControllerBuilder {
	b.impl = b.impl.Watches(object, eventHandler, opts...)
	return b
}

// Complete wraps the [builder.Builder.Complete] method
func (b *ctrlBuilder) Complete(r reconcile.Reconciler) error {
	return b.impl.Complete(r)
}

// EnqueueRequestsFromMapFunc wraps the [handler.EnqueueRequestsFromMapFunc] function
func (b *ctrlBuilder) EnqueueRequestsFromMapFunc(fn handler.MapFunc) handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(fn)
}

// WithPredicates wraps the [builder.WithPredicates] function
func (b *ctrlBuilder) WithPredicates(predicates ...predicate.Predicate) builder.Predicates {
	return builder.WithPredicates(predicates...)
}

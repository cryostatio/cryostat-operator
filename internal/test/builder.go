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

package test

import (
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/cryostatio/cryostat-operator/internal/controllers/common"
)

// TestCtrlBuilder is a fake ControllerBuilder to aid testing of
// controller watches
type TestCtrlBuilder struct {
	ForCalls       []ForArgs
	OwnsCalls      []OwnsArgs
	WatchesCalls   []WatchesArgs
	MapFuncs       []handler.MapFunc
	Predicates     []predicate.Predicate
	CompleteCalled bool
}

// ForArgs contain arguments used for a call to For
type ForArgs struct {
	Object client.Object
	Opts   []builder.ForOption
}

// OwnsArgs contain arguments used for a call to Owns
type OwnsArgs struct {
	Object client.Object
	Opts   []builder.OwnsOption
}

// WatchesArgs contain arguments used for a call to Watches
type WatchesArgs struct {
	Object       client.Object
	EventHandler handler.EventHandler
	Opts         []builder.WatchesOption
}

var _ common.ControllerBuilder = (*TestCtrlBuilder)(nil)

func NewControllerBuilder(config *TestReconcilerConfig) func(ctrl.Manager) common.ControllerBuilder {
	return func(ctrl.Manager) common.ControllerBuilder {
		return config.ControllerBuilder
	}
}

func (b *TestCtrlBuilder) For(object client.Object, opts ...builder.ForOption) common.ControllerBuilder {
	b.ForCalls = append(b.ForCalls, ForArgs{
		Object: object,
		Opts:   opts,
	})
	return b
}

func (b *TestCtrlBuilder) Owns(object client.Object, opts ...builder.OwnsOption) common.ControllerBuilder {
	b.OwnsCalls = append(b.OwnsCalls, OwnsArgs{
		Object: object,
		Opts:   opts,
	})
	return b
}

func (b *TestCtrlBuilder) Watches(object client.Object, eventHandler handler.EventHandler,
	opts ...builder.WatchesOption) common.ControllerBuilder {
	b.WatchesCalls = append(b.WatchesCalls, WatchesArgs{
		Object:       object,
		EventHandler: eventHandler,
		Opts:         opts,
	})
	return b
}

func (b *TestCtrlBuilder) Complete(r reconcile.Reconciler) error {
	b.CompleteCalled = true
	return nil
}

func (b *TestCtrlBuilder) EnqueueRequestsFromMapFunc(fn handler.MapFunc) handler.EventHandler {
	b.MapFuncs = append(b.MapFuncs, fn)
	return handler.EnqueueRequestsFromMapFunc(fn)
}

func (b *TestCtrlBuilder) WithPredicates(predicates ...predicate.Predicate) builder.Predicates {
	b.Predicates = append(b.Predicates, predicates...)
	return builder.WithPredicates(predicates...)
}

package common

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// AddFinalizer adds the provided finalizer to the object and updates it in the cluster
func AddFinalizer(ctx context.Context, client client.Client, obj controllerutil.Object, finalizer string) error {
	log.Info("adding finalizer to object", "namespace", obj.GetNamespace(), "name", obj.GetName())
	controllerutil.AddFinalizer(obj, finalizer)
	err := client.Update(ctx, obj)
	if err != nil {
		log.Error(err, "failed to add finalizer to object", "namespace", obj.GetNamespace(),
			"name", obj.GetName())
		return err
	}
	return nil
}

// RemoveFinalizer removes the provided finalizer from the object and updates it in the cluster
func RemoveFinalizer(ctx context.Context, client client.Client, obj controllerutil.Object, finalizer string) error {
	log.Info("removing finalizer from object", "namespace", obj.GetNamespace(), "name", obj.GetName())
	controllerutil.RemoveFinalizer(obj, finalizer)
	err := client.Update(ctx, obj)
	if err != nil {
		log.Error(err, "failed to remove finalizer from object", "namespace", obj.GetNamespace(),
			"name", obj.GetName())
		return err
	}

	return nil
}

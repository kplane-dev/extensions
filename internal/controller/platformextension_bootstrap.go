package controller

import (
	"context"
	"fmt"

	extensionsv1alpha1 "github.com/kplane-dev/extensions/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	mcmanager "sigs.k8s.io/multicluster-runtime/pkg/manager"
)

// PlatformExtensionBootstrapper watches project ControlPlanes and pushes all
// PlatformExtensions to each VCP when it first becomes engaged.
type PlatformExtensionBootstrapper struct {
	LocalClient client.Client
	MCRManager  mcmanager.Manager
}

func (r *PlatformExtensionBootstrapper) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	cp := &unstructured.Unstructured{}
	cp.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "controlplane.kplane.dev",
		Version: "v1alpha1",
		Kind:    "ControlPlane",
	})
	if err := r.LocalClient.Get(ctx, req.NamespacedName, cp); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if cp.GetLabels()["platform.kplane.dev/type"] != "project" {
		return ctrl.Result{}, nil
	}

	clusterName := cp.GetNamespace()
	cl, err := r.MCRManager.GetCluster(ctx, clusterName)
	if err != nil {
		// Not engaged yet; ProjectVCPProvider will reconcile the CP again once it is.
		return ctrl.Result{}, nil
	}

	var list extensionsv1alpha1.PlatformExtensionList
	if err := r.LocalClient.List(ctx, &list); err != nil {
		return ctrl.Result{}, fmt.Errorf("list PlatformExtensions: %w", err)
	}

	for i := range list.Items {
		if err := upsertPlatformExtension(ctx, cl.GetClient(), &list.Items[i]); err != nil {
			log.Error(err, "failed to bootstrap PlatformExtension", "cluster", clusterName, "name", list.Items[i].Name)
		}
	}
	return ctrl.Result{}, nil
}

func (r *PlatformExtensionBootstrapper) SetupWithManager(mgr ctrl.Manager) error {
	r.LocalClient = mgr.GetClient()
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "controlplane.kplane.dev",
		Version: "v1alpha1",
		Kind:    "ControlPlane",
	})
	return ctrl.NewControllerManagedBy(mgr).
		For(obj).
		Named("platformextension-bootstrapper").
		Complete(r)
}

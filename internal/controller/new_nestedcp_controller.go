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

// NewNestedCPController watches GKE ControlPlane resources with a vcp-name label
// (i.e. nested CPs) and touches matching EnabledExtension objects in the project
// VCP when a new CP appears. The annotation update fires the MCR reconciler.
type NewNestedCPController struct {
	LocalClient client.Client
	MCRManager  mcmanager.Manager
}

// +kubebuilder:rbac:groups=controlplane.kplane.dev,resources=controlplanes,verbs=get;list;watch

func (r *NewNestedCPController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
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

	labels := cp.GetLabels()
	vcpName := labels["platform.kplane.dev/vcp-name"]
	vcpNS := labels["platform.kplane.dev/vcp-namespace"]
	gkeNS := cp.GetNamespace()
	if vcpName == "" || vcpNS == "" {
		return ctrl.Result{}, nil
	}

	// gkeNS is the MCR cluster name for this project VCP.
	remoteCluster, err := r.MCRManager.GetCluster(ctx, gkeNS)
	if err != nil {
		// VCP not yet engaged — will be retried when it is.
		return ctrl.Result{}, nil
	}

	var eeList extensionsv1alpha1.EnabledExtensionList
	if err := remoteCluster.GetClient().List(ctx, &eeList, client.InNamespace(vcpNS)); err != nil {
		return ctrl.Result{}, fmt.Errorf("list EnabledExtensions in %s/%s: %w", gkeNS, vcpNS, err)
	}

	for i := range eeList.Items {
		ee := &eeList.Items[i]
		if !CPMatches(ee.Spec.ControlPlanes, vcpName) {
			continue
		}
		// Touch the object so the MCR watch fires.
		ann := ee.Annotations
		if ann == nil {
			ann = map[string]string{}
		}
		ann["extensions.kplane.dev/last-cp-trigger"] = cp.GetResourceVersion()
		ee.Annotations = ann
		if err := remoteCluster.GetClient().Update(ctx, ee); err != nil && !apierrors.IsConflict(err) {
			return ctrl.Result{}, fmt.Errorf("touch EnabledExtension %s: %w", ee.Name, err)
		}
	}
	return ctrl.Result{}, nil
}

func (r *NewNestedCPController) SetupWithManager(mgr ctrl.Manager) error {
	r.LocalClient = mgr.GetClient()
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "controlplane.kplane.dev",
		Version: "v1alpha1",
		Kind:    "ControlPlane",
	})
	return ctrl.NewControllerManagedBy(mgr).
		For(obj).
		Named("new-nestedcp-trigger").
		Complete(r)
}

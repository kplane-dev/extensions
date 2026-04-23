package controller

import (
	"context"
	"fmt"

	extensionsv1alpha1 "github.com/kplane-dev/extensions/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	mcmanager "sigs.k8s.io/multicluster-runtime/pkg/manager"
)

// PlatformExtensionReplicator watches PlatformExtension objects in GKE and
// replicates them into every project VCP.
type PlatformExtensionReplicator struct {
	// LocalClient is the GKE management cluster client.
	LocalClient client.Client
	// MCRManager is used to get per-VCP clients for replication.
	MCRManager mcmanager.Manager
}

// +kubebuilder:rbac:groups=extensions.kplane.dev,resources=platformextensions,verbs=get;list;watch
// +kubebuilder:rbac:groups=controlplane.kplane.dev,resources=controlplanes,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

func (r *PlatformExtensionReplicator) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx).WithValues("platformextension", req.Name)

	var pe extensionsv1alpha1.PlatformExtension
	if err := r.LocalClient.Get(ctx, req.NamespacedName, &pe); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, r.deleteFromAllVCPs(ctx, req.Name)
		}
		return ctrl.Result{}, err
	}

	namespaces, err := r.projectVCPNamespaces(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}

	for _, ns := range namespaces {
		cl, err := r.MCRManager.GetCluster(ctx, ns)
		if err != nil {
			log.Error(err, "VCP not yet engaged", "gkeNamespace", ns)
			continue
		}
		if err := upsertPlatformExtension(ctx, cl.GetClient(), &pe); err != nil {
			log.Error(err, "failed to replicate", "gkeNamespace", ns)
		}
	}
	return ctrl.Result{}, nil
}

func (r *PlatformExtensionReplicator) deleteFromAllVCPs(ctx context.Context, name string) error {
	log := ctrl.LoggerFrom(ctx)
	namespaces, err := r.projectVCPNamespaces(ctx)
	if err != nil {
		return err
	}
	for _, ns := range namespaces {
		cl, err := r.MCRManager.GetCluster(ctx, ns)
		if err != nil {
			continue
		}
		existing := &extensionsv1alpha1.PlatformExtension{}
		if err := cl.GetClient().Get(ctx, types.NamespacedName{Name: name}, existing); err != nil {
			if !apierrors.IsNotFound(err) {
				log.Error(err, "failed to get for deletion", "gkeNamespace", ns)
			}
			continue
		}
		if err := cl.GetClient().Delete(ctx, existing); err != nil && !apierrors.IsNotFound(err) {
			log.Error(err, "failed to delete", "gkeNamespace", ns)
		}
	}
	return nil
}

func upsertPlatformExtension(ctx context.Context, vcpClient client.Client, pe *extensionsv1alpha1.PlatformExtension) error {
	desired := &extensionsv1alpha1.PlatformExtension{
		ObjectMeta: metav1.ObjectMeta{
			Name:        pe.Name,
			Annotations: map[string]string{"extensions.kplane.dev/managed-by": "platform"},
		},
		Spec: pe.Spec,
	}

	existing := &extensionsv1alpha1.PlatformExtension{}
	err := vcpClient.Get(ctx, types.NamespacedName{Name: pe.Name}, existing)
	if apierrors.IsNotFound(err) {
		return vcpClient.Create(ctx, desired)
	}
	if err != nil {
		return err
	}
	existing.Spec = desired.Spec
	if existing.Annotations == nil {
		existing.Annotations = map[string]string{}
	}
	existing.Annotations["extensions.kplane.dev/managed-by"] = "platform"
	return vcpClient.Update(ctx, existing)
}

func (r *PlatformExtensionReplicator) projectVCPNamespaces(ctx context.Context) ([]string, error) {
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "controlplane.kplane.dev",
		Version: "v1alpha1",
		Kind:    "ControlPlaneList",
	})
	if err := r.LocalClient.List(ctx, list,
		client.MatchingLabels{"platform.kplane.dev/type": "project"},
	); err != nil {
		return nil, fmt.Errorf("list project ControlPlanes: %w", err)
	}

	seen := map[string]struct{}{}
	var namespaces []string
	for _, cp := range list.Items {
		ns := cp.GetNamespace()
		if _, ok := seen[ns]; !ok {
			seen[ns] = struct{}{}
			namespaces = append(namespaces, ns)
		}
	}
	return namespaces, nil
}

func (r *PlatformExtensionReplicator) SetupWithManager(mgr ctrl.Manager) error {
	r.LocalClient = mgr.GetClient()
	return ctrl.NewControllerManagedBy(mgr).
		For(&extensionsv1alpha1.PlatformExtension{}).
		Named("platformextension-replicator").
		Complete(r)
}

// Package controller implements the extensions.kplane.dev controllers.
package controller

import (
	"context"
	"fmt"
	"io"
	"net/http"

	extensionsv1alpha1 "github.com/kplane-dev/extensions/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	mcmanager "sigs.k8s.io/multicluster-runtime/pkg/manager"
	mcreconcile "sigs.k8s.io/multicluster-runtime/pkg/reconcile"
	"sigs.k8s.io/yaml"
)

// EnabledExtensionReconciler watches EnabledExtension in project VCPs (via MCR)
// and installs the referenced extension's CRDs into the target nested CPs.
type EnabledExtensionReconciler struct {
	// Manager is the MCR manager, used to resolve per-VCP clients.
	Manager mcmanager.Manager
	// LocalClient is the GKE management cluster client, used for nested CP lookups.
	LocalClient client.Client
}

// +kubebuilder:rbac:groups=extensions.kplane.dev,resources=enabledextensions,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=extensions.kplane.dev,resources=enabledextensions/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=extensions.kplane.dev,resources=platformextensions,verbs=get;list;watch
// +kubebuilder:rbac:groups=controlplane.kplane.dev,resources=controlplanes,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

func (r *EnabledExtensionReconciler) Reconcile(ctx context.Context, req mcreconcile.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx).WithValues("enabledextension", req.NamespacedName, "cluster", req.ClusterName)

	remoteCluster, err := r.Manager.GetCluster(ctx, req.ClusterName)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("get cluster %q: %w", req.ClusterName, err)
	}
	vcpClient := remoteCluster.GetClient()

	var ee extensionsv1alpha1.EnabledExtension
	if err := vcpClient.Get(ctx, req.NamespacedName, &ee); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Resolve CRDs — sets Accepted condition.
	crds, err := r.resolveCRDs(ctx, vcpClient, &ee)
	if err != nil {
		return ctrl.Result{}, r.setAccepted(ctx, vcpClient, &ee, metav1.ConditionFalse, "RefNotFound", err.Error())
	}
	if err := r.setAccepted(ctx, vcpClient, &ee, metav1.ConditionTrue, "Resolved", "extension reference resolved"); err != nil {
		return ctrl.Result{}, err
	}

	if len(crds) == 0 {
		return ctrl.Result{}, r.setProgrammed(ctx, vcpClient, &ee, metav1.ConditionTrue, "NoCRDs", "no CRDs to install")
	}

	// req.ClusterName is the GKE namespace for this project VCP (see provider.go).
	gkeNamespace := req.ClusterName

	targetCPs, err := r.resolveTargetCPs(ctx, gkeNamespace, req.Namespace, ee.Spec.ControlPlanes)
	if err != nil {
		return ctrl.Result{}, r.setProgrammed(ctx, vcpClient, &ee, metav1.ConditionFalse, "LookupError", err.Error())
	}

	if len(targetCPs) == 0 {
		return ctrl.Result{}, r.setProgrammed(ctx, vcpClient, &ee, metav1.ConditionTrue, "NoTargets", "no matching control planes found")
	}

	for cpName, kubeconfigData := range targetCPs {
		if err := r.installCRDs(ctx, kubeconfigData, crds); err != nil {
			log.Error(err, "failed to install CRDs", "controlPlane", cpName)
			return ctrl.Result{}, r.setProgrammed(ctx, vcpClient, &ee, metav1.ConditionFalse, "InstallError",
				fmt.Sprintf("control plane %s: %v", cpName, err))
		}
		log.Info("installed CRDs", "controlPlane", cpName, "count", len(crds))
	}

	return ctrl.Result{}, r.setProgrammed(ctx, vcpClient, &ee, metav1.ConditionTrue, "Programmed",
		fmt.Sprintf("CRDs installed in %d control plane(s)", len(targetCPs)))
}

func (r *EnabledExtensionReconciler) resolveCRDs(
	ctx context.Context, vcpClient client.Client, ee *extensionsv1alpha1.EnabledExtension,
) ([]string, error) {
	ref := ee.Spec.ExtensionRef
	switch ref.Kind {
	case "PlatformExtension":
		var pe extensionsv1alpha1.PlatformExtension
		if err := vcpClient.Get(ctx, types.NamespacedName{Name: ref.Name}, &pe); err != nil {
			return nil, fmt.Errorf("get PlatformExtension %q: %w", ref.Name, err)
		}
		return pe.Spec.CRDs, nil
	case "Extension":
		var ext extensionsv1alpha1.Extension
		if err := vcpClient.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: ee.Namespace}, &ext); err != nil {
			return nil, fmt.Errorf("get Extension %q: %w", ref.Name, err)
		}
		return ext.Spec.CRDs, nil
	default:
		return nil, fmt.Errorf("unknown ExtensionRef kind %q", ref.Kind)
	}
}

// resolveTargetCPs returns a map of CP name → kubeconfig bytes for each nested CP
// in gkeNamespace whose vcp-name label matches the EnabledExtension's controlPlanes list.
func (r *EnabledExtensionReconciler) resolveTargetCPs(
	ctx context.Context, gkeNamespace, vcpNamespace string, controlPlanes []string,
) (map[string][]byte, error) {
	cpList := &unstructured.UnstructuredList{}
	cpList.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "controlplane.kplane.dev",
		Version: "v1alpha1",
		Kind:    "ControlPlaneList",
	})
	if err := r.LocalClient.List(ctx, cpList,
		client.InNamespace(gkeNamespace),
		client.MatchingLabels{"platform.kplane.dev/vcp-namespace": vcpNamespace},
	); err != nil {
		return nil, fmt.Errorf("list nested ControlPlanes: %w", err)
	}

	result := map[string][]byte{}
	for _, cp := range cpList.Items {
		vcpName := cp.GetLabels()["platform.kplane.dev/vcp-name"]
		if vcpName == "" || !CPMatches(controlPlanes, vcpName) {
			continue
		}

		secretName, _, _ := unstructured.NestedString(cp.Object, "status", "kubeconfigSecretRef", "name")
		secretNS, _, _ := unstructured.NestedString(cp.Object, "status", "kubeconfigSecretRef", "namespace")
		if secretName == "" || secretNS == "" {
			continue
		}

		var secret unstructured.Unstructured
		secret.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: "Secret"})
		if err := r.LocalClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: secretNS}, &secret); err != nil {
			return nil, fmt.Errorf("get kubeconfig secret for %s: %w", vcpName, err)
		}
		kubeconfig, _, _ := unstructured.NestedString(secret.Object, "data", "kubeconfig")
		if kubeconfig == "" {
			return nil, fmt.Errorf("kubeconfig missing from secret for %s", vcpName)
		}
		result[vcpName] = []byte(kubeconfig)
	}
	return result, nil
}

func (r *EnabledExtensionReconciler) installCRDs(ctx context.Context, kubeconfigData []byte, crdURLs []string) error {
	restCfg, err := clientcmd.RESTConfigFromKubeConfig(kubeconfigData)
	if err != nil {
		return fmt.Errorf("parse kubeconfig: %w", err)
	}
	cpClient, err := client.New(restCfg, client.Options{})
	if err != nil {
		return fmt.Errorf("create client: %w", err)
	}
	for _, url := range crdURLs {
		manifest, err := fetchURL(ctx, url)
		if err != nil {
			return fmt.Errorf("fetch CRD %s: %w", url, err)
		}
		if err := applyCRD(ctx, cpClient, manifest); err != nil {
			return fmt.Errorf("apply CRD %s: %w", url, err)
		}
	}
	return nil
}

func fetchURL(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func applyCRD(ctx context.Context, c client.Client, manifest []byte) error {
	obj := &unstructured.Unstructured{}
	jsonBytes, err := yaml.YAMLToJSON(manifest)
	if err != nil {
		return fmt.Errorf("yaml to json: %w", err)
	}
	if err := obj.UnmarshalJSON(jsonBytes); err != nil {
		return fmt.Errorf("unmarshal: %w", err)
	}
	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(obj.GroupVersionKind())
	err = c.Get(ctx, types.NamespacedName{Name: obj.GetName()}, existing)
	if apierrors.IsNotFound(err) {
		return c.Create(ctx, obj)
	}
	if err != nil {
		return err
	}
	obj.SetResourceVersion(existing.GetResourceVersion())
	return c.Update(ctx, obj)
}

// setAccepted updates the Accepted condition.
func (r *EnabledExtensionReconciler) setAccepted(
	ctx context.Context, c client.Client, ee *extensionsv1alpha1.EnabledExtension,
	status metav1.ConditionStatus, reason, msg string,
) error {
	meta.SetStatusCondition(&ee.Status.Conditions, metav1.Condition{
		Type:               extensionsv1alpha1.ConditionAccepted,
		Status:             status,
		Reason:             reason,
		Message:            msg,
		ObservedGeneration: ee.Generation,
	})
	return c.Status().Update(ctx, ee)
}

// setProgrammed updates the Programmed condition.
func (r *EnabledExtensionReconciler) setProgrammed(
	ctx context.Context, c client.Client, ee *extensionsv1alpha1.EnabledExtension,
	status metav1.ConditionStatus, reason, msg string,
) error {
	meta.SetStatusCondition(&ee.Status.Conditions, metav1.Condition{
		Type:               extensionsv1alpha1.ConditionProgrammed,
		Status:             status,
		Reason:             reason,
		Message:            msg,
		ObservedGeneration: ee.Generation,
	})
	return c.Status().Update(ctx, ee)
}


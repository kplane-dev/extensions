// Package provider discovers project VCPs and engages them as clusters.
// Copied from controlplane-replicator; watches ControlPlane resources labeled
// platform.kplane.dev/type=project and connects to each VCP's apiserver.
package provider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/log"
	mcmanager "sigs.k8s.io/multicluster-runtime/pkg/manager"
	"sigs.k8s.io/multicluster-runtime/pkg/multicluster"
)

var _ multicluster.Provider = &ProjectVCPProvider{}

var controlPlaneGVK = schema.GroupVersionKind{
	Group:   "controlplane.kplane.dev",
	Version: "v1alpha1",
	Kind:    "ControlPlane",
}

// Options configure the provider.
type Options struct {
	KubeconfigKey           string
	ClusterOptions          []cluster.Option
	RESTOptions             []func(cfg *rest.Config) error
	MaxConcurrentReconciles int
	LabelSelector           map[string]string
}

// +kubebuilder:rbac:groups=controlplane.kplane.dev,resources=controlplanes,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=create;get;list;update;watch

// ProjectVCPProvider discovers project ControlPlanes in the management cluster
// and engages each one as a remote cluster via multicluster-runtime.
type ProjectVCPProvider struct {
	opts Options
	log  logr.Logger

	lock     sync.RWMutex
	clusters map[string]activeCluster
	indexers []index

	mgr mcmanager.Manager
}

type activeCluster struct {
	Cluster cluster.Cluster
	Context context.Context
	Cancel  context.CancelFunc
	Hash    string
}

type index struct {
	object       client.Object
	field        string
	extractValue client.IndexerFunc
}

func New(opts Options) *ProjectVCPProvider {
	if opts.KubeconfigKey == "" {
		opts.KubeconfigKey = "kubeconfig"
	}
	if opts.MaxConcurrentReconciles <= 0 {
		opts.MaxConcurrentReconciles = 16
	}
	return &ProjectVCPProvider{
		opts:     opts,
		log:      log.Log.WithName("project-vcp-provider"),
		clusters: map[string]activeCluster{},
	}
}

func (p *ProjectVCPProvider) SetupWithManager(ctx context.Context, mgr mcmanager.Manager) error {
	if mgr == nil {
		return fmt.Errorf("manager is nil")
	}
	p.mgr = mgr

	localMgr := mgr.GetLocalManager()
	if localMgr == nil {
		return fmt.Errorf("local manager is nil")
	}

	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(controlPlaneGVK)
	return ctrl.NewControllerManagedBy(localMgr).
		For(obj).
		WithOptions(controller.Options{MaxConcurrentReconciles: p.opts.MaxConcurrentReconciles}).
		Complete(p)
}

func (p *ProjectVCPProvider) Get(_ context.Context, clusterName string) (cluster.Cluster, error) {
	p.lock.RLock()
	defer p.lock.RUnlock()
	if ac, ok := p.clusters[clusterName]; ok {
		return ac.Cluster, nil
	}
	return nil, multicluster.ErrClusterNotFound
}

func (p *ProjectVCPProvider) IndexField(
	ctx context.Context, obj client.Object, field string, extractValue client.IndexerFunc,
) error {
	p.lock.Lock()
	p.indexers = append(p.indexers, index{object: obj, field: field, extractValue: extractValue})
	p.lock.Unlock()

	p.lock.RLock()
	defer p.lock.RUnlock()
	for name, ac := range p.clusters {
		if err := ac.Cluster.GetFieldIndexer().IndexField(ctx, obj, field, extractValue); err != nil {
			return fmt.Errorf("failed to index field %q on cluster %q: %w", field, name, err)
		}
	}
	return nil
}

func (p *ProjectVCPProvider) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	cp := &unstructured.Unstructured{}
	cp.SetGroupVersionKind(controlPlaneGVK)
	if err := p.mgr.GetLocalManager().GetClient().Get(ctx, req.NamespacedName, cp); err != nil {
		if apierrors.IsNotFound(err) {
			p.removeCluster(req.Namespace)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if cp.GetDeletionTimestamp() != nil {
		p.removeCluster(req.Namespace)
		return ctrl.Result{}, nil
	}

	if len(p.opts.LabelSelector) > 0 {
		labels := cp.GetLabels()
		for k, v := range p.opts.LabelSelector {
			if labels[k] != v {
				return ctrl.Result{}, nil
			}
		}
	}

	secretName, _, err := unstructured.NestedString(cp.Object, "status", "kubeconfigSecretRef", "name")
	if err != nil {
		return ctrl.Result{}, err
	}
	secretNamespace, _, err := unstructured.NestedString(cp.Object, "status", "kubeconfigSecretRef", "namespace")
	if err != nil {
		return ctrl.Result{}, err
	}
	if secretName == "" || secretNamespace == "" {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	var secret corev1.Secret
	if err := p.mgr.GetLocalManager().GetClient().Get(ctx,
		types.NamespacedName{Name: secretName, Namespace: secretNamespace},
		&secret,
	); err != nil {
		return ctrl.Result{}, err
	}
	kubeconfig, ok := secret.Data[p.opts.KubeconfigKey]
	if !ok || len(kubeconfig) == 0 {
		return ctrl.Result{}, fmt.Errorf("kubeconfig secret %s/%s missing %q", secretNamespace, secretName, p.opts.KubeconfigKey)
	}

	endpoint, _, err := unstructured.NestedString(cp.Object, "status", "endpoint")
	if err != nil {
		return ctrl.Result{}, err
	}

	clusterKey := cp.GetNamespace()
	hash := p.hashKubeconfig(kubeconfig, endpoint)
	if existing, found := p.getCluster(clusterKey); found {
		if existing.Hash == hash {
			return ctrl.Result{}, nil
		}
		p.removeCluster(clusterKey)
	}

	return ctrl.Result{}, p.createAndEngageCluster(ctx, clusterKey, kubeconfig, endpoint, hash)
}

func (p *ProjectVCPProvider) getCluster(name string) (activeCluster, bool) {
	p.lock.RLock()
	defer p.lock.RUnlock()
	ac, ok := p.clusters[name]
	return ac, ok
}

func (p *ProjectVCPProvider) setCluster(name string, ac activeCluster) {
	p.lock.Lock()
	defer p.lock.Unlock()
	p.clusters[name] = ac
}

func (p *ProjectVCPProvider) hashKubeconfig(kubeconfig []byte, endpoint string) string {
	sum := sha256.New()
	sum.Write(kubeconfig)
	sum.Write([]byte(endpoint))
	return hex.EncodeToString(sum.Sum(nil))
}

func (p *ProjectVCPProvider) createAndEngageCluster(
	ctx context.Context, name string, kubeconfig []byte, endpoint, hash string,
) error {
	cfg, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to parse kubeconfig: %w", err)
	}
	if endpoint != "" {
		cfg.Host = endpoint
	}
	for _, opt := range p.opts.RESTOptions {
		if err := opt(cfg); err != nil {
			return fmt.Errorf("failed to apply REST option: %w", err)
		}
	}

	cl, err := cluster.New(cfg, p.opts.ClusterOptions...)
	if err != nil {
		return fmt.Errorf("failed to create cluster: %w", err)
	}

	for _, idx := range p.indexers {
		if err := cl.GetFieldIndexer().IndexField(ctx, idx.object, idx.field, idx.extractValue); err != nil {
			return fmt.Errorf("failed to index field %q: %w", idx.field, err)
		}
	}

	clusterCtx, cancel := context.WithCancel(ctx)
	go func() {
		if err := cl.Start(clusterCtx); err != nil {
			p.log.Error(err, "failed to start cluster", "cluster", name)
		}
	}()

	if err := p.mgr.Engage(clusterCtx, name, cl); err != nil {
		cancel()
		return fmt.Errorf("failed to engage manager: %w", err)
	}

	if !cl.GetCache().WaitForCacheSync(clusterCtx) {
		cancel()
		return fmt.Errorf("failed to sync cache for cluster %q", name)
	}

	p.setCluster(name, activeCluster{
		Cluster: cl,
		Context: clusterCtx,
		Cancel:  cancel,
		Hash:    hash,
	})
	p.log.Info("engaged project VCP", "cluster", name)
	return nil
}

func (p *ProjectVCPProvider) removeCluster(name string) {
	p.lock.Lock()
	ac, ok := p.clusters[name]
	if ok {
		delete(p.clusters, name)
	}
	p.lock.Unlock()
	if ok {
		ac.Cancel()
		p.log.Info("disengaged project VCP", "cluster", name)
	}
}

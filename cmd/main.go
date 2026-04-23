package main

import (
	"context"
	"flag"
	"os"

	extensionsv1alpha1 "github.com/kplane-dev/extensions/api/v1alpha1"
	"github.com/kplane-dev/extensions/internal/controller"
	"github.com/kplane-dev/extensions/internal/provider"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	mcbuilder "sigs.k8s.io/multicluster-runtime/pkg/builder"
	mcmanager "sigs.k8s.io/multicluster-runtime/pkg/manager"
	mcreconcile "sigs.k8s.io/multicluster-runtime/pkg/reconcile"
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(extensionsv1alpha1.AddToScheme(scheme))
}

func main() {
	var metricsAddr string
	var healthAddr string
	var leaderElect bool

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "Address for the metrics endpoint.")
	flag.StringVar(&healthAddr, "health-probe-bind-address", ":8081", "Address for health probes.")
	flag.BoolVar(&leaderElect, "leader-elect", false, "Enable leader election.")
	opts := zap.Options{Development: true}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
	log := ctrl.Log.WithName("main")

	ctx := ctrl.SetupSignalHandler()

	vcpProvider := provider.New(provider.Options{
		LabelSelector: map[string]string{"platform.kplane.dev/type": "project"},
		ClusterOptions: []cluster.Option{
			func(o *cluster.Options) { o.Scheme = scheme },
		},
	})

	mgr, err := mcmanager.New(ctrl.GetConfigOrDie(), vcpProvider, ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsserver.Options{BindAddress: metricsAddr},
		HealthProbeBindAddress: healthAddr,
		LeaderElection:         leaderElect,
		LeaderElectionID:       "extensions.kplane.dev",
	})
	if err != nil {
		log.Error(err, "unable to create manager")
		os.Exit(1)
	}

	if err := vcpProvider.SetupWithManager(ctx, mgr); err != nil {
		log.Error(err, "unable to setup VCP provider")
		os.Exit(1)
	}

	replicator := &controller.PlatformExtensionReplicator{
		MCRManager: mgr,
	}
	if err := replicator.SetupWithManager(mgr.GetLocalManager()); err != nil {
		log.Error(err, "unable to setup PlatformExtensionReplicator")
		os.Exit(1)
	}

	// Watch PlatformExtension on all VCPs. If a project deletes one, re-enqueue
	// the GKE-side object so the replicator restores it.
	if err := mcbuilder.ControllerManagedBy(mgr).
		For(&extensionsv1alpha1.PlatformExtension{}).
		Named("platformextension-vcp-watcher").
		Complete(mcreconcile.Func(func(ctx context.Context, req mcreconcile.Request) (ctrl.Result, error) {
			return replicator.Reconcile(ctx, ctrl.Request{NamespacedName: req.NamespacedName})
		})); err != nil {
		log.Error(err, "unable to setup PlatformExtension VCP watcher")
		os.Exit(1)
	}

	recorders := controller.NewRecorderCache(scheme, "kplane-extensions")
	defer recorders.Shutdown()

	reconciler := &controller.EnabledExtensionReconciler{
		Manager:     mgr,
		LocalClient: mgr.GetLocalManager().GetClient(),
		Recorders:   recorders,
	}

	if err := mcbuilder.ControllerManagedBy(mgr).
		For(&extensionsv1alpha1.EnabledExtension{}).
		Complete(mcreconcile.Func(func(ctx context.Context, req mcreconcile.Request) (ctrl.Result, error) {
			return reconciler.Reconcile(ctx, req)
		})); err != nil {
		log.Error(err, "unable to setup EnabledExtensionReconciler")
		os.Exit(1)
	}

	// Separate local controller: watches GKE ControlPlane resources and touches
	// matching EnabledExtension objects when a new nested CP appears, triggering
	// the MCR reconciler above via the annotation change.
	if err := (&controller.PlatformExtensionBootstrapper{
		MCRManager: mgr,
	}).SetupWithManager(mgr.GetLocalManager()); err != nil {
		log.Error(err, "unable to setup PlatformExtensionBootstrapper")
		os.Exit(1)
	}

	if err := (&controller.NewNestedCPController{
		LocalClient: mgr.GetLocalManager().GetClient(),
		MCRManager:  mgr,
	}).SetupWithManager(mgr.GetLocalManager()); err != nil {
		log.Error(err, "unable to setup NewNestedCPController")
		os.Exit(1)
	}

	if err := mgr.GetLocalManager().AddHealthzCheck("healthz", healthz.Ping); err != nil {
		log.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.GetLocalManager().AddReadyzCheck("readyz", healthz.Ping); err != nil {
		log.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	log.Info("starting extensions operator")
	if err := mgr.Start(ctx); err != nil {
		log.Error(err, "problem running manager")
		os.Exit(1)
	}
}

package manager

import (
	"fmt"

	namespaceclassv1alpha1 "github.com/forest/namespace-class/api/v1alpha1"
	nccontroller "github.com/forest/namespace-class/internal/controller"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

const defaultLeaderElectionID = "namespace-class-controller.namespaceclass.akuity.io"

type Options struct {
	MetricsBindAddress     string
	HealthProbeBindAddress string
	LeaderElection         bool
	LeaderElectionID       string
}

func DefaultOptions() Options {
	return Options{
		MetricsBindAddress:     "0",
		HealthProbeBindAddress: ":8081",
		LeaderElection:         false,
		LeaderElectionID:       defaultLeaderElectionID,
	}
}

func New(restConfig *rest.Config, options Options) (ctrl.Manager, error) {
	if restConfig == nil {
		return nil, fmt.Errorf("rest config must not be nil")
	}

	options = withDefaults(options)

	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("add client-go scheme: %w", err)
	}
	if err := namespaceclassv1alpha1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("add namespaceclass scheme: %w", err)
	}

	mgr, err := ctrl.NewManager(restConfig, ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: options.MetricsBindAddress,
		},
		HealthProbeBindAddress: options.HealthProbeBindAddress,
		LeaderElection:         options.LeaderElection,
		LeaderElectionID:       options.LeaderElectionID,
	})
	if err != nil {
		return nil, fmt.Errorf("create manager: %w", err)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return nil, fmt.Errorf("add healthz check: %w", err)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return nil, fmt.Errorf("add readyz check: %w", err)
	}
	if err := nccontroller.SetupNamespaceReconciler(mgr); err != nil {
		return nil, fmt.Errorf("setup namespace reconciler: %w", err)
	}

	return mgr, nil
}

func withDefaults(options Options) Options {
	defaults := DefaultOptions()
	if options.MetricsBindAddress == "" {
		options.MetricsBindAddress = defaults.MetricsBindAddress
	}
	if options.HealthProbeBindAddress == "" {
		options.HealthProbeBindAddress = defaults.HealthProbeBindAddress
	}
	if options.LeaderElectionID == "" {
		options.LeaderElectionID = defaults.LeaderElectionID
	}
	return options
}

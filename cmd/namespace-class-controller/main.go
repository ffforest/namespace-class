package main

import (
	"flag"
	"fmt"
	"os"

	ncmanager "github.com/forest/namespace-class/internal/manager"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var version = "dev"

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	options := ncmanager.DefaultOptions()
	flag.StringVar(&options.MetricsBindAddress, "metrics-bind-address", options.MetricsBindAddress, "address for metrics endpoint; use 0 to disable")
	flag.StringVar(&options.HealthProbeBindAddress, "health-probe-bind-address", options.HealthProbeBindAddress, "address for health and readiness probes")
	flag.BoolVar(&options.LeaderElection, "leader-elect", options.LeaderElection, "enable leader election")
	flag.StringVar(&options.LeaderElectionID, "leader-election-id", options.LeaderElectionID, "leader election lease name")
	flag.StringVar(&options.PolicyAllowGVKs, "policy-allow-gvks", options.PolicyAllowGVKs, "comma-separated allowed managed resource GVKs; empty allows all except denylist")
	flag.StringVar(&options.PolicyDenyGVKs, "policy-deny-gvks", options.PolicyDenyGVKs, "comma-separated denied managed resource GVKs")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	mgr, err := ncmanager.New(ctrl.GetConfigOrDie(), options)
	if err != nil {
		fmt.Fprintf(os.Stderr, "create manager: %v\n", err)
		os.Exit(1)
	}

	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		fmt.Fprintf(os.Stderr, "run manager: %v\n", err)
		os.Exit(1)
	}
}

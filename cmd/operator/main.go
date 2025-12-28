package main

import (
	"flag"
	"os"

	"mnemosyne/internal/operator"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "metrics address")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false, "leader election")
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	if err := operator.Run(operator.Options{
		MetricsAddr:          metricsAddr,
		EnableLeaderElection: enableLeaderElection,
	}); err != nil {
		os.Exit(1)
	}
}

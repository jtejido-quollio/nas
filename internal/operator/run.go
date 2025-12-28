package operator

import (
	"fmt"
	"net/http"
	"os"

	nasv1 "mnemosyne/api/v1alpha1"
	"mnemosyne/internal/operator/controllers"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

type Options struct {
	MetricsAddr          string
	EnableLeaderElection bool
}

func Run(opts Options) error {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(nasv1.AddToScheme(scheme))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: server.Options{
			BindAddress: opts.MetricsAddr,
		},
		LeaderElection:         opts.EnableLeaderElection,
		LeaderElectionID:       "nas-operator.nas.io",
		HealthProbeBindAddress: ":8081",
	})
	if err != nil {
		return fmt.Errorf("create manager: %w", err)
	}

	baseURL := os.Getenv("NODE_AGENT_BASE_URL")
	authHeader := os.Getenv("NODE_AGENT_AUTH_HEADER")
	authValue := os.Getenv("NODE_AGENT_AUTH_VALUE")
	if baseURL == "" {
		baseURL = "http://nas-node-agent.nas-system.svc.cluster.local:9808"
	}

	cfg := controllers.Config{
		NodeAgentBaseURL: baseURL,
		AuthHeader:       authHeader,
		AuthValue:        authValue,
		Namespace:        "nas-system",
	}

	if err := controllers.SetupAll(mgr, cfg); err != nil {
		return fmt.Errorf("setup controllers: %w", err)
	}

	if err := mgr.AddHealthzCheck("healthz", func(_ *http.Request) error { return nil }); err != nil {
		return err
	}
	if err := mgr.AddReadyzCheck("readyz", func(_ *http.Request) error { return nil }); err != nil {
		return err
	}

	return mgr.Start(ctrl.SetupSignalHandler())
}

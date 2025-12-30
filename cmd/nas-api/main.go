package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"time"

	nasv1 "mnemosyne/api/v1alpha1"
	"mnemosyne/internal/nasapi"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

func main() {
	var listenAddr string
	var namespace string
	var webRoot string

	flag.StringVar(&listenAddr, "listen", ":8080", "listen address")
	flag.StringVar(&namespace, "namespace", "nas-system", "default namespace for CRDs")
	flag.StringVar(&webRoot, "web-root", "", "optional static web root to serve")
	flag.Parse()

	logger := log.New(os.Stdout, "nas-api ", log.LstdFlags)

	restCfg, err := config.GetConfig()
	if err != nil {
		logger.Fatalf("kube config: %v", err)
	}

	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		logger.Fatalf("scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		logger.Fatalf("scheme corev1: %v", err)
	}
	if err := nasv1.AddToScheme(scheme); err != nil {
		logger.Fatalf("scheme nasv1: %v", err)
	}

	k8sClient, err := client.New(restCfg, client.Options{Scheme: scheme})
	if err != nil {
		logger.Fatalf("client: %v", err)
	}

	srv := nasapi.NewServer(k8sClient, namespace, webRoot, logger)

	httpServer := &http.Server{
		Addr:              listenAddr,
		Handler:           srv,
		ReadHeaderTimeout: 10 * time.Second,
	}

	logger.Printf("listening on %s", listenAddr)
	if err := httpServer.ListenAndServe(); err != nil {
		logger.Fatalf("server: %v", err)
	}
}

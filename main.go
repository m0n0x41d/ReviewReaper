package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/NaNameUz3r/review_autostop_service/mylog"
	"github.com/NaNameUz3r/review_autostop_service/namespaces_informer"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func main() {
	logger := mylog.NewLogger()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	clusterConfig, err := setClusterConfig()
	if err != nil {
		logger.WithError(err).Fatal("Could not get config")
	}

	clusterClient, err := kubernetes.NewForConfig(clusterConfig)
	if err != nil {
		logger.WithError(err).Fatal("Could not make client")
	}
	newInformer := namespaces_informer.NewNsInformer(clusterClient, logger)
	if err := newInformer.Run(ctx); err != nil {
		logger.WithError(err).Fatal("Could not start informer")
	}
	<-ctx.Done()
}

func setClusterConfig() (*rest.Config, error) {
	config, err := rest.InClusterConfig()

	if kubeConfig := os.Getenv("KUBECONFIG"); kubeConfig != "" {
		config, err = clientcmd.BuildConfigFromFlags("", kubeConfig)
	}
	if err != nil {
		return nil, err
	}
	return config, nil
}

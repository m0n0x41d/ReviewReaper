package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"NaNameUz3r/ReviewReaper/logs"
	"NaNameUz3r/ReviewReaper/namespaces_informer"
	"NaNameUz3r/ReviewReaper/utils"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	// tmp import for dev running
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
)

func main() {

	appConfig, err := utils.LoadConfig()
	if err != nil {

		panic(err)
	}

	logger := logs.NewLogger(appConfig)
	logs.StartUp(appConfig, logger)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	clusterConfig, err := setClusterConfig()
	if err != nil {
		logger.Error("Could not get ClusterConfig", err)
	}

	clusterClient, err := kubernetes.NewForConfig(clusterConfig)
	if err != nil {
		logger.Error("Could not make client", err)
	}

	newInformer := namespaces_informer.NewNsInformer(clusterConfig, clusterClient, logger, appConfig)
	if err := newInformer.Run(ctx); err != nil {
		logger.Error("Could not start informer", err)
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

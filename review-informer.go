package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/NaNameUz3r/review_autostop_service/logs"
	"github.com/NaNameUz3r/review_autostop_service/namespaces_informer"
	"github.com/NaNameUz3r/review_autostop_service/utils"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	// tmp import for dev running
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
)

var ()

func main() {
	logger := logs.NewLogger()

	config, err := utils.LoadConfig()
	if err != nil {
		logger.Fatal("Could not load config.yaml, aborting.")
	}

	fmt.Printf("%+v\n", config)

	logger.Info("Will watch for namespaces with prefixes: ", config.WatchNamespaces)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	clusterConfig, err := setClusterConfig()
	if err != nil {
		logger.WithError(err).Fatal("Could not get ClusterConfig")
	}

	clusterClient, err := kubernetes.NewForConfig(clusterConfig)
	if err != nil {
		logger.WithError(err).Fatal("Could not make client.")
	}

	newInformer := namespaces_informer.NewNsInformer(clusterClient, logger, config)
	if err := newInformer.Run(ctx); err != nil {
		logger.WithError(err).Fatal("Could not start informer.")
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

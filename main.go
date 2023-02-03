package main

import (
	"os"

	"github.com/NaNameUz3r/review_autostop_operator/namespaces_informer"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func main() {

	clusterConfig, err := setClusterConfig()
	if err != nil {
		logrus.WithError(err).Fatal("Could not get config")
	}

	clusterClient, err := dynamic.NewForConfig(clusterConfig)
	if err != nil {
		logrus.WithError(err).Panic("Could not make client")
	}

	namespaces_informer.RunNsInformer(clusterClient)

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

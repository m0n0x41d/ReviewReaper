package main

import (
	"os"

	"github.com/NaNameUz3r/review_autostop_service/namespaces_informer"
	"github.com/NaNameUz3r/review_autostop_service/utils"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func main() {

	clusterConfig, err := setClusterConfig()
	if err != nil {
		utils.Logger.WithError(err).Fatalln("Could not get config")
	}

	clusterClient, err := kubernetes.NewForConfig(clusterConfig)
	if err != nil {
		utils.Logger.WithError(err).Panicln("Could not make client")
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

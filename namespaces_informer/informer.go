package namespaces_informer

import (
	"fmt"

	"github.com/NaNameUz3r/review_autostop_service/utils"
	"github.com/sirupsen/logrus"

	// "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

func RunNsInformer(client *kubernetes.Clientset) {

	// stop signal for the informer
	stopper := make(chan struct{})
	defer close(stopper)

	factory := informers.NewSharedInformerFactory(client, 0)
	namespaceInformer := factory.Core().V1().Namespaces()
	informer := namespaceInformer.Informer()

	defer runtime.HandleCrash()

	// start informer ->
	go factory.Start(stopper)

	// start to sync and call list
	if !cache.WaitForCacheSync(stopper, informer.HasSynced) {
		runtime.HandleError(fmt.Errorf("Timed out waiting for caches to sync"))
		return
	}

	// TODO: We going to lable all review namaspaces with some lable containing expiration timestamp.
	// At first glance we need only AddFunc, but, probably, we need some fool-protection for lable deletion in UpdateFunc.
	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    onAdd,
		UpdateFunc: func(interface{}, interface{}) { fmt.Println("update not implemented") },
		DeleteFunc: func(interface{}) { fmt.Println("delete not implemented") },
	})

	// TODO: find all namespaces
	lister := namespaceInformer.Lister()

	namespaces, err := lister.List(labels.Everything())

	if err != nil {
		utils.Logger.WithError(err).Errorln("Could not list namespaces")
	}

	// TODO: Make fiels type usable through project module
	utils.Logger.WithFields(utils.logrus.Fields{
		"ClusterNamespaces": namespaces,
	}).Info("Trololo")

	<-stopper
}

func onAdd(obj interface{}) {
	logrus.Info("Check-check, new namespace added and list resynchronized")
}

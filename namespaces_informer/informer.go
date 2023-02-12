package namespaces_informer

import (
	"context"
	"errors"
	"fmt"

	"github.com/NaNameUz3r/review_autostop_service/mylog"

	// "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

type NsInformer struct {
	client *kubernetes.Clientset
	logger mylog.Logger
}

func NewNsInformer(client *kubernetes.Clientset, logger mylog.Logger) *NsInformer {
	return &NsInformer{
		client: client,
		logger: logger,
	}
}

func (n *NsInformer) Run(stopper context.Context) error {

	factory := informers.NewSharedInformerFactory(n.client, 0)
	namespaceInformer := factory.Core().V1().Namespaces()
	informer := namespaceInformer.Informer()

	// TODO: We going to lable all review namaspaces with some lable containing expiration timestamp.
	// At first glance we need only AddFunc, but, probably, we need some fool-protection for lable deletion in UpdateFunc.
	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    n.onAdd,
		UpdateFunc: func(interface{}, interface{}) { fmt.Println("update not implemented") },
		DeleteFunc: func(interface{}) { fmt.Println("delete not implemented") },
	})

	// start informer ->
	go factory.Start(stopper.Done())

	// start to sync and call list
	if !cache.WaitForCacheSync(stopper.Done(), informer.HasSynced) {
		// runtime.HandleError()
		return errors.New("timed out waiting for caches to sync")
	}

	// TODO: find all namespaces
	lister := namespaceInformer.Lister()

	namespaces, err := lister.List(labels.Everything())
	if err != nil {
		return errors.New("could not list namespaces")
	}

	namespacesNames := make([]string, len(namespaces))
	namespacesMap := make(mylog.Fields)
	for i, name := range namespaces {
		namespacesNames[i] = name.Name
		namespacesMap[name.Name] = name
	}
	n.logger.WithField("ClusterNamespaces", namespacesNames).Info("Trololo")
	n.logger.WithFields(namespacesMap).Info("Trololololo2")
	return nil
}

func (n *NsInformer) onAdd(obj interface{}) {
	n.logger.Info("Check-check, new namespace added and list resynchronized")
}

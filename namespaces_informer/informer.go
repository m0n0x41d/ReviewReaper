package namespaces_informer

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/NaNameUz3r/review_autostop_service/logs"
	"github.com/NaNameUz3r/review_autostop_service/util"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

type NsInformer struct {
	client *kubernetes.Clientset
	logger logs.Logger
}

func NewNsInformer(client *kubernetes.Clientset, logger logs.Logger) *NsInformer {
	return &NsInformer{
		client: client,
		logger: logger,
	}
}

func (n *NsInformer) Run(stopper context.Context, config util.Config) error {

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

	// fmt.Println(config.WatchNamespaces[0])
	watchedNamespacesNames := make([]string, 0)
	watchedNamespaces := make([]*corev1.Namespace, 0)
	namespacesMap := make(logs.Fields)
	for _, ns := range namespaces {
		if isWatched(ns.Name, config.WatchNamespaces) {
			watchedNamespacesNames = append(watchedNamespacesNames, ns.Name)
			watchedNamespaces = append(watchedNamespaces, ns)
			namespacesMap[ns.Name] = ns
		}

	}
	n.logger.WithField("WatchedNamespaces", watchedNamespacesNames).Info("Trololo")
	// n.logger.WithFields(namespacesMap).Info("Trololololo2")

	err = ensureLabeled(stopper, n.client, watchedNamespaces, config)
	if err != nil {
		n.logger.Error(err)
	}

	defDecomissionTimestamp(watchedNamespaces[0], config.RetentionTime)

	return nil
}

func (n *NsInformer) onAdd(obj interface{}) {
	n.logger.Info("NsInformer cache is resynced.")
}

func isWatched(nsName string, watchedNs []string) bool {
	isMatched := false
	for _, ns := range watchedNs {
		if strings.HasPrefix(nsName, ns) {
			isMatched = true
			break
		}
	}
	return isMatched
}

func ensureLabeled(context context.Context, client *kubernetes.Clientset, namespaces []*corev1.Namespace, config util.Config) error {
	for _, ns := range namespaces {
		annotations := getNsAnnotations(ns)
		_, ok := annotations[config.DeletionAnnotationKey]
		if !ok {

			decommissionTimestamp := defDecomissionTimestamp(ns, config.RetentionTime).UTC().Format(time.RFC3339)
			newNs := ns.DeepCopy()
			annotations := newNs.ObjectMeta.Annotations

			if annotations == nil {
				annotations = make(map[string]string)
			}

			annotations[config.DeletionAnnotationKey] = decommissionTimestamp

			newNs.ObjectMeta.Annotations = annotations

			updateOptions := metav1.UpdateOptions{}
			_, err := client.CoreV1().Namespaces().Update(context, newNs, updateOptions)

			if err != nil {
				return err
			}
		}
	}

	return nil
}

func getNsCreationTimestamp(ns *corev1.Namespace) time.Time {
	return ns.ObjectMeta.CreationTimestamp.Time
}

func getNsAnnotations(ns *corev1.Namespace) map[string]string {
	return ns.ObjectMeta.Annotations
}

func defDecomissionTimestamp(ns *corev1.Namespace, retention int) time.Time {
	createdAt := getNsCreationTimestamp(ns)

	var timeout time.Duration
	timeout = time.Duration(retention)
	decommissionTimestamp := createdAt.Add(time.Hour * 24 * timeout)

	// fmt.Println(decommissionTimestamp.UTC().Format(time.RFC3339))
	return decommissionTimestamp
}

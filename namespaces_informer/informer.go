package namespaces_informer

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/NaNameUz3r/review_autostop_service/logs"
	"github.com/NaNameUz3r/review_autostop_service/utils"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	v1 "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

// pass this in struct below. Make corresponding functions struct methods.
var Config utils.Config

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

func (n *NsInformer) Run(ctx context.Context, cfg utils.Config) error {

	Config = cfg
	// TODO: this timeout should be changed on release -------------↓
	informerFactory := informers.NewSharedInformerFactory(n.client, 0)
	namespaceInformer := informerFactory.Core().V1().Namespaces()
	informer := namespaceInformer.Informer()

	// TODO: Мы делаем onAdd и обновляем новый неймспейс, если он вотчится, с аннотацией.
	// Мы в onUpdate чекаем не удалили ли случайно пользователи аннотацию с неймспейса, который мы вотчим, и пишем её снова, если удалили.
	// Необходимо определить наиболее оптимальный способ релистануть все неймспейсы в шедуленной горутине-удоляторе. Пихать в неё namespaceInformer и делать list снова?
	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    n.onAdd(ctx),
		UpdateFunc: n.onUpdate(ctx),
		DeleteFunc: func(interface{}) { return },
	})

	// start informer ->
	go informerFactory.Start(ctx.Done())

	// start to sync and call list
	if !cache.WaitForCacheSync(ctx.Done(), informer.HasSynced) {
		return errors.New("timed out waiting for caches to sync")
	}

	watchedNamespaces, err := listWatchedNamespaces(namespaceInformer, Config)
	if err != nil {
		return errors.New("Could not list namespaces")
	}

	fmt.Println("[DEBUG]: watchedNamespaces count now: ", len(watchedNamespaces))

	err = ensureAnnotated(ctx, n.client, watchedNamespaces, Config)
	if err != nil {
		n.logger.Error(err)
	}

	return nil
}

func (n *NsInformer) onAdd(ctx context.Context) func(interface{}) {

	return func(obj interface{}) {
		namespace := obj.(*corev1.Namespace)
		if isWatched(namespace.Name, Config.WatchNamespaces) {
			ensureAnnotated(ctx, n.client, []*corev1.Namespace{namespace}, Config)
		}
	}
}

func (n *NsInformer) onUpdate(ctx context.Context) func(interface{}, interface{}) {
	return func(oldObj interface{}, newObj interface{}) {
		newNamespace := newObj.(*corev1.Namespace)
		if isWatched(newNamespace.Name, Config.WatchNamespaces) {

			// TODO: Probably we could just copy annotation
			ensureAnnotated(ctx, n.client, []*corev1.Namespace{newNamespace}, Config)
		}
	}
}

func listWatchedNamespaces(informer v1.NamespaceInformer, config utils.Config) (namespaces []*corev1.Namespace, err error) {
	lister := informer.Lister()
	watchedNamespaces := make([]*corev1.Namespace, 0)

	namespaces, err = lister.List(labels.Everything())
	if err != nil {
		return watchedNamespaces, errors.New("could not list namespaces")
	}

	for _, ns := range namespaces {
		if isWatched(ns.Name, config.WatchNamespaces) {
			watchedNamespaces = append(watchedNamespaces, ns)
		}

	}

	return watchedNamespaces, err

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

// TODO: Might be a good idea to decompose is down on two funcs: isAnnotated and Annotate.
func ensureAnnotated(context context.Context, client *kubernetes.Clientset, namespaces []*corev1.Namespace, config utils.Config) error {
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

// TODO: Andd hours in config for retention fine-tuning.
func defDecomissionTimestamp(ns *corev1.Namespace, retention int) time.Time {
	createdAt := getNsCreationTimestamp(ns)

	var timeout time.Duration
	timeout = time.Duration(retention)
	decommissionTimestamp := createdAt.Add(time.Hour * 24 * timeout)

	// fmt.Println(decommissionTimestamp.UTC().Format(time.RFC3339))
	return decommissionTimestamp
}

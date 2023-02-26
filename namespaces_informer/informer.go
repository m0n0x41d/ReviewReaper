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
	"k8s.io/client-go/kubernetes"
	v1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

// pass this in struct below. Make corresponding functions struct methods.

type NsInformer struct {
	client *kubernetes.Clientset
	logger logs.Logger
	config utils.Config
}

func NewNsInformer(client *kubernetes.Clientset, logger logs.Logger, config utils.Config) *NsInformer {
	return &NsInformer{
		client: client,
		logger: logger,
		config: config,
	}
}

func (n *NsInformer) Run(ctx context.Context) error {

	// TODO: this timeout should be changed on release -------------↓
	informerFactory := informers.NewSharedInformerFactory(n.client, 0)
	namespaceInformer := informerFactory.Core().V1().Namespaces().Informer()
	namespaceLister := informerFactory.Core().V1().Namespaces().Lister()

	// eventInformer := informerFactory.Core().V1().Events().Informer()
	// eventsLister := informerFactory.Core().V1().Events().Lister()

	// TODO: Мы делаем onAdd и обновляем новый неймспейс, если он вотчится, с аннотацией.
	// Мы в onUpdate чекаем не удалили ли случайно пользователи аннотацию с неймспейса, который мы вотчим, и пишем её снова, если удалили.
	// Необходимо определить наиболее оптимальный способ релистануть все неймспейсы в шедуленной горутине-удоляторе. Пихать в неё namespaceInformer и делать list снова?
	namespaceInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    n.onAdd(ctx),
		UpdateFunc: n.onUpdate(ctx),
		DeleteFunc: func(interface{}) { return },
	})

	// start informer ->
	go informerFactory.Start(ctx.Done())

	// start to sync and call list
	if !cache.WaitForCacheSync(ctx.Done(), namespaceInformer.HasSynced) {
		return errors.New("timed out waiting for caches to sync")
	}

	n.testLister(namespaceLister)

	n.DeletionTicker()

	// 1. Проверgить что onAdd втоматом делает ensure на старте информера DONE
	// 2. Передавать листер в горутину тикер. OK FINE
	// 3. Узнать прилетает ли эвент при удалении или добавлении пода (любой эвент.) Если нет то надо хуярить информеры дополнительные.
	// TODO: касаемо пункта 3 — нет, эвенты не вызывают onUpdate информера неймспейсов. Чтобы не делать кучу информеров на все ресурсы, можно попробовать сделать eventInformer который
	// TODO: будет слушать все эвенты, и реагировать на эвенты у которых в ObjectMeta.Namespace неймспейс который мы вотчим (по префиксу снова проверяем.)
	// TODO: смотреть за эвентами - хуйня. Эвенты может срать какой нибудь рэббитоператор, тогда как может ничего не выкатываться в неймспейс вообще неделями. Это не надежно.
	return nil
}

func (n *NsInformer) onAdd(ctx context.Context) func(interface{}) {

	return func(obj interface{}) {
		namespace := obj.(*corev1.Namespace)
		if isWatched(namespace.Name, n.config.WatchNamespaces) {
			n.ensureAnnotated(ctx, n.client, namespace)
		}
	}
}

func (n *NsInformer) onUpdate(ctx context.Context) func(interface{}, interface{}) {

	return func(oldObj interface{}, newObj interface{}) {
		newNamespace := newObj.(*corev1.Namespace)

		fmt.Println("Hey buddy we got some event here in namespace: ", newNamespace.Name)
		if isWatched(newNamespace.Name, n.config.WatchNamespaces) {

			// TODO: Probably we could just copy annotation
			n.ensureAnnotated(ctx, n.client, newNamespace)
		}
	}
}

func (n *NsInformer) listWatchedNamespaces(lister v1.NamespaceLister) (namespaces []*corev1.Namespace, err error) {
	watchedNamespaces := make([]*corev1.Namespace, 0)

	namespaces, err = lister.List(labels.Everything())
	if err != nil {
		return watchedNamespaces, errors.New("could not list namespaces")
	}

	for _, ns := range namespaces {
		if isWatched(ns.Name, n.config.WatchNamespaces) {
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
// TODO: It alway get one namespace no need to pass list you dumbass
func (n *NsInformer) ensureAnnotated(context context.Context, client *kubernetes.Clientset, ns *corev1.Namespace) error {
	annotations := getNsAnnotations(ns)
	_, ok := annotations[n.config.DeletionAnnotationKey]
	if !ok {

		decommissionTimestamp := defDecomissionTimestamp(ns, n.config.RetentionDays).UTC().Format(time.RFC3339)
		newNs := ns.DeepCopy()
		annotations := newNs.ObjectMeta.Annotations

		if annotations == nil {
			annotations = make(map[string]string)
		}

		annotations[n.config.DeletionAnnotationKey] = decommissionTimestamp

		newNs.ObjectMeta.Annotations = annotations

		updateOptions := metav1.UpdateOptions{}
		_, err := client.CoreV1().Namespaces().Update(context, newNs, updateOptions)

		if err != nil {
			return err
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

func (n *NsInformer) testLister(lister v1.NamespaceLister) (err error) {

	namespaces, err := lister.List(labels.Everything())
	if err != nil {
		return err
	}

	watchedNs, err := n.listWatchedNamespaces(lister)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Printf("Lister got %d namespaces\n", len(namespaces))
	fmt.Printf("Watche among them: %d\n", len(watchedNs))

	return nil
}

func (n *NsInformer) DeletionTicker() {
	ticker := time.NewTicker(3 * time.Second)
	done := make(chan bool)

	go func() {
		for {
			select {
			case <-done:
				return
			case t := <-ticker.C:
				if n.isAllowedWindow(t) {
					fmt.Println("WINDOW IS GREEN, LETS ROCK AND DELETE SOME BITCHES!")
				}
			}
		}
	}()
}

func (n *NsInformer) isAllowedWindow(t time.Time) bool {
	const HH_MM = "15:04"
	isAllowed := false

	nbCfg := n.config.MaintenanceWindow.NotBefore
	naCfg := n.config.MaintenanceWindow.NotAfter

	todayWeekday := t.UTC().Weekday().String()[0:3]
	weekdayOk := utils.IsContains(n.config.MaintenanceWindow.WeekDays, todayWeekday)

	if weekdayOk == false {
		return isAllowed
	}

	notBeforeCfg, _ := time.Parse(HH_MM, nbCfg)
	notAfterCfg, _ := time.Parse(HH_MM, naCfg)

	notBefore := time.Date(t.Year(), t.Month(), t.Day(), notBeforeCfg.Hour(), notBeforeCfg.Minute(), 0, 0, time.UTC)
	notAfter := time.Date(t.Year(), t.Month(), t.Day(), notAfterCfg.Hour(), notAfterCfg.Minute(), 0, 0, time.UTC)

	if t.After(notBefore) && t.Before(notAfter) {
		isAllowed = true
	}

	return isAllowed
}

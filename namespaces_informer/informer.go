package namespaces_informer

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/NaNameUz3r/review_autostop_service/logs"
	"github.com/NaNameUz3r/review_autostop_service/utils"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/cli"

	"helm.sh/helm/v3/pkg/release"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	v1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

// pass this in struct below. Make corresponding functions struct methods.
const (
	HH_MM        = "15:04"
	RFC3339local = "2006-01-02T15:04:05Z"
)

type NsInformer struct {
	restConfig *rest.Config
	client     *kubernetes.Clientset
	logger     logs.Logger
	appConfig  utils.Config
}

func NewNsInformer(restConfig *rest.Config, client *kubernetes.Clientset, logger logs.Logger, appConfig utils.Config) *NsInformer {
	return &NsInformer{
		restConfig: restConfig,
		client:     client,
		logger:     logger,
		appConfig:  appConfig,
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

	n.DeletionTicker(ctx, namespaceLister)

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
		if isWatched(namespace.Name, n.appConfig.NamespacePrefixes) {
			n.ensureAnnotated(ctx, n.client, namespace)
		}
	}
}

func (n *NsInformer) onUpdate(ctx context.Context) func(interface{}, interface{}) {

	return func(oldObj interface{}, newObj interface{}) {
		newNamespace := newObj.(*corev1.Namespace)

		if isWatched(newNamespace.Name, n.appConfig.NamespacePrefixes) {

			// TODO: Probably we could just copy annotation
			n.ensureAnnotated(ctx, n.client, newNamespace)
		}
	}
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
func (n *NsInformer) ensureAnnotated(ctx context.Context, client *kubernetes.Clientset, ns *corev1.Namespace) error {
	annotations := getNsAnnotations(ns)
	_, ok := annotations[n.appConfig.AnnotationKey]
	if !ok {

		decommissionTimestamp := n.defDecomissionTimestamp(ns).UTC().Format(time.RFC3339)
		newNs := ns.DeepCopy()
		annotations := newNs.ObjectMeta.Annotations

		if annotations == nil {
			annotations = make(map[string]string)
		}

		annotations[n.appConfig.AnnotationKey] = decommissionTimestamp

		newNs.ObjectMeta.Annotations = annotations

		updateOptions := metav1.UpdateOptions{}
		_, err := client.CoreV1().Namespaces().Update(ctx, newNs, updateOptions)

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
func (n *NsInformer) defDecomissionTimestamp(ns *corev1.Namespace) time.Time {
	createdAt := getNsCreationTimestamp(ns)

	retentionDays := n.appConfig.RetentionDays
	retentionHours := n.appConfig.RetentionHours

	timeoutDays := time.Duration(retentionDays)
	decommissionTimestamp := createdAt.Add(time.Hour * 24 * timeoutDays)

	if retentionHours > 0 {
		timeoutHours := time.Duration(retentionHours)
		decommissionTimestamp = decommissionTimestamp.Add(time.Hour * timeoutHours)
	}

	return decommissionTimestamp
}

func (n *NsInformer) DeletionTicker(ctx context.Context, lister v1.NamespaceLister) {
	ticker := time.NewTicker(5 * time.Second)
	done := make(chan bool)

	go func() {
		isActive := false
		for {
			if !isActive {
				select {
				case <-done:
					return
				case t := <-ticker.C:
					if n.isAllowedWindow(t) && !isActive {
						isActive = true
						expiredNamespaces, err := n.getExpiredNamespaces(lister)
						if err != nil {
							n.logger.Error("Could not list watched namespaces for deletion", err)
						}

						n.logger.Info("Expired namespaces found", "count", len(expiredNamespaces))
						err = n.processExpiredNamespaces(ctx, expiredNamespaces)
						if err != nil {
							n.logger.Error("Could not process expired namespaces", err)
						}
						time.Sleep(5 * time.Second)
						isActive = false
					}
				}
			}
			continue
		}
	}()
}

func (n *NsInformer) processExpiredNamespaces(ctx context.Context, namespaces []*corev1.Namespace) error {

	batchSize := n.appConfig.DeletionBatchSize
	napSeconds := time.Duration(n.appConfig.DeletionNapSeconds) * time.Second

	if batchSize == 0 {
		batchSize = len(namespaces)
	}

	for i := 0; i < len(namespaces); i += batchSize {
		batchTail := i + batchSize
		if batchTail > len(namespaces) {
			batchTail = len(namespaces)
		}
		batch := namespaces[i:batchTail]

		// Process the batch of namespaces
		err := n.deleteNamespaces(ctx, batch)
		if err != nil {
			n.logger.Error("Could not delete namespaces", err)
			return err
		}

		time.Sleep(napSeconds)
	}

	return nil
}

func (n *NsInformer) deleteNamespaces(ctx context.Context, namespaces []*corev1.Namespace) error {
	deleteOptions := metav1.DeleteOptions{}
	for _, ns := range namespaces {

		if n.appConfig.IsDeleteByRelease {
			n.deleteNamespaceReleases(ctx, ns)
		}
		err := n.client.CoreV1().Namespaces().Delete(ctx, ns.Name, deleteOptions)
		if err != nil {
			// If the namespace is already deleted, return without error.
			if apierrors.IsNotFound(err) {
				return nil
			}
			return err
		}
		n.logger.Info("Namespace", ns.Name, "Deleted.")

	}
	return nil
}

func (n *NsInformer) deleteNamespaceReleases(ctx context.Context, namespace *corev1.Namespace) error {

	settings := cli.New()
	settings.SetNamespace(namespace.Name)
	actionConfig := new(action.Configuration)

	if err := actionConfig.Init(settings.RESTClientGetter(), settings.Namespace(), "secret", n.logger.Debug); err != nil {
		n.logger.Error("Could not initialize helm action config", err)
		return err
	}

	listAction := action.NewList(actionConfig)

	releases, err := listAction.Run()
	if err != nil {
		n.logger.Error("Could not list releases", err)
		return err
	}

	deleteAction := action.NewUninstall(actionConfig)
	deleteAction.DisableHooks = false

	wg := &sync.WaitGroup{}

	for _, r := range releases {
		wg.Add(1)
		go func(r *release.Release, wg *sync.WaitGroup) {
			deleteAction.Run(r.Name)
			n.logger.Info("Uninstalling helm release", "name", r.Name, "from namespace", namespace.Name)
			wg.Done()
		}(r, wg)
	}
	wg.Wait()

	return nil
}

func (n *NsInformer) getExpiredNamespaces(lister v1.NamespaceLister) (expiredNamespaces []*corev1.Namespace, err error) {
	timeNow := time.Now().UTC()
	watchedNamespaces, err := n.listWatchedNamespaces(lister)
	if err != nil {
		return expiredNamespaces, err
	}

	for _, ns := range watchedNamespaces {
		timeStampAnnotation := ns.Annotations[n.appConfig.AnnotationKey]
		nsDeletionTimespamp, err := time.Parse(RFC3339local, timeStampAnnotation)
		if err != nil {
			return expiredNamespaces, err
		}

		if nsDeletionTimespamp.Before(timeNow) {
			expiredNamespaces = append(expiredNamespaces, ns)
		}
	}

	return expiredNamespaces, nil
}

func (n *NsInformer) listWatchedNamespaces(lister v1.NamespaceLister) (namespaces []*corev1.Namespace, err error) {
	watchedNamespaces := make([]*corev1.Namespace, 0)

	namespaces, err = lister.List(labels.Everything())
	if err != nil {
		return watchedNamespaces, errors.New("could not list namespaces")
	}

	for _, ns := range namespaces {
		if isWatched(ns.Name, n.appConfig.NamespacePrefixes) {
			watchedNamespaces = append(watchedNamespaces, ns)
		}

	}
	return watchedNamespaces, err

}

func (n *NsInformer) isAllowedWindow(t time.Time) bool {

	isAllowed := false

	nbCfg := n.appConfig.DeletionWindow.NotBefore
	naCfg := n.appConfig.DeletionWindow.NotAfter

	todayWeekday := t.UTC().Weekday().String()[0:3]
	weekdayOk := utils.IsContains(n.appConfig.DeletionWindow.WeekDays, todayWeekday)

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

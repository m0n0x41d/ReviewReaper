package namespaces_informer

import (
	"context"
	"errors"
	"fmt"
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
	listers "k8s.io/client-go/listers/core/v1"
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

	nsLister     listers.NamespaceLister
	eventsLister listers.EventLister
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

	factoryNsInformer := informerFactory.Core().V1().Namespaces()
	namespaceInformer := factoryNsInformer.Informer()
	namespaceLister := factoryNsInformer.Lister()

	n.nsLister = namespaceLister

	eventInformer := informerFactory.Core().V1().Events().Informer()
	eventsLister := informerFactory.Core().V1().Events().Lister()

	n.eventsLister = eventsLister

	namespaceInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    n.onAddNamespace(ctx),
		UpdateFunc: n.onUpdateNamespace(ctx),
		DeleteFunc: func(interface{}) { return },
	})

	eventInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    n.onAddEvent(ctx),
		UpdateFunc: func(interface{}, interface{}) { return },
		DeleteFunc: func(interface{}) { return },
	})

	// start informer ->
	go informerFactory.Start(ctx.Done())
	// start to sync and call list
	if !cache.WaitForCacheSync(ctx.Done(), namespaceInformer.HasSynced) {
		return errors.New("timed out waiting for caches to sync")
	}

	n.DeletionTicker(ctx)

	return nil
}

func (n *NsInformer) onAddNamespace(ctx context.Context) func(interface{}) {

	return func(obj interface{}) {
		namespace := obj.(*corev1.Namespace)
		if isWatched(namespace.Name, n.appConfig.NamespacePrefixes) {
			n.ensureAnnotated(ctx, n.client, namespace)
		}
	}
}

func (n *NsInformer) onUpdateNamespace(ctx context.Context) func(interface{}, interface{}) {

	return func(oldObj interface{}, newObj interface{}) {
		newNamespace := newObj.(*corev1.Namespace)

		if isWatched(newNamespace.Name, n.appConfig.NamespacePrefixes) {

			// TODO: Probably we could just copy annotation
			n.ensureAnnotated(ctx, n.client, newNamespace)
		}
	}
}

func (n *NsInformer) onAddEvent(ctx context.Context) func(interface{}) {
	return func(obj interface{}) {
		// watchedNamespacesNames, _ := n.listWatchedNamespacesNames(n.nsLister)
		// event := obj.(*corev1.Event)

		// if utils.IsContains(watchedNamespacesNames, event.ObjectMeta.Namespace) {
		// 	fmt.Printf("GOT THIS EVENT FROM NAMESPACE %s", event.ObjectMeta.Namespace)
		// 	fmt.Println(event.CreationTimestamp.UTC())
		// }
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
// TODO: We need to be sure that annotation value (timestamp) not changed to some gibberish.
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

func (n *NsInformer) DeletionTicker(ctx context.Context) {
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

						watchedNamespaces, err := n.listWatchedNamespaces()
						if err != nil {
							n.logger.Error("Could not list watched namespaces for deletion", err)
						}

						if n.appConfig.PostponeDeletion && len(watchedNamespaces) > 0 {
							n.postponeActive(watchedNamespaces)
						}

						expiredNamespaces := n.filterExpiredNamespaces(watchedNamespaces)

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

func (n *NsInformer) postponeActive(watchedNamespaces []*corev1.Namespace) error {
	// fmt.Println("EVENT TIMES OF NAMESPACE: ", watchedNamespaces[0].Name)
	// for _, namespace := range watchedNamespaces {
	// 	events, _ := n.eventsLister.Events(namespace.Name).List(labels.Everything())

	// 	// TODO: Filter events by types (create and update) and by default k8s resources: pod, ingress, svc.
	// 	for _, e := range events {
	// 		fmt.Print(e.Name, " CreatedIS:", e.ObjectMeta.CreationTimestamp, " Reason:", e.Reason, " OwnerRef: ", e.OwnerReferences, "\n\n\n")

	// 		ey, em, ed := e.EventTime.UTC().Date()
	// 		ty, tm, td := time.Now().UTC().Date()
	// 		if ey == ty && em == tm && ed == td {
	// 			fmt.Println("Event", e.Name, "is happening today, updating retention date.")
	// 			fmt.Println(e.CreationTimestamp)
	// 			// TODO: Add retention time to current timestamp
	// 		}
	// 	}
	// }
	// TODO: It is look like that k8s events is not reliable source of information for our needs.
	for _, ns := range watchedNamespaces {
		nsReleases, _ := n.listNamespaceReleases(ns)
		for _, release := range nsReleases {
			fmt.Println(release.Name, " last deployed: ", release.Info.LastDeployed.UTC())
		}
	}
	return nil
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
			releases, err := n.listNamespaceReleases(ns)
			if err != nil {
				n.logger.Error("Could not list releases", "namespace", ns.Name)
			}
			n.deleteNamespaceReleases(releases, ns)
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

func (n *NsInformer) listNamespaceReleases(namespace *corev1.Namespace) ([]*release.Release, error) {
	releasesList := make([]*release.Release, 0)

	settings := cli.New()
	settings.SetNamespace(namespace.Name)
	actionConfig := new(action.Configuration)

	if err := actionConfig.Init(settings.RESTClientGetter(), settings.Namespace(), "secret", n.logger.Debug); err != nil {
		n.logger.Error("Could not initialize helm action config", err)
		return releasesList, err
	}

	listAction := action.NewList(actionConfig)

	releasesList, err := listAction.Run()
	if err != nil {
		n.logger.Error("Could not list releases", err)
		return releasesList, err
	}

	return releasesList, nil

}

func (n *NsInformer) deleteNamespaceReleases(releases []*release.Release, namespace *corev1.Namespace) error {
	// TODO: is there some way to catch error?
	settings := cli.New()
	settings.SetNamespace(namespace.Name)
	actionConfig := new(action.Configuration)

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

func (n *NsInformer) filterExpiredNamespaces(watchedNamespaces []*corev1.Namespace) (expiredNamespaces []*corev1.Namespace) {
	timeNow := time.Now().UTC()

	for _, ns := range watchedNamespaces {
		timeStampAnnotation := ns.Annotations[n.appConfig.AnnotationKey]
		nsDeletionTimespamp, err := time.Parse(RFC3339local, timeStampAnnotation)
		if err != nil {
			n.logger.Error("Invalid timestamp parsed from watched namespace")
			return expiredNamespaces
		}

		if nsDeletionTimespamp.Before(timeNow) {
			expiredNamespaces = append(expiredNamespaces, ns)
		}
	}

	return expiredNamespaces
}

func (n *NsInformer) listWatchedNamespaces() (namespaces []*corev1.Namespace, err error) {
	watchedNamespaces := make([]*corev1.Namespace, 0)

	namespaces, err = n.nsLister.List(labels.Everything())
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

func (n *NsInformer) listWatchedNamespacesNames() (namespaces []string, err error) {
	watchedNamespaces, err := n.listWatchedNamespaces()
	names := make([]string, 0)

	if err != nil {
		return names, err
	}
	for _, ns := range watchedNamespaces {
		names = append(names, ns.Name)
	}
	return names, err
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

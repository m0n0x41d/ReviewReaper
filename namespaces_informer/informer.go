package namespaces_informer

import (
	"context"
	"errors"
	"sync"
	"time"

	"NaNameUz3r/ReviewReaper/logs"
	"NaNameUz3r/ReviewReaper/utils"

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
	TICK_SECONDS = 1
)

type NsInformer struct {
	restConfig *rest.Config
	client     *kubernetes.Clientset
	logger     logs.Logger
	appConfig  utils.Config

	nsLister listers.NamespaceLister
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

	namespaceInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    n.onAddNamespace(ctx),
		UpdateFunc: n.onUpdateNamespace(ctx),
		DeleteFunc: func(interface{}) { return },
	})

	// start informer ->
	go informerFactory.Start(ctx.Done())
	// start to sync and call list
	if !cache.WaitForCacheSync(ctx.Done(), namespaceInformer.HasSynced) {
		return errors.New("timed out waiting for caches to sync")
	}

	go n.DeletionTicker(ctx)

	return nil
}

func (n *NsInformer) onAddNamespace(ctx context.Context) func(interface{}) {
	return func(obj interface{}) {
		namespace := obj.(*corev1.Namespace)
		if n.isWatched(namespace) {
			n.ensureAnnotated(ctx, namespace)
		}
	}
}

func (n *NsInformer) onUpdateNamespace(ctx context.Context) func(interface{}, interface{}) {
	return func(oldObj interface{}, newObj interface{}) {
		newNamespace := newObj.(*corev1.Namespace)

		if n.isWatched(newNamespace) {
			n.ensureAnnotated(ctx, newNamespace)
		}
	}
}

func (n *NsInformer) isWatched(namespace *corev1.Namespace) bool {
	isNameWatched := n.appConfig.DeletionRegexp.MatchString(namespace.Name)
	_, ok := namespace.Annotations[n.appConfig.NsPreserveAnnotation]
	return isNameWatched && !ok
}

func (n *NsInformer) ensureAnnotated(ctx context.Context, ns *corev1.Namespace) error {
	annotations := n.getNsAnnotations(ns)
	_, ok := annotations[n.appConfig.AnnotationKey]
	if !ok {
		createdAt := n.getNsCreationTimestamp(ns)
		decommissionTimestamp := n.shiftTimeStampByRetention(createdAt).UTC().Format(time.RFC3339)
		n.annotateRetention(ctx, ns, decommissionTimestamp)
		n.logger.Info("Namespace", ns.Name, "annotated for deletion", "deadline timestamp", decommissionTimestamp)
	}

	return nil
}

func (n *NsInformer) annotateRetention(ctx context.Context, ns *corev1.Namespace, annotationValue string) error {
	if ns.ObjectMeta.Annotations[n.appConfig.AnnotationKey] == annotationValue {
		return nil
	}

	newNs := ns.DeepCopy()
	annotations := newNs.ObjectMeta.Annotations

	if annotations == nil {
		annotations = make(map[string]string)
	}

	annotations[n.appConfig.AnnotationKey] = annotationValue

	newNs.ObjectMeta.Annotations = annotations

	updateOptions := metav1.UpdateOptions{}
	_, err := n.client.CoreV1().Namespaces().Update(ctx, newNs, updateOptions)

	if err != nil {
		n.logger.Error("Unable annotate", "namespace", ns.Name, "err", err)
	}
	return nil
}

func (n *NsInformer) DeletionTicker(ctx context.Context) {
	ticker := time.NewTicker(TICK_SECONDS * time.Second)
	mutex := new(sync.Mutex)
	for range ticker.C {
		tickTime := <-ticker.C
		if n.isAllowedWindow(tickTime) && mutex.TryLock() {
			n.logger.Info("Engaging in maintenance procedures", "Time", tickTime.UTC().Format(time.RFC822))
			watchedNamespaces, err := n.listWatchedNamespaces()
			if err != nil {
				n.logger.Error("Could not list watched namespaces for deletion", err)
			}
			if n.appConfig.PostponeDeletion && len(watchedNamespaces) > 0 {
				n.postponeDelOfActive(ctx, watchedNamespaces)
			}

			expiredNamespaces := n.filterExpiredNamespaces(watchedNamespaces)

			if len(expiredNamespaces) > 0 {
				n.logger.Info("Found expired namespaces", "count", len(expiredNamespaces))

				err = n.processExpiredNamespaces(ctx, expiredNamespaces)
				if err != nil {
					n.logger.Error("Could not process expired namespaces", err)
				}
			} else {
				n.logger.Info("Nothing to delete.")
				time.Sleep(time.Second * 1800)
			}

			mutex.Unlock()
		}
	}
	<-ctx.Done()
	n.logger.Info("Finishig deletion ticker...")
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

func (n *NsInformer) listWatchedNamespaces() (namespaces []*corev1.Namespace, err error) {
	watchedNamespaces := make([]*corev1.Namespace, 0)

	namespaces, err = n.nsLister.List(labels.Everything())
	if err != nil {
		return watchedNamespaces, errors.New("could not list namespaces")
	}

	for _, ns := range namespaces {
		if n.isWatched(ns) && ns.Annotations[n.appConfig.NsPreserveAnnotation] != "true" {
			watchedNamespaces = append(watchedNamespaces, ns)
		}

	}
	return watchedNamespaces, err

}

func (n *NsInformer) postponeDelOfActive(ctx context.Context, watchedNamespaces []*corev1.Namespace) error {
	for _, ns := range watchedNamespaces {

		nsReleases, _ := n.listNamespaceReleases(ns)
		if len(nsReleases) <= 0 {
			continue
		}

		nsDeletionTs, _ := n.getNsDeletionTimespamp(ns)
		nsCreationTs := n.getNsCreationTimestamp(ns)
		latestRelease := n.latestDeployedRelease(nsReleases)

		latestDeployTs := latestRelease.Info.LastDeployed.UTC().Time
		if latestDeployTs.After(nsCreationTs) && latestDeployTs.Before(nsDeletionTs) {
			newRetention := n.shiftTimeStampByRetention(latestDeployTs).UTC().Format(time.RFC3339)
			n.annotateRetention(ctx, ns, newRetention)
		}
	}
	return nil
}

func (n *NsInformer) latestDeployedRelease(releases []*release.Release) *release.Release {
	latest := releases[0]
	for _, release := range releases {

		if release.Info.LastDeployed.After(latest.Info.LastDeployed) {
			latest = release
		}
	}

	return latest
}

func (n *NsInformer) filterExpiredNamespaces(watchedNamespaces []*corev1.Namespace) (expiredNamespaces []*corev1.Namespace) {
	timeNow := time.Now().UTC()

	for _, ns := range watchedNamespaces {
		nsDeletionTimespamp, err := n.getNsDeletionTimespamp(ns)
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

func (n *NsInformer) getNsDeletionTimespamp(namespace *corev1.Namespace) (time.Time, error) {
	timeStampAnnotation := namespace.Annotations[n.appConfig.AnnotationKey]
	nsDeletionTimespamp, err := time.Parse(RFC3339local, timeStampAnnotation)

	return nsDeletionTimespamp, err
}

func (n *NsInformer) getNsCreationTimestamp(ns *corev1.Namespace) time.Time {
	return ns.ObjectMeta.CreationTimestamp.Time
}

func (n *NsInformer) getNsAnnotations(ns *corev1.Namespace) map[string]string {
	return ns.ObjectMeta.Annotations
}

func (n *NsInformer) shiftTimeStampByRetention(timestamp time.Time) time.Time {

	retentionDays := n.appConfig.RetentionDays
	retentionHours := n.appConfig.RetentionHours

	timeoutDays := time.Duration(retentionDays)
	shiftedTs := timestamp.Add(time.Hour * 24 * timeoutDays)

	if retentionHours > 0 {
		timeoutHours := time.Duration(retentionHours)
		shiftedTs = shiftedTs.Add(time.Hour * timeoutHours)
	}

	return shiftedTs

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

		if n.appConfig.IsUninstallReleases {
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

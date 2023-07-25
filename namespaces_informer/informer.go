package namespaces_informer

import (
	"NaNameUz3r/ReviewReaper/logs"
	"NaNameUz3r/ReviewReaper/utils"
	"context"
	"errors"
	"sync"
	"time"

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

const (
	HH_MM          = "15:04"
	RFC3339local   = "2006-01-02T15:04:05Z"
	TICK_SECONDS   = 5
	RESYNC_TIMEOUT = 30 * time.Second
)

type NsInformer struct {
	restConfig *rest.Config
	client     *kubernetes.Clientset
	logger     logs.Logger
	appConfig  utils.Config

	nsLister listers.NamespaceLister
}

func NewNsInformer(
	restConfig *rest.Config,
	client *kubernetes.Clientset,
	logger logs.Logger,
	appConfig utils.Config,
) *NsInformer {
	return &NsInformer{
		restConfig: restConfig,
		client:     client,
		logger:     logger,
		appConfig:  appConfig,
	}
}

func (n *NsInformer) Run(ctx context.Context) error {
	informerFactory := informers.NewSharedInformerFactory(n.client, RESYNC_TIMEOUT)

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
		err := errors.New("Timeout occurred while waiting for caches to synchronize")
		n.logger.Error("Error waiting for caches to sync", err)
		return err
	}

	go n.DeletionTicker(ctx)

	return nil
}

func (n *NsInformer) onAddNamespace(ctx context.Context) func(interface{}) {
	return func(obj interface{}) {
		namespace := obj.(*corev1.Namespace)
		if n.isWatched(namespace) {
			err := n.ensureAnnotated(ctx, namespace)
			if err != nil {
				n.logger.Error("Error occurred while ensuring annotation of new namespace", namespace.Name, err)
			}
		}
	}
}

func (n *NsInformer) onUpdateNamespace(ctx context.Context) func(interface{}, interface{}) {
	return func(oldObj interface{}, newObj interface{}) {
		newNamespace := newObj.(*corev1.Namespace)

		if n.isWatched(newNamespace) {
			err := n.ensureAnnotated(ctx, newNamespace)
			if err != nil {
				n.logger.Error("Error occurred while ensuring annotation of updated namespace", newNamespace.Name, err)
			}
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
		err := n.annotateRetention(ctx, ns, decommissionTimestamp)
		if err != nil {
			return err
		}
		n.logger.Info(
			"Annotated for deletion",
			"NsName",
			ns.Name,
			"DeletionTimestamp",
			decommissionTimestamp,
		)
	}

	return nil
}

func (n *NsInformer) annotateRetention(
	ctx context.Context,
	ns *corev1.Namespace,
	annotationValue string,
) error {
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
		n.logger.Error("Unable to annotate", "NsName", ns.Name, "ERROR:", err)
		return err
	}
	return nil
}

func (n *NsInformer) DeletionTicker(ctx context.Context) {
	ticker := time.NewTicker(TICK_SECONDS * time.Second)
	mutex := new(sync.Mutex)
	mtInProgress := false
	for range ticker.C {
		if mtInProgress {
			continue
		}

		if n.isNowAllowed() {
			mutex.Lock()
			mtInProgress = true
			n.logger.Info(
				"Beginning scheduled maintenance iteration",
				"At",
				time.Now().UTC().Format(time.RFC822),
			)
			watchedNamespaces, err := n.listWatchedNamespaces()
			if err != nil {
				n.logger.Error("Could not list watched namespaces", err)
			}
			if n.appConfig.PostponeDeletion && len(watchedNamespaces) > 0 {
				isPostponed, err := n.postponeDelOfActive(ctx, watchedNamespaces)
				if err != nil {
					n.logger.Error("Failed while checking watched namespaces for helm release activeness", err)
				}
				if isPostponed {
					n.logger.Info("Need to re-sync watched namespaces list, to fetch possible changes of deletion postponer...")
					time.Sleep(RESYNC_TIMEOUT)
					time.Sleep(time.Second * 5)
					watchedNamespaces, err = n.listWatchedNamespaces()
					if err != nil {
						n.logger.Error("Could not list watched namespaces", err)
					}
				}
			}

			expiredNamespaces := n.filterExpiredNamespaces(watchedNamespaces)

			if len(expiredNamespaces) > 0 {
				n.logger.Info("Found expired namespaces", "Count", len(expiredNamespaces))

				err = n.processExpiredNamespaces(ctx, expiredNamespaces)
				if err != nil {
					n.logger.Error("Could not process expired namespaces", err)
				}
			} else {
				n.logger.Info("Nothing to delete.")
				n.logger.Info("Taking a nap for 15 minutes...")
				time.Sleep(time.Minute * 15)
			}
			mutex.Unlock()
			mtInProgress = false
		} else {
			n.logger.Info("Seems that maintenance window is over...")
			sleepFor := n.durationUntilMaintenance()
			n.logger.Info("Taking a nap until next maintenance window", "SleepFor", sleepFor)
			time.Sleep(sleepFor)
		}
	}
	<-ctx.Done()
	n.logger.Info("Finishig deletion ticker...")
}

func (n *NsInformer) isNowAllowed() bool {
	timeNow := time.Now().UTC()
	isAllowed := false

	if n.isTodayAllowed(timeNow) && n.isTimeNowAllowed(timeNow) {
		isAllowed = true
	}

	return isAllowed
}

func (n *NsInformer) isTodayAllowed(t time.Time) bool {
	todayWeekday := t.UTC().Weekday().String()[0:3]
	weekdayOk := utils.IsContains(n.appConfig.DeletionWindow.WeekDays, todayWeekday)
	return weekdayOk
}

func (n *NsInformer) isTimeNowAllowed(t time.Time) bool {
	isAllowed := false
	nbCfg, _ := time.Parse(HH_MM, n.appConfig.DeletionWindow.NotBefore)
	naCfg, _ := time.Parse(HH_MM, n.appConfig.DeletionWindow.NotAfter)

	notBefore := time.Date(
		t.Year(),
		t.Month(),
		t.Day(),
		nbCfg.Hour(),
		nbCfg.Minute(),
		0,
		0,
		time.UTC,
	)
	notAfter := time.Date(
		t.Year(),
		t.Month(),
		t.Day(),
		naCfg.Hour(),
		naCfg.Minute(),
		0,
		0,
		time.UTC,
	)

	if t.After(notBefore) && t.Before(notAfter) {
		isAllowed = true
	}
	return isAllowed
}

func (n *NsInformer) isMaintenanceToday(t time.Time) bool {
	if !n.isTodayAllowed(t) {
		return false
	}
	nbCfg, _ := time.Parse(HH_MM, n.appConfig.DeletionWindow.NotBefore)
	nowHHMM, _ := time.Parse(HH_MM, t.Format(HH_MM))

	isLater := nowHHMM.UTC().Before(nbCfg.UTC())
	return isLater
}

func (n *NsInformer) durationUntilMaintenance() time.Duration {
	now := time.Now().UTC()
	nextMaintenanceTime := n.getNextMaintenanceTime(now)
	timeDifference := time.Until(time.Unix(nextMaintenanceTime.Unix(), 0))
	return timeDifference
}

func (n *NsInformer) isTodayLastDayOfTheMonth(t time.Time) bool {
	today := t.UTC()
	tomorrow := today.AddDate(0, 0, 1)
	if tomorrow.Month() != today.Month() {
		return true
	}
	return false
}

func (n *NsInformer) isTodayLastDayOfTheYear(t time.Time) bool {
	today := t.UTC()
	tomorrow := today.AddDate(0, 0, 1)
	if tomorrow.Year() != today.Year() {
		return true
	}
	return false
}

func (n *NsInformer) getNextMaintenanceTime(now time.Time) time.Time {
	currentMonth := now.Month()
	currentYear := now.Year()
	var nextDate time.Time

	nbCfg, _ := time.Parse(HH_MM, n.appConfig.DeletionWindow.NotBefore)
	n.logger.Info("Seeking next allowed maintenance window")

	if n.isMaintenanceToday(now) {
		nextDate = time.Date(
			currentYear,
			currentMonth,
			int(now.Day()),
			nbCfg.Hour(),
			nbCfg.Minute(),
			0,
			0, time.UTC)
		n.logger.Info("Maintenanse will be today later")
	} else {
		nextDate = time.Date(
			currentYear,
			currentMonth,
			int(now.Day())+1,
			nbCfg.Hour(),
			nbCfg.Minute(),
			0,
			0, time.UTC)
		n.logger.Info("No maintenance window today, will check tomorrow")
	}

	if n.isTodayLastDayOfTheMonth(now) {
		nextDate.AddDate(0, 1, 0)
	}

	if n.isTodayLastDayOfTheMonth(now) {
		nextDate.AddDate(1, 0, 0)
	}

	return nextDate
}

func (n *NsInformer) listWatchedNamespaces() (namespaces []*corev1.Namespace, err error) {
	watchedNamespaces := make([]*corev1.Namespace, 0)

	namespaces, err = n.nsLister.List(labels.Everything())
	if err != nil {
		return watchedNamespaces, err
	}

	for _, ns := range namespaces {
		if n.isWatched(ns) && ns.Annotations[n.appConfig.NsPreserveAnnotation] != "true" {
			watchedNamespaces = append(watchedNamespaces, ns)
		}
	}
	return watchedNamespaces, err
}

func (n *NsInformer) postponeDelOfActive(
	ctx context.Context,
	watchedNamespaces []*corev1.Namespace,
) (bool, error) {
	isSomethingPostponed := false
	n.logger.Info(
		"Comparing the timestamps of the last deployed Helm releases in the watched namespaces with the deletion timestamps of these namespaces",
	)

	for _, ns := range watchedNamespaces {

		nsReleases, err := n.listNamespaceReleases(ns)
		if err != nil {
			return false, err
		}
		if len(nsReleases) <= 0 {
			continue
		}

		nsDeletionTs, _ := n.getNsDeletionTimespamp(ns)
		latestRelease := n.latestDeployedRelease(nsReleases)

		latestDeployTs := latestRelease.Info.LastDeployed.UTC().Time
		considerDeletionTs := latestDeployTs.AddDate(0, 0, n.appConfig.RetentionDays)
		considerDeletionTs.Add(time.Hour * time.Duration(n.appConfig.RetentionHours))

		truncatedNsDeletionTs := nsDeletionTs.Truncate(time.Second)
		truncatedConsiderDeletionTs := considerDeletionTs.Truncate(time.Second)

		if truncatedNsDeletionTs.Equal(truncatedConsiderDeletionTs) {
			n.logger.Info(
				"namespace",
				ns.Name,
				"deletion scheduled correctly.",
				"Deletion timestamp is",
				nsDeletionTs,
			)
			continue
		}

		if nsDeletionTs.Before(considerDeletionTs) {
			newRetention := considerDeletionTs.Format(time.RFC3339)
			n.annotateRetention(ctx, ns, newRetention)
			n.logger.Info("namespace", ns.Name, "deletion postponed", "for", newRetention)
			isSomethingPostponed = true
		}
	}
	return isSomethingPostponed, nil
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

func (n *NsInformer) filterExpiredNamespaces(
	watchedNamespaces []*corev1.Namespace,
) (expiredNamespaces []*corev1.Namespace) {
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

func (n *NsInformer) processExpiredNamespaces(
	ctx context.Context,
	namespaces []*corev1.Namespace,
) error {
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
			if n.appConfig.DryRun {
				n.logger.Info("[DRY-RUN] want to uininstall releases from", "namespace", ns.Name)
			} else {
				releases, err := n.listNamespaceReleases(ns)
				if err != nil {
					n.logger.Error("Could not list releases", "namespace", ns.Name)
				}
				n.deleteNamespaceReleases(releases, ns)
			}
		}

		if n.appConfig.DryRun {
			n.logger.Info("[DRY-RUN] want to delete", "namespace", ns.Name)
			continue
		} else {
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
	}
	return nil
}

func (n *NsInformer) listNamespaceReleases(
	namespace *corev1.Namespace,
) ([]*release.Release, error) {
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

func (n *NsInformer) deleteNamespaceReleases(
	releases []*release.Release,
	namespace *corev1.Namespace,
) error {
	// TODO: is there some way to catch error?
	settings := cli.New()
	settings.SetNamespace(namespace.Name)
	actionConfig := new(action.Configuration)

	if err := actionConfig.Init(settings.RESTClientGetter(), settings.Namespace(), "secret", n.logger.Debug); err != nil {
		n.logger.Error("Failed to set up helm action config", err)
		panic(err)
	}
	deleteAction := action.NewUninstall(actionConfig)
	deleteAction.DisableHooks = false

	wg := &sync.WaitGroup{}

	for _, r := range releases {
		wg.Add(1)
		go func(r *release.Release, wg *sync.WaitGroup) {
			deleteAction.Run(r.Name)
			n.logger.Info(
				"Uninstalling helm release",
				"name",
				r.Name,
				"from namespace",
				namespace.Name,
			)
			wg.Done()
		}(r, wg)
	}
	wg.Wait()

	return nil
}

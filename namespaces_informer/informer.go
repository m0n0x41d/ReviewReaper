package namespaces_informer

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
)

func RunNsInformer(client *dynamic.DynamicClient) {

	resource := schema.GroupVersionResource{Group: "core", Version: "v1", Resource: "namespaces"}
	factory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(client, time.Minute, corev1.NamespaceAll, nil)
	informer := factory.ForResource(resource).Informer()

	mux := &sync.RWMutex{}
	synced := false

	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			mux.RLock()
			defer mux.RUnlock()
			if !synced {
				return
			}

			fmt.Println("Check-check")
		},
	})

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	go informer.Run(ctx.Done())

	isSynced := cache.WaitForCacheSync(ctx.Done(), informer.HasSynced)
	mux.Lock()
	synced = isSynced
	mux.Unlock()

	if !isSynced {
		logrus.Fatal("Could not sync resources")
	}

	<-ctx.Done()
}

package k8s

import (
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/pkg/k8s/clientset/versioned"
	"netguard-pg-backend/pkg/k8s/informers/externalversions"

	"context"
	"log"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

// ExampleUsage демонстрирует использование сгенерированного кода
func ExampleUsage() error {
	// Создание клиента
	config, err := rest.InClusterConfig()
	if err != nil {
		return err
	}

	client, err := versioned.NewForConfig(config)
	if err != nil {
		return err
	}

	// Использование клиента для получения Services
	_, err = client.NetguardV1beta1().Services("default").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}

	// Создание информера
	informerFactory := externalversions.NewSharedInformerFactory(client, time.Minute)
	serviceInformer := informerFactory.Netguard().V1beta1().Services()

	// Создание листера
	serviceLister := serviceInformer.Lister()

	// Добавление обработчика событий
	serviceInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			service := obj.(*netguardv1beta1.Service)
			log.Printf("Service added: %s", service.Name)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			service := newObj.(*netguardv1beta1.Service)
			log.Printf("Service updated: %s", service.Name)
		},
		DeleteFunc: func(obj interface{}) {
			service := obj.(*netguardv1beta1.Service)
			log.Printf("Service deleted: %s", service.Name)
		},
	})

	// Запуск информера
	ctx := context.Background()
	informerFactory.Start(ctx.Done())
	informerFactory.WaitForCacheSync(ctx.Done())

	// Использование листера для получения данных из кеша
	_, err = serviceLister.Services("default").List(labels.Everything())
	if err != nil {
		return err
	}

	return nil
}

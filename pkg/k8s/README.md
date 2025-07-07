# Сгенерированный клиентский код для Kubernetes API

Этот пакет содержит автоматически сгенерированный код для работы с Netguard API типами в Kubernetes.

## Структура

- **`clientset/`** - Клиентский код для прямого взаимодействия с API
- **`informers/`** - Информеры для отслеживания изменений объектов в реальном времени
- **`listers/`** - Листеры для эффективного чтения данных из локального кеша

## Генерация кода

Для регенерации кода используйте:

```bash
bash hack/k8s/update-codegen.sh
```

## Пример использования

```go
// Создание клиента
config, err := rest.InClusterConfig()
client, err := versioned.NewForConfig(config)

// Получение списка Services
services, err := client.NetguardV1beta1().Services("default").List(context.TODO(), metav1.ListOptions{})

// Создание информера
informerFactory := externalversions.NewSharedInformerFactory(client, time.Minute)
serviceInformer := informerFactory.Netguard().V1beta1().Services()

// Использование листера
serviceLister := serviceInformer.Lister()
services, err := serviceLister.Services("default").List(labels.Everything())
```

## API Группы

- **Группа**: `netguard.sgroups.io`
- **Версия**: `v1beta1`

## Типы ресурсов

- `Service` - Сетевые сервисы с портами и протоколами
- `AddressGroup` - Группы сетевых адресов
- `AddressGroupBinding` - Привязки адресных групп к сервисам
- `AddressGroupPortMapping` - Маппинги портов для адресных групп
- `RuleS2S` - Правила взаимодействия сервис-к-сервису
- `ServiceAlias` - Алиасы для сервисов
- `AddressGroupBindingPolicy` - Политики привязки адресных групп
- `IEAgAgRule` - Правила ingress/egress между адресными группами

## Примечания

- Код генерируется автоматически из типов в `internal/k8s/apis/netguard/v1beta1/`
- Не редактируйте сгенерированные файлы вручную - они будут перезаписаны
- Используйте теги `+genclient` в типах для включения их в генерацию клиентского кода 
/*
Copyright 2024 The Netguard Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package apiserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/klog/v2"

	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/k8s/client"
	"netguard-pg-backend/internal/k8s/registry/addressgroup"
	"netguard-pg-backend/internal/k8s/registry/addressgroupbinding"
	"netguard-pg-backend/internal/k8s/registry/addressgroupbindingpolicy"
	"netguard-pg-backend/internal/k8s/registry/addressgroupportmapping"
	"netguard-pg-backend/internal/k8s/registry/ieagagrule"
	"netguard-pg-backend/internal/k8s/registry/rules2s"
	"netguard-pg-backend/internal/k8s/registry/service"
	"netguard-pg-backend/internal/k8s/registry/servicealias"
)

// SimpleAPIServer - простой HTTP сервер с CRUD операциями для всех ресурсов
type SimpleAPIServer struct {
	server        *http.Server
	backendClient client.BackendClient
	config        APIServerConfig

	// Storage instances для всех ресурсов
	serviceStorage                   *service.ServiceStorage
	addressGroupStorage              *addressgroup.AddressGroupStorage
	addressGroupBindingStorage       *addressgroupbinding.AddressGroupBindingStorage
	addressGroupPortMappingStorage   *addressgroupportmapping.AddressGroupPortMappingStorage
	ruleS2SStorage                   *rules2s.RuleS2SStorage
	serviceAliasStorage              *servicealias.ServiceAliasStorage
	addressGroupBindingPolicyStorage *addressgroupbindingpolicy.AddressGroupBindingPolicyStorage
	ieAgAgRuleStorage                *ieagagrule.IEAgAgRuleStorage
}

// NewSimpleAPIServer создает простой API сервер с поддержкой TLS
func NewSimpleAPIServer(config APIServerConfig, backendClient client.BackendClient) (*SimpleAPIServer, error) {
	mux := http.NewServeMux()

	s := &SimpleAPIServer{
		backendClient: backendClient,
		config:        config,
		// Создаем все storage instances как в старом server.go
		serviceStorage:                   service.NewServiceStorage(backendClient),
		addressGroupStorage:              addressgroup.NewAddressGroupStorage(backendClient),
		addressGroupBindingStorage:       addressgroupbinding.NewAddressGroupBindingStorage(backendClient),
		addressGroupPortMappingStorage:   addressgroupportmapping.NewAddressGroupPortMappingStorage(backendClient),
		ruleS2SStorage:                   rules2s.NewRuleS2SStorage(backendClient),
		serviceAliasStorage:              servicealias.NewServiceAliasStorage(backendClient),
		addressGroupBindingPolicyStorage: addressgroupbindingpolicy.NewAddressGroupBindingPolicyStorage(backendClient),
		ieAgAgRuleStorage:                ieagagrule.NewIEAgAgRuleStorage(backendClient),
	}

	// Регистрируем основные endpoints
	mux.HandleFunc("/", s.handleRoot)
	mux.HandleFunc("/apis", s.handleAPIs)
	mux.HandleFunc("/apis/", s.handleAPIGroups)
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/readyz", s.handleReadyz)
	mux.HandleFunc("/livez", s.handleLivez)

	// Определяем адрес и порт
	var addr string
	if config.IsTLSEnabled() {
		addr = fmt.Sprintf("%s:%d", config.BindAddress, config.SecurePort)
	} else {
		addr = fmt.Sprintf("%s:%d", config.BindAddress, config.InsecurePort)
	}

	// Создаем HTTP сервер
	s.server = &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	// Настраиваем TLS если включен
	if config.IsTLSEnabled() {
		tlsConfig, err := config.GetTLSConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to get TLS config: %w", err)
		}
		s.server.TLSConfig = tlsConfig
	}

	return s, nil
}

// Start запускает сервер с поддержкой TLS
func (s *SimpleAPIServer) Start(ctx context.Context) error {
	if s.config.IsTLSEnabled() {
		klog.Infof("Starting HTTPS API server on %s", s.server.Addr)
	} else {
		klog.Infof("Starting HTTP API server on %s", s.server.Addr)
	}

	go func() {
		<-ctx.Done()
		klog.Info("Shutting down simple API server...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		s.server.Shutdown(shutdownCtx)
	}()

	var err error
	if s.config.IsTLSEnabled() {
		err = s.server.ListenAndServeTLS("", "") // Сертификаты уже в TLSConfig
	} else {
		err = s.server.ListenAndServe()
	}

	if err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("failed to start server: %w", err)
	}

	return nil
}

// handleRoot обрабатывает корневой путь
func (s *SimpleAPIServer) handleRoot(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"paths": []string{
			"/apis",
			"/apis/netguard.sgroups.io",
			"/apis/netguard.sgroups.io/v1beta1",
			"/healthz",
			"/livez",
			"/readyz",
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleAPIs обрабатывает /apis
func (s *SimpleAPIServer) handleAPIs(w http.ResponseWriter, r *http.Request) {
	response := &metav1.APIGroupList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "APIGroupList",
			APIVersion: "v1",
		},
		Groups: []metav1.APIGroup{
			{
				Name: "netguard.sgroups.io",
				Versions: []metav1.GroupVersionForDiscovery{
					{
						GroupVersion: "netguard.sgroups.io/v1beta1",
						Version:      "v1beta1",
					},
				},
				PreferredVersion: metav1.GroupVersionForDiscovery{
					GroupVersion: "netguard.sgroups.io/v1beta1",
					Version:      "v1beta1",
				},
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleAPIGroups обрабатывает /apis/*
func (s *SimpleAPIServer) handleAPIGroups(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/apis/netguard.sgroups.io" {
		s.handleNetguardAPIGroup(w, r)
		return
	}

	if r.URL.Path == "/apis/netguard.sgroups.io/v1beta1" {
		s.handleNetguardV1Beta1(w, r)
		return
	}

	// Обработка namespaced ресурсов
	if strings.HasPrefix(r.URL.Path, "/apis/netguard.sgroups.io/v1beta1/namespaces/") {
		s.handleNamespacedResources(w, r)
		return
	}

	http.NotFound(w, r)
}

// handleNetguardAPIGroup обрабатывает /apis/netguard.sgroups.io
func (s *SimpleAPIServer) handleNetguardAPIGroup(w http.ResponseWriter, r *http.Request) {
	response := &metav1.APIGroup{
		TypeMeta: metav1.TypeMeta{
			Kind:       "APIGroup",
			APIVersion: "v1",
		},
		Name: "netguard.sgroups.io",
		Versions: []metav1.GroupVersionForDiscovery{
			{
				GroupVersion: "netguard.sgroups.io/v1beta1",
				Version:      "v1beta1",
			},
		},
		PreferredVersion: metav1.GroupVersionForDiscovery{
			GroupVersion: "netguard.sgroups.io/v1beta1",
			Version:      "v1beta1",
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleNetguardV1Beta1 обрабатывает /apis/netguard.sgroups.io/v1beta1
func (s *SimpleAPIServer) handleNetguardV1Beta1(w http.ResponseWriter, r *http.Request) {
	response := &metav1.APIResourceList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "APIResourceList",
			APIVersion: "v1",
		},
		GroupVersion: "netguard.sgroups.io/v1beta1",
		APIResources: []metav1.APIResource{
			{
				Name:         "services",
				SingularName: "service",
				Namespaced:   true,
				Kind:         "Service",
				Verbs:        []string{"get", "list", "create", "update", "patch", "delete", "watch"},
			},
			{
				Name:         "addressgroups",
				SingularName: "addressgroup",
				Namespaced:   true,
				Kind:         "AddressGroup",
				Verbs:        []string{"get", "list", "create", "update", "patch", "delete", "watch"},
			},
			{
				Name:         "addressgroupbindings",
				SingularName: "addressgroupbinding",
				Namespaced:   true,
				Kind:         "AddressGroupBinding",
				Verbs:        []string{"get", "list", "create", "update", "patch", "delete", "watch"},
			},
			{
				Name:         "rules2s",
				SingularName: "rules2s",
				Namespaced:   true,
				Kind:         "RuleS2S",
				Verbs:        []string{"get", "list", "create", "update", "patch", "delete", "watch"},
			},
			{
				Name:         "servicealiases",
				SingularName: "servicealias",
				Namespaced:   true,
				Kind:         "ServiceAlias",
				Verbs:        []string{"get", "list", "create", "update", "patch", "delete", "watch"},
			},
			{
				Name:         "addressgroupbindingpolicies",
				SingularName: "addressgroupbindingpolicy",
				Namespaced:   true,
				Kind:         "AddressGroupBindingPolicy",
				Verbs:        []string{"get", "list", "create", "update", "patch", "delete", "watch"},
			},
			{
				Name:         "ieagagrules",
				SingularName: "ieagagrule",
				Namespaced:   true,
				Kind:         "IEAgAgRule",
				Verbs:        []string{"get", "list", "create", "update", "patch", "delete", "watch"},
			},
			{
				Name:         "addressgroupportmappings",
				SingularName: "addressgroupportmapping",
				Namespaced:   true,
				Kind:         "AddressGroupPortMapping",
				Verbs:        []string{"get", "list", "create", "update", "patch", "delete", "watch"},
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleNamespacedResources обрабатывает CRUD операции для всех namespaced ресурсов
func (s *SimpleAPIServer) handleNamespacedResources(w http.ResponseWriter, r *http.Request) {
	// Парсим путь: /apis/netguard.sgroups.io/v1beta1/namespaces/{namespace}/{resource}[/{name}]
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/apis/netguard.sgroups.io/v1beta1/namespaces/"), "/")

	if len(pathParts) < 2 {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	namespace := pathParts[0]
	resource := pathParts[1]
	var name string
	if len(pathParts) > 2 {
		name = pathParts[2]
	}

	klog.Infof("Handling %s request for resource=%s, namespace=%s, name=%s", r.Method, resource, namespace, name)

	// Добавляем namespace в context
	ctx := context.WithValue(r.Context(), "namespace", namespace)

	// Маршрутизация по ресурсам
	switch resource {
	case "services":
		s.handleResourceCRUD(w, r.WithContext(ctx), name, s.serviceStorage, &netguardv1beta1.Service{})
	case "addressgroups":
		s.handleResourceCRUD(w, r.WithContext(ctx), name, s.addressGroupStorage, &netguardv1beta1.AddressGroup{})
	case "addressgroupbindings":
		s.handleResourceCRUD(w, r.WithContext(ctx), name, s.addressGroupBindingStorage, &netguardv1beta1.AddressGroupBinding{})
	case "rules2s":
		s.handleResourceCRUD(w, r.WithContext(ctx), name, s.ruleS2SStorage, &netguardv1beta1.RuleS2S{})
	case "servicealiases":
		s.handleResourceCRUD(w, r.WithContext(ctx), name, s.serviceAliasStorage, &netguardv1beta1.ServiceAlias{})
	case "addressgroupbindingpolicies":
		s.handleResourceCRUD(w, r.WithContext(ctx), name, s.addressGroupBindingPolicyStorage, &netguardv1beta1.AddressGroupBindingPolicy{})
	case "ieagagrules":
		s.handleResourceCRUD(w, r.WithContext(ctx), name, s.ieAgAgRuleStorage, &netguardv1beta1.IEAgAgRule{})
	case "addressgroupportmappings":
		s.handleResourceCRUD(w, r.WithContext(ctx), name, s.addressGroupPortMappingStorage, &netguardv1beta1.AddressGroupPortMapping{})
	default:
		http.Error(w, fmt.Sprintf("Resource %s not found", resource), http.StatusNotFound)
	}
}

// ResourceStorage интерфейс для всех storage implementations
type ResourceStorage interface {
	Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error)
	List(ctx context.Context, options *metainternalversion.ListOptions) (runtime.Object, error)
	Create(ctx context.Context, obj runtime.Object, createValidation rest.ValidateObjectFunc, options *metav1.CreateOptions) (runtime.Object, error)
	Update(ctx context.Context, name string, objInfo rest.UpdatedObjectInfo, createValidation rest.ValidateObjectFunc, updateValidation rest.ValidateObjectUpdateFunc, forceAllowCreate bool, options *metav1.UpdateOptions) (runtime.Object, bool, error)
	Delete(ctx context.Context, name string, deleteValidation rest.ValidateObjectFunc, options *metav1.DeleteOptions) (runtime.Object, bool, error)
}

// UpdatedObjectInfo интерфейс для update operations

// handleResourceCRUD - универсальный обработчик CRUD операций для любого ресурса
func (s *SimpleAPIServer) handleResourceCRUD(w http.ResponseWriter, r *http.Request, name string, storage ResourceStorage, objTemplate runtime.Object) {
	switch r.Method {
	case http.MethodGet:
		if name == "" {
			// LIST resources
			s.handleResourceList(w, r, storage)
		} else {
			// GET specific resource
			s.handleResourceGet(w, r, name, storage)
		}
	case http.MethodPost:
		// CREATE resource
		s.handleResourceCreate(w, r, storage, objTemplate)
	case http.MethodPut:
		// UPDATE resource
		s.handleResourceUpdate(w, r, name, storage, objTemplate)
	case http.MethodDelete:
		// DELETE resource
		s.handleResourceDelete(w, r, name, storage)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleResourceList обрабатывает LIST операцию для любого ресурса
func (s *SimpleAPIServer) handleResourceList(w http.ResponseWriter, r *http.Request, storage ResourceStorage) {
	listOptions := &metainternalversion.ListOptions{}

	obj, err := storage.List(r.Context(), listOptions)
	if err != nil {
		klog.Errorf("Failed to list resources: %v", err)
		http.Error(w, fmt.Sprintf("Failed to list resources: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(obj); err != nil {
		klog.Errorf("Failed to encode response: %v", err)
	}
}

// handleResourceGet обрабатывает GET операцию для конкретного ресурса
func (s *SimpleAPIServer) handleResourceGet(w http.ResponseWriter, r *http.Request, name string, storage ResourceStorage) {
	getOptions := &metav1.GetOptions{}

	obj, err := storage.Get(r.Context(), name, getOptions)
	if err != nil {
		klog.Errorf("Failed to get resource %s: %v", name, err)
		http.Error(w, fmt.Sprintf("Resource %s not found: %v", name, err), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(obj); err != nil {
		klog.Errorf("Failed to encode response: %v", err)
	}
}

// handleResourceCreate обрабатывает CREATE операцию для любого ресурса
func (s *SimpleAPIServer) handleResourceCreate(w http.ResponseWriter, r *http.Request, storage ResourceStorage, objTemplate runtime.Object) {
	// Создаем новый объект того же типа что и template
	obj := objTemplate.DeepCopyObject()

	if err := json.NewDecoder(r.Body).Decode(obj); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	createOptions := &metav1.CreateOptions{}

	createdObj, err := storage.Create(r.Context(), obj, nil, createOptions)
	if err != nil {
		klog.Errorf("Failed to create resource: %v", err)
		http.Error(w, fmt.Sprintf("Failed to create resource: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(createdObj); err != nil {
		klog.Errorf("Failed to encode response: %v", err)
	}
}

// handleResourceUpdate обрабатывает UPDATE операцию для любого ресурса
func (s *SimpleAPIServer) handleResourceUpdate(w http.ResponseWriter, r *http.Request, name string, storage ResourceStorage, objTemplate runtime.Object) {
	// Создаем новый объект того же типа что и template
	obj := objTemplate.DeepCopyObject()

	if err := json.NewDecoder(r.Body).Decode(obj); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	// Ensure name matches URL
	if metaObj, ok := obj.(metav1.Object); ok {
		metaObj.SetName(name)
	}

	updateOptions := &metav1.UpdateOptions{}

	// Create updatedObjectInfo
	updatedObjectInfo := &simpleUpdatedObjectInfo{obj: obj}

	updatedObj, created, err := storage.Update(r.Context(), name, updatedObjectInfo, nil, nil, false, updateOptions)
	if err != nil {
		klog.Errorf("Failed to update resource %s: %v", name, err)
		http.Error(w, fmt.Sprintf("Failed to update resource: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if created {
		w.WriteHeader(http.StatusCreated)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	if err := json.NewEncoder(w).Encode(updatedObj); err != nil {
		klog.Errorf("Failed to encode response: %v", err)
	}
}

// handleResourceDelete обрабатывает DELETE операцию для любого ресурса
func (s *SimpleAPIServer) handleResourceDelete(w http.ResponseWriter, r *http.Request, name string, storage ResourceStorage) {
	deleteOptions := &metav1.DeleteOptions{}

	obj, immediate, err := storage.Delete(r.Context(), name, nil, deleteOptions)
	if err != nil {
		klog.Errorf("Failed to delete resource %s: %v", name, err)
		http.Error(w, fmt.Sprintf("Failed to delete resource: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if immediate {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusAccepted)
	}
	if err := json.NewEncoder(w).Encode(obj); err != nil {
		klog.Errorf("Failed to encode response: %v", err)
	}
}

// simpleUpdatedObjectInfo implements rest.UpdatedObjectInfo
type simpleUpdatedObjectInfo struct {
	obj runtime.Object
}

func (u *simpleUpdatedObjectInfo) Preconditions() *metav1.Preconditions {
	return nil
}

func (u *simpleUpdatedObjectInfo) UpdatedObject(ctx context.Context, oldObj runtime.Object) (runtime.Object, error) {
	return u.obj, nil
}

// handleHealthz обрабатывает проверку здоровья
func (s *SimpleAPIServer) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

// handleReadyz обрабатывает проверку готовности
func (s *SimpleAPIServer) handleReadyz(w http.ResponseWriter, r *http.Request) {
	// Проверяем подключение к backend
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := s.backendClient.HealthCheck(ctx); err != nil {
		klog.Errorf("Backend health check failed: %v", err)
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("backend not ready"))
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

// handleLivez обрабатывает проверку живучести
func (s *SimpleAPIServer) handleLivez(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

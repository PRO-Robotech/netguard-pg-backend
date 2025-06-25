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
	"fmt"
	"net"
	"net/http"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/authorization/authorizer"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
	genericapiserver "k8s.io/apiserver/pkg/server"
	genericoptions "k8s.io/apiserver/pkg/server/options"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
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
	clientscheme "netguard-pg-backend/pkg/k8s/clientset/versioned/scheme"
)

// NetguardAPIServer wraps the generic API server
type NetguardAPIServer struct {
	GenericAPIServer *genericapiserver.GenericAPIServer
	backendClient    client.BackendClient
}

// ExtraConfig holds custom apiserver config
type ExtraConfig struct {
	BackendClient client.BackendClient
}

// Config defines the config for the apiserver
type Config struct {
	GenericConfig *genericapiserver.RecommendedConfig
	ExtraConfig   ExtraConfig
}

type completedConfig struct {
	GenericConfig genericapiserver.CompletedConfig
	ExtraConfig   *ExtraConfig
}

// CompletedConfig embeds a private pointer that cannot be instantiated outside of this package.
type CompletedConfig struct {
	*completedConfig
}

// Complete fills in any fields not set that are required to have valid data. It's mutating the receiver.
func (cfg *Config) Complete() CompletedConfig {
	c := completedConfig{
		cfg.GenericConfig.Complete(),
		&cfg.ExtraConfig,
	}

	return CompletedConfig{&c}
}

// New returns a new instance of NetguardAPIServer from the given config.
func (c completedConfig) New() (*NetguardAPIServer, error) {
	genericServer, err := c.GenericConfig.New("netguard-apiserver", genericapiserver.NewEmptyDelegate())
	if err != nil {
		return nil, err
	}

	s := &NetguardAPIServer{
		GenericAPIServer: genericServer,
		backendClient:    c.ExtraConfig.BackendClient,
	}

	scheme := runtime.NewScheme()
	if err := clientscheme.AddToScheme(scheme); err != nil {
		return nil, err
	}
	metav1.AddToGroupVersion(scheme, netguardv1beta1.SchemeGroupVersion)

	// TODO: keep the generic API server from wanting this
	unversioned := schema.GroupVersion{Group: "", Version: "v1"}
	scheme.AddUnversionedTypes(unversioned,
		&metav1.Status{},
		&metav1.APIVersions{},
		&metav1.APIGroupList{},
		&metav1.APIGroup{},
		&metav1.APIResourceList{},
	)

	codecs := serializer.NewCodecFactory(scheme)

	if err := installAPIGroup(s.GenericAPIServer, c.ExtraConfig.BackendClient, scheme, codecs); err != nil {
		return nil, err
	}

	setupHealthChecks(s.GenericAPIServer, c.ExtraConfig.BackendClient)

	return s, nil
}

// NewAPIServer creates a new Netguard API server
func NewAPIServer(config APIServerConfig, backendClient client.BackendClient) (*NetguardAPIServer, error) {
	scheme := runtime.NewScheme()
	if err := clientscheme.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to add to scheme: %w", err)
	}
	metav1.AddToGroupVersion(scheme, netguardv1beta1.SchemeGroupVersion)
	codecs := serializer.NewCodecFactory(scheme)

	serverConfig := genericapiserver.NewRecommendedConfig(codecs)

	// Настраиваем базовые компоненты
	serverConfig.Config.Authentication.Authenticator = &allowAllAuthenticator{}
	serverConfig.Config.Authorization.Authorizer = &allowAllAuthorizer{}
	serverConfig.Config.RequestInfoResolver = &request.RequestInfoFactory{
		APIPrefixes:          sets.NewString("api", "apis"),
		GrouplessAPIPrefixes: sets.NewString("api"),
	}
	serverConfig.Config.AdmissionControl = nil

	// Настраиваем serving
	var certFile, keyFile string
	var bindPort int

	if config.IsTLSEnabled() {
		certFile = config.Authn.TLS.CertFile
		keyFile = config.Authn.TLS.KeyFile
		bindPort = config.SecurePort
	} else {
		certFile = "certs/tls.crt"
		keyFile = "certs/tls.key"
		bindPort = config.InsecurePort
		klog.V(2).Info("TLS disabled in config but using self-signed certs (NOT RECOMMENDED FOR PRODUCTION)")
	}

	serverConfig.Config.ExternalAddress = config.BindAddress
	if serverConfig.Config.ExternalAddress == "" {
		serverConfig.Config.ExternalAddress = "127.0.0.1"
	}
	serverConfig.Config.PublicAddress = net.ParseIP(serverConfig.Config.ExternalAddress)

	secureOptions := &genericoptions.SecureServingOptionsWithLoopback{
		SecureServingOptions: &genericoptions.SecureServingOptions{
			BindAddress: net.ParseIP(config.BindAddress),
			BindPort:    bindPort,
			ServerCert: genericoptions.GeneratableKeyCert{
				CertKey: genericoptions.CertKey{
					CertFile: certFile,
					KeyFile:  keyFile,
				},
			},
		},
	}
	if err := secureOptions.ApplyTo(&serverConfig.Config.SecureServing, &serverConfig.Config.LoopbackClientConfig); err != nil {
		return nil, fmt.Errorf("failed to apply secure serving options: %w", err)
	}

	// Создаем kubernetes client из loopback config
	kubeClient, err := kubernetes.NewForConfig(serverConfig.Config.LoopbackClientConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	// Создаем SharedInformerFactory
	serverConfig.SharedInformerFactory = informers.NewSharedInformerFactory(kubeClient, 10*time.Minute)

	// Создаем конфигурацию по образцу sample-apiserver
	cfg := &Config{
		GenericConfig: serverConfig,
		ExtraConfig: ExtraConfig{
			BackendClient: backendClient,
		},
	}

	// Используем правильный паттерн Complete() -> New()
	return cfg.Complete().New()
}

// installAPIGroup installs the netguard.sgroups.io/v1beta1 API group
func installAPIGroup(server *genericapiserver.GenericAPIServer, backendClient client.BackendClient, scheme *runtime.Scheme, codecs serializer.CodecFactory) error {
	// Create storage for all resources
	serviceStorage := service.NewServiceStorage(backendClient)
	addressGroupStorage := addressgroup.NewAddressGroupStorage(backendClient)
	addressGroupBindingStorage := addressgroupbinding.NewAddressGroupBindingStorage(backendClient)
	addressGroupPortMappingStorage := addressgroupportmapping.NewAddressGroupPortMappingStorage(backendClient)
	ruleS2SStorage := rules2s.NewRuleS2SStorage(backendClient)
	serviceAliasStorage := servicealias.NewServiceAliasStorage(backendClient)
	addressGroupBindingPolicyStorage := addressgroupbindingpolicy.NewAddressGroupBindingPolicyStorage(backendClient)
	ieAgAgRuleStorage := ieagagrule.NewIEAgAgRuleStorage(backendClient)

	// Create status storage for Service (example)
	serviceStatusStorage := service.NewStatusREST(serviceStorage)
	serviceSyncStorage := service.NewSyncREST(serviceStorage)

	// Map all resources and subresources
	v1beta1Storage := map[string]rest.Storage{
		// Main resources
		"services":                    serviceStorage,
		"addressgroups":               addressGroupStorage,
		"addressgroupbindings":        addressGroupBindingStorage,
		"addressgroupportmappings":    addressGroupPortMappingStorage,
		"rules2s":                     ruleS2SStorage,
		"servicealiases":              serviceAliasStorage,
		"addressgroupbindingpolicies": addressGroupBindingPolicyStorage,
		"ieagagrules":                 ieAgAgRuleStorage,

		// Subresources
		"services/status": serviceStatusStorage,
		"services/sync":   serviceSyncStorage,
		// TODO: Add other subresources
	}

	// Create API group info
	apiGroupInfo := genericapiserver.NewDefaultAPIGroupInfo(
		netguardv1beta1.GroupName,
		scheme,
		metav1.ParameterCodec,
		codecs,
	)

	// Set version priority
	apiGroupInfo.PrioritizedVersions = []schema.GroupVersion{netguardv1beta1.SchemeGroupVersion}
	apiGroupInfo.VersionedResourcesStorageMap[netguardv1beta1.SchemeGroupVersion.Version] = v1beta1Storage

	// Install API group
	if err := server.InstallAPIGroup(&apiGroupInfo); err != nil {
		return fmt.Errorf("failed to install API group %s: %w", netguardv1beta1.GroupName, err)
	}

	return nil
}

// setupHealthChecks configures health check endpoints
func setupHealthChecks(server *genericapiserver.GenericAPIServer, backendClient client.BackendClient) {
	// Liveness probe - API server жив
	server.Handler.NonGoRestfulMux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// Readiness probe - API server готов принимать запросы
	server.Handler.NonGoRestfulMux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		if err := backendClient.HealthCheck(ctx); err != nil {
			http.Error(w, fmt.Sprintf("Backend unhealthy: %v", err), http.StatusServiceUnavailable)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ready"))
	})
}

// Run starts the API server
func (s *NetguardAPIServer) Run(ctx context.Context) error {
	return s.GenericAPIServer.PrepareRun().Run(ctx.Done())
}

// Shutdown gracefully shuts down the API server
func (s *NetguardAPIServer) Shutdown(ctx context.Context) error {
	if err := s.backendClient.Close(); err != nil {
		return fmt.Errorf("failed to close backend client: %w", err)
	}
	return nil
}

// allowAllAuthenticator allows all requests for testing
type allowAllAuthenticator struct{}

func (a *allowAllAuthenticator) AuthenticateRequest(req *http.Request) (*authenticator.Response, bool, error) {
	return &authenticator.Response{
		User: &user.DefaultInfo{
			Name:   "system:admin",
			Groups: []string{"system:masters"},
		},
	}, true, nil
}

// allowAllAuthorizer allows all requests for testing
type allowAllAuthorizer struct{}

func (a *allowAllAuthorizer) Authorize(ctx context.Context, attr authorizer.Attributes) (authorizer.Decision, string, error) {
	return authorizer.DecisionAllow, "", nil
}

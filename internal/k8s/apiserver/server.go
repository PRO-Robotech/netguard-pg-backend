package apiserver

import (
	"os"
	"os/signal"
	"syscall"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/registry/rest"
	genericapiserver "k8s.io/apiserver/pkg/server"

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

// ExtraConfig содержит кастомную конфигурацию.
type ExtraConfig struct {
	BackendClient client.BackendClient
}

// Config определяет конфигурацию для API сервера.
type Config struct {
	GenericConfig *genericapiserver.RecommendedConfig
	ExtraConfig   *ExtraConfig
}

// APIServer содержит состояние для Kubernetes API сервера.
type APIServer struct {
	GenericAPIServer *genericapiserver.GenericAPIServer
}

type completedConfig struct {
	GenericConfig genericapiserver.CompletedConfig
	ExtraConfig   *ExtraConfig
}

// CompletedConfig является оберткой для completedConfig.
type CompletedConfig struct {
	*completedConfig
}

// Complete fills in any fields not set that are required to have valid data. It's mutating the receiver.
func (cfg *Config) Complete() CompletedConfig {
	c := completedConfig{
		cfg.GenericConfig.Complete(),
		cfg.ExtraConfig,
	}

	return CompletedConfig{&c}
}

// New создает новый экземпляр APIServer.
func (c completedConfig) New() (*APIServer, error) {
	genericServer, err := c.GenericConfig.New("netguard-apiserver", genericapiserver.NewEmptyDelegate())
	if err != nil {
		return nil, err
	}

	s := &APIServer{
		GenericAPIServer: genericServer,
	}

	apiGroupInfo := genericapiserver.NewDefaultAPIGroupInfo(netguardv1beta1.GroupName, clientscheme.Scheme, metav1.ParameterCodec, clientscheme.Codecs)

	v1beta1Storage := make(map[string]rest.Storage)
	v1beta1Storage["services"] = service.NewServiceStorage(c.ExtraConfig.BackendClient)
	v1beta1Storage["addressgroups"] = addressgroup.NewAddressGroupStorage(c.ExtraConfig.BackendClient)
	v1beta1Storage["addressgroupbindings"] = addressgroupbinding.NewAddressGroupBindingStorage(c.ExtraConfig.BackendClient)
	v1beta1Storage["addressgroupportmappings"] = addressgroupportmapping.NewAddressGroupPortMappingStorage(c.ExtraConfig.BackendClient)
	v1beta1Storage["rules2s"] = rules2s.NewRuleS2SStorage(c.ExtraConfig.BackendClient)
	v1beta1Storage["servicealiases"] = servicealias.NewServiceAliasStorage(c.ExtraConfig.BackendClient)
	v1beta1Storage["addressgroupbindingpolicies"] = addressgroupbindingpolicy.NewAddressGroupBindingPolicyStorage(c.ExtraConfig.BackendClient)
	v1beta1Storage["ieagagrules"] = ieagagrule.NewIEAgAgRuleStorage(c.ExtraConfig.BackendClient)
	apiGroupInfo.VersionedResourcesStorageMap["v1beta1"] = v1beta1Storage

	if err := s.GenericAPIServer.InstallAPIGroup(&apiGroupInfo); err != nil {
		return nil, err
	}

	return s, nil
}

// SetupSignalHandler регистрирует обработчики сигналов.
func (s *APIServer) SetupSignalHandler() <-chan struct{} {
	stop := make(chan struct{})
	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		close(stop)
		<-c
		os.Exit(1)
	}()
	return stop
}

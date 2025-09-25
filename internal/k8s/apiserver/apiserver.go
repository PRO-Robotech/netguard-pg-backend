package apiserver

import (
	"fmt"
	"net"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	netutils "k8s.io/utils/net"

	"k8s.io/apiserver/pkg/registry/rest"
	server "k8s.io/apiserver/pkg/server"
	genericoptions "k8s.io/apiserver/pkg/server/options"
	"k8s.io/klog/v2"

	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/pkg/k8s/clientset/versioned/scheme"

	backendclient "netguard-pg-backend/internal/k8s/client"
	agstorage "netguard-pg-backend/internal/k8s/registry/addressgroup"
	bindingstorage "netguard-pg-backend/internal/k8s/registry/addressgroupbinding"
	policybindingstorage "netguard-pg-backend/internal/k8s/registry/addressgroupbindingpolicy"
	portmappingstorage "netguard-pg-backend/internal/k8s/registry/addressgroupportmapping"
	hoststorage "netguard-pg-backend/internal/k8s/registry/host"
	hostbindingstorage "netguard-pg-backend/internal/k8s/registry/host_binding"
	ieagagstorage "netguard-pg-backend/internal/k8s/registry/ieagagrule"
	networkstorage "netguard-pg-backend/internal/k8s/registry/network"
	networkbindingstorage "netguard-pg-backend/internal/k8s/registry/network_binding"
	rules2sstorage "netguard-pg-backend/internal/k8s/registry/rules2s"
	svcstorage "netguard-pg-backend/internal/k8s/registry/service"
	aliasstorage "netguard-pg-backend/internal/k8s/registry/servicealias"

	openapi "k8s.io/apiserver/pkg/endpoints/openapi"
	compatibility "k8s.io/apiserver/pkg/util/compatibility"
)

// ExtraConfig holds custom apiserver config
type ExtraConfig struct {
	// Place you custom config here.
}

// Config defines the config for the apiserver
type Config struct {
	GenericConfig *server.RecommendedConfig
	ExtraConfig   *ExtraConfig
}

// NetguardServer contains state for a Kubernetes cluster master/api server.
type NetguardServer struct {
	GenericAPIServer *server.GenericAPIServer
}

type completedConfig struct {
	GenericConfig server.CompletedConfig
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
		cfg.ExtraConfig,
	}

	// Provide an explicit external address to bypass implicit derivation that
	// expects the secure listener to be already in place during .Complete().
	hostPort := "localhost:8443"
	c.GenericConfig.ExternalAddress = hostPort

	if c.GenericConfig.EffectiveVersion == nil {
		c.GenericConfig.EffectiveVersion = compatibility.DefaultBuildEffectiveVersion()
	}

	// Config's OpenAPI will be set during server construction; nothing to do here.

	return CompletedConfig{&c}
}

// New returns a new instance of NetguardServer from the given config.
func (c completedConfig) New() (*NetguardServer, error) {
	genericServer, err := c.GenericConfig.New("netguard-apiserver", server.NewEmptyDelegate())
	if err != nil {
		return nil, err
	}

	s := &NetguardServer{
		GenericAPIServer: genericServer,
	}

	return s, nil
}

// Run starts the NetguardServer
func (s *NetguardServer) Run(stopCh <-chan struct{}) error {
	return s.GenericAPIServer.PrepareRun().Run(stopCh)
}

// NewServer builds and returns a ready-to-run aggregated API server instance.
// It mirrors the construction flow used in Kubernetes sample-apiserver, but
// disables embedded etcd because an aggregated server stores no state locally.
func NewServer(opts *genericoptions.RecommendedOptions) (*server.GenericAPIServer, error) {
	// Generate self-signed certs for localhost if the user didn't provide any.
	if err := opts.SecureServing.MaybeDefaultWithSelfSignedCerts("localhost", nil, []net.IP{netutils.ParseIPSloppy("127.0.0.1")}); err != nil {
		return nil, fmt.Errorf("self-signed certs: %w", err)
	}

	// Build the generic apiserver config and apply all selected options.
	genericCfg := server.NewRecommendedConfig(scheme.Codecs)
	if err := opts.ApplyTo(genericCfg); err != nil {
		return nil, fmt.Errorf("apply options: %w", err)
	}

	// Real OpenAPI configs using generated definitions with enum support
	genericCfg.OpenAPIConfig = server.DefaultOpenAPIConfig(netguardv1beta1.GetOpenAPIDefinitionsWithEnums, openapi.NewDefinitionNamer(scheme.Scheme))
	genericCfg.OpenAPIConfig.Info.Title = "Netguard"
	genericCfg.OpenAPIConfig.Info.Version = "v1beta1"

	genericCfg.OpenAPIV3Config = server.DefaultOpenAPIV3Config(netguardv1beta1.GetOpenAPIDefinitionsWithEnums, openapi.NewDefinitionNamer(scheme.Scheme))
	genericCfg.OpenAPIV3Config.Info.Title = "Netguard"
	genericCfg.OpenAPIV3Config.Info.Version = "v1beta1"

	// Explicit external address to avoid nil listener issues during .Complete().
	genericCfg.ExternalAddress = "localhost:8443"

	if genericCfg.EffectiveVersion == nil {
		genericCfg.EffectiveVersion = compatibility.DefaultBuildEffectiveVersion()
	}

	// Create the GenericAPIServer object.
	completedCfg := genericCfg.Complete()
	gs, err := completedCfg.New("netguard-apiserver", server.NewEmptyDelegate())
	if err != nil {
		return nil, fmt.Errorf("create generic server: %w", err)
	}

	// ------------------------------------------------------------------
	// Backend client
	// ------------------------------------------------------------------

	cfg, err := backendclient.LoadBackendClientConfig("")
	if err != nil {
		return nil, fmt.Errorf("load backend config: %w", err)
	}

	klog.Infof("backend endpoint: %s", cfg.Endpoint)
	bClient, err := backendclient.NewBackendClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("init backend client: %w", err)
	}

	// TODO: add graceful shutdown hook; for now rely on process exit

	// ------------------------------------------------------------------
	// Register API group "netguard.sgroups.io/v1beta1" with real storage.
	// ------------------------------------------------------------------

	apiGroupInfo := server.NewDefaultAPIGroupInfo(netguardv1beta1.GroupName, scheme.Scheme, metav1.ParameterCodec, scheme.Codecs)

	// Shared storage instances - using old BackendClient approach for now
	agStore := agstorage.NewAddressGroupStorage(bClient)
	svcStore := svcstorage.NewServiceStorageWithClient(bClient) // Use correct BackendClient approach
	aliasStore := aliasstorage.NewServiceAliasStorage(bClient)
	policyStore := policybindingstorage.NewAddressGroupBindingPolicyStorage(bClient)
	bindingStore := bindingstorage.NewAddressGroupBindingStorage(bClient)
	pmStore := portmappingstorage.NewAddressGroupPortMappingStorage(bClient)
	rules2sStore := rules2sstorage.NewRuleS2SStorage(bClient)
	ieagagStore := ieagagstorage.NewIEAgAgRuleStorage(bClient)

	// Use BaseStorage approach for Network resources (supports generateName)
	networkStore := networkstorage.NewNetworkStorageWithClient(bClient)
	networkBindingStore := networkbindingstorage.NewNetworkBindingStorageWithClient(bClient)

	// Host and HostBinding storage
	hostStore := hoststorage.NewHostStorage(bClient)
	hostBindingStore := hostbindingstorage.NewHostBindingStorage(bClient)

	apiGroupInfo.VersionedResourcesStorageMap[netguardv1beta1.SchemeGroupVersion.Version] = map[string]rest.Storage{
		// Основные ресурсы
		"addressgroups":               agStore,
		"services":                    svcStore,
		"servicealiases":              aliasStore,
		"addressgroupbindingpolicies": policyStore,
		"addressgroupbindings":        bindingStore,
		"addressgroupportmappings":    pmStore,
		"rules2s":                     rules2sStore,
		"ieagagrules":                 ieagagStore,
		"networks":                    networkStore,
		"networkbindings":             networkBindingStore,
		"hosts":                       hostStore,
		"hostbindings":                hostBindingStore,

		// Status subresources для всех основных ресурсов
		"addressgroups/status":               agstorage.NewStatusREST(agStore),
		"services/status":                    svcstorage.NewStatusREST(svcStore),
		"servicealiases/status":              aliasstorage.NewStatusREST(aliasStore),
		"addressgroupbindingpolicies/status": policybindingstorage.NewStatusREST(policyStore),
		"addressgroupbindings/status":        bindingstorage.NewStatusREST(bindingStore),
		"addressgroupportmappings/status":    portmappingstorage.NewStatusREST(pmStore),
		"rules2s/status":                     rules2sstorage.NewStatusREST(rules2sStore),
		"ieagagrules/status":                 ieagagstorage.NewStatusREST(ieagagStore),
		"networks/status":                    networkstorage.NewStatusREST(networkStore),
		"networkbindings/status":             networkbindingstorage.NewStatusREST(networkBindingStore),
		"hosts/status":                       hoststorage.NewStatusREST(hostStore),
		"hostbindings/status":                hostbindingstorage.NewStatusREST(hostBindingStore),

		// Additional subresources (REMOVED services/addressgroups - not needed anymore)
		"services/rules2sdstownref":            svcstorage.NewRuleS2SDstOwnRefREST(bClient),
		"addressgroupportmappings/accessports": portmappingstorage.NewAccessPortsREST(bClient),
		"addressgroups/networks":               agstorage.NewNetworksREST(bClient),
	}

	if err := gs.InstallAPIGroup(&apiGroupInfo); err != nil {
		return nil, fmt.Errorf("install API group: %w", err)
	}

	return gs, nil
}

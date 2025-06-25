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

package v1beta1

import (
	"net/http"

	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	scheme "netguard-pg-backend/pkg/k8s/clientset/versioned/scheme"

	rest "k8s.io/client-go/rest"
)

type NetguardV1beta1Interface interface {
	RESTClient() rest.Interface
	ServicesGetter
}

type ServicesGetter interface {
	Services(namespace string) ServiceInterface
}

// NetguardV1beta1Client is used to interact with features provided by the netguard group.
type NetguardV1beta1Client struct {
	restClient rest.Interface
}

func (c *NetguardV1beta1Client) Services(namespace string) ServiceInterface {
	return newServices(c, namespace)
}

// NewForConfig creates a new NetguardV1beta1Client for the given config.
func NewForConfig(c *rest.Config) (*NetguardV1beta1Client, error) {
	config := *c
	setConfigDefaults(&config)
	httpClient, err := rest.HTTPClientFor(&config)
	if err != nil {
		return nil, err
	}
	return NewForConfigAndClient(&config, httpClient)
}

// NewForConfigAndClient creates a new NetguardV1beta1Client for the given config and http client.
func NewForConfigAndClient(c *rest.Config, h *http.Client) (*NetguardV1beta1Client, error) {
	config := *c
	setConfigDefaults(&config)
	client, err := rest.RESTClientForConfigAndClient(&config, h)
	if err != nil {
		return nil, err
	}
	return &NetguardV1beta1Client{client}, nil
}

// NewForConfigOrDie creates a new NetguardV1beta1Client for the given config and
// panics if there is an error in the config.
func NewForConfigOrDie(c *rest.Config) *NetguardV1beta1Client {
	client, err := NewForConfig(c)
	if err != nil {
		panic(err)
	}
	return client
}

// New creates a new NetguardV1beta1Client for the given RESTClient.
func New(c rest.Interface) *NetguardV1beta1Client {
	return &NetguardV1beta1Client{c}
}

func setConfigDefaults(config *rest.Config) {
	gv := netguardv1beta1.SchemeGroupVersion
	config.GroupVersion = &gv
	config.APIPath = "/apis"
	config.NegotiatedSerializer = scheme.Codecs.WithoutConversion()

	if config.UserAgent == "" {
		config.UserAgent = rest.DefaultKubernetesUserAgent()
	}
}

// RESTClient returns a RESTClient that is used to communicate
// with API server by this client implementation.
func (c *NetguardV1beta1Client) RESTClient() rest.Interface {
	if c == nil {
		return nil
	}
	return c.restClient
}

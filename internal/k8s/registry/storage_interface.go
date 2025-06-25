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

package registry

import (
	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/registry/rest"

	"netguard-pg-backend/internal/k8s/client"
)

// Storage defines the interface for all resource storage implementations
type Storage interface {
	rest.Storage
	rest.Scoper
	rest.StandardStorage
	rest.Watcher
}

// BaseStorage provides common functionality for all storage implementations
type BaseStorage struct {
	backendClient client.BackendClient
	singularName  string
	groupResource schema.GroupResource
}

// NewBaseStorage creates a new base storage
func NewBaseStorage(backendClient client.BackendClient, singularName string, groupResource schema.GroupResource) *BaseStorage {
	return &BaseStorage{
		backendClient: backendClient,
		singularName:  singularName,
		groupResource: groupResource,
	}
}

// NamespaceScoped returns true as all our resources are namespaced
func (s *BaseStorage) NamespaceScoped() bool {
	return true
}

// GetSingularName returns the singular name for the resource
func (s *BaseStorage) GetSingularName() string {
	return s.singularName
}

// Helper functions for common operations

// HandleNotFound handles not found errors consistently
func HandleNotFound(err error, resource schema.GroupResource, name string) error {
	if err != nil {
		// Convert backend errors to Kubernetes API errors
		return err // TODO: Convert to proper Kubernetes API errors
	}
	return err
}

// ValidateObjectMeta validates object metadata
func ValidateObjectMeta(obj runtime.Object) error {
	// TODO: Implement common validation logic
	return nil
}

// SetDefaultConditions sets default status conditions
func SetDefaultConditions(obj runtime.Object) {
	// TODO: Implement default condition setting
}

// ConvertToK8sListOptions converts internal list options to backend scope
func ConvertToK8sListOptions(options *metainternalversion.ListOptions) interface{} {
	// TODO: Convert list options to backend scope
	return nil
}

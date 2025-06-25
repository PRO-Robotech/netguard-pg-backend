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

package admission

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"

	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	clientscheme "netguard-pg-backend/pkg/k8s/clientset/versioned/scheme"
)

// MutationWebhook handles admission mutation requests
type MutationWebhook struct {
	decoder runtime.Decoder
}

// NewMutationWebhook creates a new mutation webhook
func NewMutationWebhook() (*MutationWebhook, error) {
	scheme := runtime.NewScheme()
	if err := clientscheme.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to add to scheme: %w", err)
	}

	// Add admission types
	if err := admissionv1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to add admission to scheme: %w", err)
	}

	codecs := serializer.NewCodecFactory(scheme)
	decoder := codecs.UniversalDeserializer()

	return &MutationWebhook{
		decoder: decoder,
	}, nil
}

// Handle processes admission mutation requests
func (w *MutationWebhook) Handle(ctx context.Context, req *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
	klog.V(2).Infof("Processing mutation request for %s/%s %s", req.Kind.Group, req.Kind.Version, req.Kind.Kind)

	switch req.Kind.Kind {
	case "Service":
		return w.mutateService(ctx, req)
	case "AddressGroup":
		return w.mutateAddressGroup(ctx, req)
	case "AddressGroupBinding":
		return w.mutateAddressGroupBinding(ctx, req)
	case "AddressGroupPortMapping":
		return w.mutateAddressGroupPortMapping(ctx, req)
	case "RuleS2S":
		return w.mutateRuleS2S(ctx, req)
	case "ServiceAlias":
		return w.mutateServiceAlias(ctx, req)
	case "AddressGroupBindingPolicy":
		return w.mutateAddressGroupBindingPolicy(ctx, req)
	case "IEAgAgRule":
		return w.mutateIEAgAgRule(ctx, req)
	default:
		return &admissionv1.AdmissionResponse{
			UID:     req.UID,
			Allowed: true, // Allow unknown resources
		}
	}
}

// mutateService applies mutations to Service resources
func (w *MutationWebhook) mutateService(ctx context.Context, req *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
	var service netguardv1beta1.Service
	if err := runtime.DecodeInto(w.decoder, req.Object.Raw, &service); err != nil {
		return w.errorResponse(req.UID, fmt.Sprintf("Failed to decode Service: %v", err))
	}

	var patches []map[string]interface{}

	// Add managed-by label
	patches = append(patches, w.addManagedByLabel(&service)...)

	// Add created-by annotation
	patches = append(patches, w.addCreatedByAnnotation(&service)...)

	// Add finalizer for graceful deletion
	patches = append(patches, w.addFinalizer(&service, "netguard.sgroups.io/backend-sync")...)

	// Set default description if empty
	if service.Spec.Description == "" {
		patches = append(patches, map[string]interface{}{
			"op":    "replace",
			"path":  "/spec/description",
			"value": fmt.Sprintf("Service %s managed by netguard-apiserver", service.Name),
		})
	}

	return w.createPatchResponse(req.UID, patches)
}

// mutateAddressGroup applies mutations to AddressGroup resources
func (w *MutationWebhook) mutateAddressGroup(ctx context.Context, req *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
	var addressGroup netguardv1beta1.AddressGroup
	if err := runtime.DecodeInto(w.decoder, req.Object.Raw, &addressGroup); err != nil {
		return w.errorResponse(req.UID, fmt.Sprintf("Failed to decode AddressGroup: %v", err))
	}

	var patches []map[string]interface{}

	// Add managed-by label
	patches = append(patches, w.addManagedByLabel(&addressGroup)...)

	// Add created-by annotation
	patches = append(patches, w.addCreatedByAnnotation(&addressGroup)...)

	// Add finalizer for graceful deletion
	patches = append(patches, w.addFinalizer(&addressGroup, "netguard.sgroups.io/backend-sync")...)

	// Set default description if empty
	if addressGroup.Spec.Description == "" {
		patches = append(patches, map[string]interface{}{
			"op":    "replace",
			"path":  "/spec/description",
			"value": fmt.Sprintf("AddressGroup %s managed by netguard-apiserver", addressGroup.Name),
		})
	}

	return w.createPatchResponse(req.UID, patches)
}

// mutateAddressGroupBinding applies mutations to AddressGroupBinding resources
func (w *MutationWebhook) mutateAddressGroupBinding(ctx context.Context, req *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
	var binding netguardv1beta1.AddressGroupBinding
	if err := runtime.DecodeInto(w.decoder, req.Object.Raw, &binding); err != nil {
		return w.errorResponse(req.UID, fmt.Sprintf("Failed to decode AddressGroupBinding: %v", err))
	}

	var patches []map[string]interface{}

	// Add managed-by label
	patches = append(patches, w.addManagedByLabel(&binding)...)

	// Add created-by annotation
	patches = append(patches, w.addCreatedByAnnotation(&binding)...)

	// Add finalizer for graceful deletion
	patches = append(patches, w.addFinalizer(&binding, "netguard.sgroups.io/backend-sync")...)

	// Normalize namespace in AddressGroupRef if empty
	if binding.Spec.AddressGroupRef.Namespace == "" {
		patches = append(patches, map[string]interface{}{
			"op":    "replace",
			"path":  "/spec/addressGroupRef/namespace",
			"value": binding.Namespace,
		})
	}

	return w.createPatchResponse(req.UID, patches)
}

// mutateAddressGroupPortMapping applies mutations to AddressGroupPortMapping resources
func (w *MutationWebhook) mutateAddressGroupPortMapping(ctx context.Context, req *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
	var mapping netguardv1beta1.AddressGroupPortMapping
	if err := runtime.DecodeInto(w.decoder, req.Object.Raw, &mapping); err != nil {
		return w.errorResponse(req.UID, fmt.Sprintf("Failed to decode AddressGroupPortMapping: %v", err))
	}

	var patches []map[string]interface{}

	// Add managed-by label
	patches = append(patches, w.addManagedByLabel(&mapping)...)

	// Add created-by annotation
	patches = append(patches, w.addCreatedByAnnotation(&mapping)...)

	// Add finalizer for graceful deletion
	patches = append(patches, w.addFinalizer(&mapping, "netguard.sgroups.io/backend-sync")...)

	return w.createPatchResponse(req.UID, patches)
}

// mutateRuleS2S applies mutations to RuleS2S resources
func (w *MutationWebhook) mutateRuleS2S(ctx context.Context, req *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
	var rule netguardv1beta1.RuleS2S
	if err := runtime.DecodeInto(w.decoder, req.Object.Raw, &rule); err != nil {
		return w.errorResponse(req.UID, fmt.Sprintf("Failed to decode RuleS2S: %v", err))
	}

	var patches []map[string]interface{}

	// Add managed-by label
	patches = append(patches, w.addManagedByLabel(&rule)...)

	// Add created-by annotation
	patches = append(patches, w.addCreatedByAnnotation(&rule)...)

	// Add finalizer for graceful deletion
	patches = append(patches, w.addFinalizer(&rule, "netguard.sgroups.io/backend-sync")...)

	// Normalize namespace in ServiceLocalRef if empty
	if rule.Spec.ServiceLocalRef.Namespace == "" {
		patches = append(patches, map[string]interface{}{
			"op":    "replace",
			"path":  "/spec/serviceLocalRef/namespace",
			"value": rule.Namespace,
		})
	}

	// Normalize namespace in ServiceRef if empty
	if rule.Spec.ServiceRef.Namespace == "" {
		patches = append(patches, map[string]interface{}{
			"op":    "replace",
			"path":  "/spec/serviceRef/namespace",
			"value": rule.Namespace,
		})
	}

	return w.createPatchResponse(req.UID, patches)
}

// mutateServiceAlias applies mutations to ServiceAlias resources
func (w *MutationWebhook) mutateServiceAlias(ctx context.Context, req *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
	var alias netguardv1beta1.ServiceAlias
	if err := runtime.DecodeInto(w.decoder, req.Object.Raw, &alias); err != nil {
		return w.errorResponse(req.UID, fmt.Sprintf("Failed to decode ServiceAlias: %v", err))
	}

	var patches []map[string]interface{}

	// Add managed-by label
	patches = append(patches, w.addManagedByLabel(&alias)...)

	// Add created-by annotation
	patches = append(patches, w.addCreatedByAnnotation(&alias)...)

	// Add finalizer for graceful deletion
	patches = append(patches, w.addFinalizer(&alias, "netguard.sgroups.io/backend-sync")...)

	return w.createPatchResponse(req.UID, patches)
}

// mutateAddressGroupBindingPolicy applies mutations to AddressGroupBindingPolicy resources
func (w *MutationWebhook) mutateAddressGroupBindingPolicy(ctx context.Context, req *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
	var policy netguardv1beta1.AddressGroupBindingPolicy
	if err := runtime.DecodeInto(w.decoder, req.Object.Raw, &policy); err != nil {
		return w.errorResponse(req.UID, fmt.Sprintf("Failed to decode AddressGroupBindingPolicy: %v", err))
	}

	var patches []map[string]interface{}

	// Add managed-by label
	patches = append(patches, w.addManagedByLabel(&policy)...)

	// Add created-by annotation
	patches = append(patches, w.addCreatedByAnnotation(&policy)...)

	// Add finalizer for graceful deletion
	patches = append(patches, w.addFinalizer(&policy, "netguard.sgroups.io/backend-sync")...)

	// Normalize namespace in AddressGroupRef if empty
	if policy.Spec.AddressGroupRef.Namespace == "" {
		patches = append(patches, map[string]interface{}{
			"op":    "replace",
			"path":  "/spec/addressGroupRef/namespace",
			"value": policy.Namespace,
		})
	}

	// Normalize namespace in ServiceRef if empty
	if policy.Spec.ServiceRef.Namespace == "" {
		patches = append(patches, map[string]interface{}{
			"op":    "replace",
			"path":  "/spec/serviceRef/namespace",
			"value": policy.Namespace,
		})
	}

	return w.createPatchResponse(req.UID, patches)
}

// mutateIEAgAgRule applies mutations to IEAgAgRule resources
func (w *MutationWebhook) mutateIEAgAgRule(ctx context.Context, req *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
	var rule netguardv1beta1.IEAgAgRule
	if err := runtime.DecodeInto(w.decoder, req.Object.Raw, &rule); err != nil {
		return w.errorResponse(req.UID, fmt.Sprintf("Failed to decode IEAgAgRule: %v", err))
	}

	var patches []map[string]interface{}

	// Add managed-by label
	patches = append(patches, w.addManagedByLabel(&rule)...)

	// Add created-by annotation
	patches = append(patches, w.addCreatedByAnnotation(&rule)...)

	// Add finalizer for graceful deletion
	patches = append(patches, w.addFinalizer(&rule, "netguard.sgroups.io/backend-sync")...)

	return w.createPatchResponse(req.UID, patches)
}

// Helper functions for common mutations

// addManagedByLabel adds the managed-by label
func (w *MutationWebhook) addManagedByLabel(obj metav1.Object) []map[string]interface{} {
	var patches []map[string]interface{}

	labels := obj.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
		patches = append(patches, map[string]interface{}{
			"op":    "add",
			"path":  "/metadata/labels",
			"value": map[string]string{},
		})
	}

	if _, exists := labels["app.kubernetes.io/managed-by"]; !exists {
		patches = append(patches, map[string]interface{}{
			"op":    "add",
			"path":  "/metadata/labels/app.kubernetes.io~1managed-by",
			"value": "netguard-apiserver",
		})
	}

	return patches
}

// addCreatedByAnnotation adds the created-by annotation
func (w *MutationWebhook) addCreatedByAnnotation(obj metav1.Object) []map[string]interface{} {
	var patches []map[string]interface{}

	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
		patches = append(patches, map[string]interface{}{
			"op":    "add",
			"path":  "/metadata/annotations",
			"value": map[string]string{},
		})
	}

	if _, exists := annotations["netguard.sgroups.io/created-by"]; !exists {
		patches = append(patches, map[string]interface{}{
			"op":    "add",
			"path":  "/metadata/annotations/netguard.sgroups.io~1created-by",
			"value": "aggregated-api",
		})
	}

	return patches
}

// addFinalizer adds a finalizer for graceful deletion
func (w *MutationWebhook) addFinalizer(obj metav1.Object, finalizer string) []map[string]interface{} {
	var patches []map[string]interface{}

	finalizers := obj.GetFinalizers()
	if finalizers == nil {
		finalizers = []string{}
		patches = append(patches, map[string]interface{}{
			"op":    "add",
			"path":  "/metadata/finalizers",
			"value": []string{},
		})
	}

	// Check if finalizer already exists
	for _, f := range finalizers {
		if f == finalizer {
			return patches // Already exists
		}
	}

	// Add the finalizer
	patches = append(patches, map[string]interface{}{
		"op":    "add",
		"path":  "/metadata/finalizers/-",
		"value": finalizer,
	})

	return patches
}

// createPatchResponse creates a JSON patch admission response
func (w *MutationWebhook) createPatchResponse(uid types.UID, patches []map[string]interface{}) *admissionv1.AdmissionResponse {
	if len(patches) == 0 {
		return &admissionv1.AdmissionResponse{
			UID:     uid,
			Allowed: true,
		}
	}

	patchBytes, err := json.Marshal(patches)
	if err != nil {
		return w.errorResponse(uid, fmt.Sprintf("Failed to marshal patches: %v", err))
	}

	patchType := admissionv1.PatchTypeJSONPatch
	return &admissionv1.AdmissionResponse{
		UID:       uid,
		Allowed:   true,
		PatchType: &patchType,
		Patch:     patchBytes,
	}
}

// errorResponse creates an error admission response
func (w *MutationWebhook) errorResponse(uid types.UID, message string) *admissionv1.AdmissionResponse {
	return &admissionv1.AdmissionResponse{
		UID:     uid,
		Allowed: false,
		Result: &metav1.Status{
			Code:    http.StatusUnprocessableEntity,
			Message: message,
		},
	}
}

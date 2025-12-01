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
	case "SvcSvcRule":
		return w.mutateSvcSvcRule(ctx, req)
	case "SvcFqdnRule":
		return w.mutateSvcFqdnRule(ctx, req)
	case "AddressGroupBindingPolicy":
		return w.mutateAddressGroupBindingPolicy(ctx, req)
	case "HostBinding":
		return w.mutateHostBinding(ctx, req)
	case "NetworkBinding":
		return w.mutateNetworkBinding(ctx, req)
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

	for i := range service.Spec.AddressGroups {
		if service.Spec.AddressGroups[i].Namespace == "" {
			patches = append(patches, map[string]interface{}{
				"op":    "replace",
				"path":  fmt.Sprintf("/spec/addressGroups/%d/namespace", i),
				"value": service.Namespace,
			})
		}
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

	// Set default action if empty
	if addressGroup.Spec.DefaultAction == "" {
		patches = append(patches, map[string]interface{}{
			"op":    "replace",
			"path":  "/spec/defaultAction",
			"value": "ACCEPT", // Default to ACCEPT action
		})
	}

	for i := range addressGroup.Spec.Hosts {
		if addressGroup.Spec.Hosts[i].Namespace == "" {
			patches = append(patches, map[string]interface{}{
				"op":    "replace",
				"path":  fmt.Sprintf("/spec/hosts/%d/namespace", i),
				"value": addressGroup.Namespace,
			})
		}
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

	// Normalize namespace in ServiceRef if empty
	if binding.Spec.ServiceRef.Namespace == "" {
		patches = append(patches, map[string]interface{}{
			"op":    "replace",
			"path":  "/spec/serviceRef/namespace",
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

// mutateSvcSvcRule applies mutations to SvcSvcRule resources
func (w *MutationWebhook) mutateSvcSvcRule(ctx context.Context, req *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
	var rule netguardv1beta1.SvcSvcRule
	if err := runtime.DecodeInto(w.decoder, req.Object.Raw, &rule); err != nil {
		return w.errorResponse(req.UID, fmt.Sprintf("Failed to decode SvcSvcRule: %v", err))
	}

	var patches []map[string]interface{}

	// Add managed-by label
	patches = append(patches, w.addManagedByLabel(&rule)...)

	// Add created-by annotation
	patches = append(patches, w.addCreatedByAnnotation(&rule)...)

	// Normalize namespace in ServiceFrom if empty
	if rule.Spec.ServiceFrom.Namespace == "" {
		patches = append(patches, map[string]interface{}{
			"op":    "add",
			"path":  "/spec/serviceFrom/namespace",
			"value": rule.Namespace,
		})
	}

	// Normalize namespace in ServiceTo if empty
	if rule.Spec.ServiceTo.Namespace == "" {
		patches = append(patches, map[string]interface{}{
			"op":    "add",
			"path":  "/spec/serviceTo/namespace",
			"value": rule.Namespace,
		})
	}

	return w.createPatchResponse(req.UID, patches)
}

// mutateSvcFqdnRule applies defaulting logic to SvcFqdnRule resources.
func (w *MutationWebhook) mutateSvcFqdnRule(ctx context.Context, req *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
	var rule netguardv1beta1.SvcFqdnRule
	if err := runtime.DecodeInto(w.decoder, req.Object.Raw, &rule); err != nil {
		return w.errorResponse(req.UID, fmt.Sprintf("Failed to decode SvcFqdnRule: %v", err))
	}

	var patches []map[string]interface{}

	patches = append(patches, w.addManagedByLabel(&rule)...)
	patches = append(patches, w.addCreatedByAnnotation(&rule)...)

	if rule.Spec.ServiceFrom.Namespace == "" {
		patches = append(patches, map[string]interface{}{
			"op":    "add",
			"path":  "/spec/serviceFrom/namespace",
			"value": rule.Namespace,
		})
	}
	if rule.Spec.ServiceFrom.APIVersion == "" {
		patches = append(patches, map[string]interface{}{
			"op":    "add",
			"path":  "/spec/serviceFrom/apiVersion",
			"value": "netguard.sgroups.io/v1beta1",
		})
	}
	if rule.Spec.ServiceFrom.Kind == "" {
		patches = append(patches, map[string]interface{}{
			"op":    "add",
			"path":  "/spec/serviceFrom/kind",
			"value": "Service",
		})
	}

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

// mutateHostBinding applies mutations to HostBinding resources
func (w *MutationWebhook) mutateHostBinding(ctx context.Context, req *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
	var binding netguardv1beta1.HostBinding
	if err := runtime.DecodeInto(w.decoder, req.Object.Raw, &binding); err != nil {
		return w.errorResponse(req.UID, fmt.Sprintf("Failed to decode HostBinding: %v", err))
	}

	var patches []map[string]interface{}

	// Add managed-by label
	patches = append(patches, w.addManagedByLabel(&binding)...)

	// Add created-by annotation
	patches = append(patches, w.addCreatedByAnnotation(&binding)...)

	// Normalize namespace in HostRef if empty - HostBinding and Host must be in same namespace
	if binding.Spec.HostRef.Namespace == "" {
		patches = append(patches, map[string]interface{}{
			"op":    "replace",
			"path":  "/spec/hostRef/namespace",
			"value": binding.Namespace,
		})
	}

	// Normalize namespace in AddressGroupRef if empty - HostBinding and AddressGroup must be in same namespace
	if binding.Spec.AddressGroupRef.Namespace == "" {
		patches = append(patches, map[string]interface{}{
			"op":    "replace",
			"path":  "/spec/addressGroupRef/namespace",
			"value": binding.Namespace,
		})
	}

	return w.createPatchResponse(req.UID, patches)
}

// mutateNetworkBinding applies mutations to NetworkBinding resources
func (w *MutationWebhook) mutateNetworkBinding(ctx context.Context, req *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
	var binding netguardv1beta1.NetworkBinding
	if err := runtime.DecodeInto(w.decoder, req.Object.Raw, &binding); err != nil {
		return w.errorResponse(req.UID, fmt.Sprintf("Failed to decode NetworkBinding: %v", err))
	}

	var patches []map[string]interface{}

	// Add managed-by label
	patches = append(patches, w.addManagedByLabel(&binding)...)

	// Add created-by annotation
	patches = append(patches, w.addCreatedByAnnotation(&binding)...)

	// Normalize namespace in NetworkRef if empty - NetworkBinding and Network must be in same namespace
	if binding.Spec.NetworkRef.Namespace == "" {
		patches = append(patches, map[string]interface{}{
			"op":    "replace",
			"path":  "/spec/networkRef/namespace",
			"value": binding.Namespace,
		})
	}

	// Normalize namespace in AddressGroupRef if empty - NetworkBinding and AddressGroup must be in same namespace
	if binding.Spec.AddressGroupRef.Namespace == "" {
		patches = append(patches, map[string]interface{}{
			"op":    "replace",
			"path":  "/spec/addressGroupRef/namespace",
			"value": binding.Namespace,
		})
	}

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

package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// GroupName is the group name used in this package
const GroupName = "netguard.sgroups.io"

// SchemeGroupVersion is group version used to register these objects
var SchemeGroupVersion = schema.GroupVersion{Group: GroupName, Version: "v1beta1"}

// HubVersion is the internal hub; we reuse the external Go structs for simplicity.
var SchemeGroupVersionInternal = schema.GroupVersion{Group: GroupName, Version: runtime.APIVersionInternal}

// Resource takes an unqualified resource and returns a Group qualified GroupResource
func Resource(resource string) schema.GroupResource {
	return SchemeGroupVersion.WithResource(resource).GroupResource()
}

var (
	// SchemeBuilder is used to add go types to the GroupVersionKind scheme
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes, addKnownTypesInternal, addFieldLabelConversionFuncs)
	// AddToScheme adds the types in this group-version to the given scheme.
	AddToScheme = SchemeBuilder.AddToScheme
)

// addKnownTypes adds the set of types defined in this package to the supplied scheme.
func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(SchemeGroupVersion,
		&Service{},
		&ServiceList{},
		&AddressGroup{},
		&AddressGroupList{},
		&AddressGroupBinding{},
		&AddressGroupBindingList{},
		&AddressGroupPortMapping{},
		&AddressGroupPortMappingList{},
		&RuleS2S{},
		&RuleS2SList{},
		&ServiceAlias{},
		&ServiceAliasList{},
		&AddressGroupBindingPolicy{},
		&AddressGroupBindingPolicyList{},
		&IEAgAgRule{},
		&IEAgAgRuleList{},
		&AddressGroupsSpec{},
		&AddressGroupsSpecList{},
		&RuleS2SDstOwnRefSpec{},
		&RuleS2SDstOwnRefSpecList{},
		&AccessPortsSpec{},
		&AccessPortsSpecList{},
		&NetworksSpec{},
		&NetworksSpecList{},
		&Network{},
		&NetworkList{},
		&NetworkBinding{},
		&NetworkBindingList{},
		&Host{},
		&HostList{},
		&HostBinding{},
		&HostBindingList{},
		&SvcSvcRule{},
		&SvcSvcRuleList{},
		&SvcFqdnRule{},
		&SvcFqdnRuleList{},
	)
	metav1.AddToGroupVersion(scheme, SchemeGroupVersion)
	return nil
}

// addKnownTypesInternal registers the same structs for the internal (hub) version.
func addKnownTypesInternal(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(SchemeGroupVersionInternal,
		&Service{},
		&ServiceList{},
		&AddressGroup{},
		&AddressGroupList{},
		&AddressGroupBinding{},
		&AddressGroupBindingList{},
		&AddressGroupPortMapping{},
		&AddressGroupPortMappingList{},
		&RuleS2S{},
		&RuleS2SList{},
		&ServiceAlias{},
		&ServiceAliasList{},
		&AddressGroupBindingPolicy{},
		&AddressGroupBindingPolicyList{},
		&IEAgAgRule{},
		&IEAgAgRuleList{},
		&AddressGroupsSpec{},
		&AddressGroupsSpecList{},
		&RuleS2SDstOwnRefSpec{},
		&RuleS2SDstOwnRefSpecList{},
		&AccessPortsSpec{},
		&AccessPortsSpecList{},
		&NetworksSpec{},
		&NetworksSpecList{},
		&Network{},
		&NetworkList{},
		&NetworkBinding{},
		&NetworkBindingList{},
		&Host{},
		&HostList{},
		&HostBinding{},
		&HostBindingList{},
		&SvcSvcRule{},
		&SvcSvcRuleList{},
		&SvcFqdnRule{},
		&SvcFqdnRuleList{},
	)
	// do NOT call metav1.AddToGroupVersion for internal hub version to avoid
	// duplicate registration of meta types like WatchEvent.
	return nil
}

// addFieldLabelConversionFuncs registers field label conversion functions for resources.
// This allows kubectl to use custom field selectors beyond the default metadata.name and metadata.namespace.
//
// For example, after registration, these queries work:
//
//	kubectl get hosts --field-selector=status.isBound=true
//	kubectl get hosts --field-selector=status.addressGroupRef.name=example
//	kubectl get networks --field-selector=status.isBound=true
//	kubectl get networks --field-selector=status.addressGroupRef.name=example
//	kubectl get hostbindings --field-selector=spec.addressGroupRef.name=example
//	kubectl get networkbindings --field-selector=spec.networkRef.name=example
//	kubectl get addressgroupbindings --field-selector=spec.serviceRef.name=example
//
// Without registration, kubectl returns error: "field label not supported"
func addFieldLabelConversionFuncs(scheme *runtime.Scheme) error {
	// Register field label conversion function for Host resource
	// This enables custom field selectors for Host status fields
	if err := scheme.AddFieldLabelConversionFunc(
		SchemeGroupVersion.WithKind("Host"),
		HostFieldLabelConversion,
	); err != nil {
		return err
	}

	// Register field label conversion function for Network resource
	// This enables custom field selectors for Network status fields
	if err := scheme.AddFieldLabelConversionFunc(
		SchemeGroupVersion.WithKind("Network"),
		NetworkFieldLabelConversion,
	); err != nil {
		return err
	}

	// Register field label conversion function for HostBinding resource
	// This enables custom field selectors for HostBinding spec fields
	if err := scheme.AddFieldLabelConversionFunc(
		SchemeGroupVersion.WithKind("HostBinding"),
		HostBindingFieldLabelConversion,
	); err != nil {
		return err
	}

	// Register field label conversion function for NetworkBinding resource
	// This enables custom field selectors for NetworkBinding spec fields
	if err := scheme.AddFieldLabelConversionFunc(
		SchemeGroupVersion.WithKind("NetworkBinding"),
		NetworkBindingFieldLabelConversion,
	); err != nil {
		return err
	}

	// Register field label conversion function for AddressGroupBinding resource
	// This enables custom field selectors for AddressGroupBinding spec fields
	if err := scheme.AddFieldLabelConversionFunc(
		SchemeGroupVersion.WithKind("AddressGroupBinding"),
		AddressGroupBindingFieldLabelConversion,
	); err != nil {
		return err
	}

	// Register field label conversion function for SvcSvcRule resource
	if err := scheme.AddFieldLabelConversionFunc(
		SchemeGroupVersion.WithKind("SvcSvcRule"),
		SvcSvcRuleFieldLabelConversion,
	); err != nil {
		return err
	}

	// Register field label conversion function for SvcFqdnRule resource
	if err := scheme.AddFieldLabelConversionFunc(
		SchemeGroupVersion.WithKind("SvcFqdnRule"),
		SvcFqdnRuleFieldLabelConversion,
	); err != nil {
		return err
	}

	return nil
}

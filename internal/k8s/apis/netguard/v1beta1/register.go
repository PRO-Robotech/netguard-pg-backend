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

// Resource takes an unqualified resource and returns a Group qualified GroupResource
func Resource(resource string) schema.GroupResource {
	return SchemeGroupVersion.WithResource(resource).GroupResource()
}

var (
	// SchemeBuilder is used to add go types to the GroupVersionKind scheme
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)
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
	)
	metav1.AddToGroupVersion(scheme, SchemeGroupVersion)
	return nil
}

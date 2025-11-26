package models

import (
	"sort"
	"strconv"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
)

// Sorting overview for deterministic slice ordering across domain resources.
//
//  - Meta.Conditions: sort by LastTransitionTime ascending and then by Type.
//  - Meta.Finalizers: sort lexicographically.
//  - Service.IngressPorts: sort by Protocol, numeric Port value, Description.
//  - Service.AddressGroups: sort by Namespace, Name.
//  - Service.AggregatedAddressGroups: sort by Ref.Namespace, Ref.Name, Source.
//  - Service.XSvcSvcRules.AsServiceFrom / AsServiceTo: sort by Namespace, Name.
//  - AddressGroup.Networks: sort by Namespace, Name, CIDR.
//  - AddressGroup.Hosts: sort by Namespace, Name.
//  - AddressGroup.AggregatedHosts: sort by Ref.Namespace, Ref.Name, Source.
//  - Host.IpList: sort lexicographically by IP value.
//  - AddressGroupPortMapping.AccessPortsWithRefs: sort by ServiceRef.Namespace, ServiceRef.Name.
//  - ProtocolPorts (map values): iterate protocols in fixed order (TCP, UDP) and
//    sort each []PortRange by Start, End.

// SortConditions orders conditions by LastTransitionTime then Type.
func SortConditions(conditions []metav1.Condition) {
	if len(conditions) <= 1 {
		return
	}
	sort.SliceStable(conditions, func(i, j int) bool {
		ti := conditions[i].LastTransitionTime.Time
		tj := conditions[j].LastTransitionTime.Time
		if ti.Equal(tj) {
			if conditions[i].Type == conditions[j].Type {
				return string(conditions[i].Status) < string(conditions[j].Status)
			}
			return conditions[i].Type < conditions[j].Type
		}
		return ti.Before(tj)
	})
}

// SortFinalizers orders finalizers lexicographically.
func SortFinalizers(finalizers []string) {
	if len(finalizers) <= 1 {
		return
	}
	sort.Strings(finalizers)
}

// SortIngressPorts orders ingress ports by protocol, port value, description.
func SortIngressPorts(ports []IngressPort) {
	if len(ports) <= 1 {
		return
	}
	sort.SliceStable(ports, func(i, j int) bool {
		pi := ports[i]
		pj := ports[j]
		if pi.Protocol != pj.Protocol {
			return string(pi.Protocol) < string(pj.Protocol)
		}
		numi, erri := strconv.Atoi(pi.Port)
		numj, errj := strconv.Atoi(pj.Port)
		sameNumeric := erri == nil && errj == nil
		if sameNumeric && numi != numj {
			return numi < numj
		}
		if sameNumeric && numi == numj {
			return pi.Description < pj.Description
		}
		if erri == nil && errj != nil {
			return true
		}
		if erri != nil && errj == nil {
			return false
		}
		if pi.Port != pj.Port {
			return pi.Port < pj.Port
		}
		return pi.Description < pj.Description
	})
}

// SortAddressGroupRefs orders address group references by namespace/name.
func SortAddressGroupRefs(refs []AddressGroupRef) {
	if len(refs) <= 1 {
		return
	}
	sort.SliceStable(refs, func(i, j int) bool {
		return namespaceNameLess(refs[i].Namespace, refs[i].Name, refs[j].Namespace, refs[j].Name)
	})
}

// SortAggregatedAddressGroupRefs orders aggregated address group refs.
func SortAggregatedAddressGroupRefs(refs []AddressGroupReference) {
	if len(refs) <= 1 {
		return
	}
	sort.SliceStable(refs, func(i, j int) bool {
		ri := refs[i]
		rj := refs[j]
		if less := namespaceNameLess(ri.Ref.Namespace, ri.Ref.Name, rj.Ref.Namespace, rj.Ref.Name); less {
			return true
		} else if namespaceNameLess(rj.Ref.Namespace, rj.Ref.Name, ri.Ref.Namespace, ri.Ref.Name) {
			return false
		}
		return string(ri.Source) < string(rj.Source)
	})
}

// SortNamespacedObjectReferences orders K8s object references by namespace/name.
func SortNamespacedObjectReferences(refs []netguardv1beta1.NamespacedObjectReference) {
	if len(refs) <= 1 {
		return
	}
	sort.SliceStable(refs, func(i, j int) bool {
		return namespaceNameLess(refs[i].Namespace, refs[i].Name, refs[j].Namespace, refs[j].Name)
	})
}

// SortNetworkItems orders address group network items by namespace/name/CIDR.
func SortNetworkItems(items []NetworkItem) {
	if len(items) <= 1 {
		return
	}
	sort.SliceStable(items, func(i, j int) bool {
		if less := namespaceNameLess(items[i].Namespace, items[i].Name, items[j].Namespace, items[j].Name); less {
			return true
		} else if namespaceNameLess(items[j].Namespace, items[j].Name, items[i].Namespace, items[i].Name) {
			return false
		}
		return items[i].CIDR < items[j].CIDR
	})
}

// SortHostReferences orders aggregated host references, preserving source as tie breaker.
func SortHostReferences(refs []HostReference) {
	if len(refs) <= 1 {
		return
	}
	sort.SliceStable(refs, func(i, j int) bool {
		if less := namespaceNameLess(refs[i].Ref.Namespace, refs[i].Ref.Name, refs[j].Ref.Namespace, refs[j].Ref.Name); less {
			return true
		} else if namespaceNameLess(refs[j].Ref.Namespace, refs[j].Ref.Name, refs[i].Ref.Namespace, refs[i].Ref.Name) {
			return false
		}
		rankI := hostReferenceSourceRank(refs[i].Source)
		rankJ := hostReferenceSourceRank(refs[j].Source)
		if rankI != rankJ {
			return rankI < rankJ
		}
		return string(refs[i].Source) < string(refs[j].Source)
	})
}

func hostReferenceSourceRank(source HostRegistrationSource) int {
	switch source {
	case HostSourceSpec:
		return 0
	case HostSourceBinding:
		return 1
	default:
		return 2
	}
}

// SortIPItems orders host IP values lexicographically.
func SortIPItems(ips []IPItem) {
	if len(ips) <= 1 {
		return
	}
	sort.SliceStable(ips, func(i, j int) bool {
		return strings.Compare(ips[i].IP, ips[j].IP) < 0
	})
}

// SortServicePortsItems orders service port items and normalizes protocol ports.
func SortServicePortsItems(items []ServicePortsItem) {
	if len(items) == 0 {
		return
	}
	for idx := range items {
		NormalizeProtocolPorts(items[idx].Ports)
	}
	sort.SliceStable(items, func(i, j int) bool {
		return namespaceNameLess(items[i].ServiceRef.Namespace, items[i].ServiceRef.Name, items[j].ServiceRef.Namespace, items[j].ServiceRef.Name)
	})
}

// SortPortRanges orders port ranges by start then end.
func SortPortRanges(ranges []PortRange) {
	if len(ranges) <= 1 {
		return
	}
	sort.SliceStable(ranges, func(i, j int) bool {
		if ranges[i].Start == ranges[j].Start {
			return ranges[i].End < ranges[j].End
		}
		return ranges[i].Start < ranges[j].Start
	})
}

// NormalizeProtocolPorts sorts port ranges for each protocol and keeps only known protocols.
func NormalizeProtocolPorts(pp ProtocolPorts) {
	if pp == nil {
		return
	}
	for protocol, ranges := range pp {
		if len(ranges) == 0 {
			continue
		}
		SortPortRanges(ranges)
		pp[protocol] = ranges
	}
}

// namespaceNameLess compares two namespace/name pairs.
func namespaceNameLess(nsA, nameA, nsB, nameB string) bool {
	if nsA == nsB {
		return nameA < nameB
	}
	return nsA < nsB
}

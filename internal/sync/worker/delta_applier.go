package worker

import (
	"encoding/json"
	"fmt"

	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"

	"netguard-pg-backend/internal/domain"
	"netguard-pg-backend/internal/domain/models"
)

// applyDelta applies the change delta to the resource based on the operation type
// This prevents lost updates by applying incremental changes to the current resource state
func applyDelta(resource interface{}, operation domain.SyncOperation, delta json.RawMessage) error {
	if delta == nil || len(delta) == 0 {
		return nil
	}

	// Delta application is primarily for UPDATE operations
	// For CREATE and DELETE, we use the full resource state
	if operation != domain.SyncOperationUpdate {
		return nil
	}

	switch r := resource.(type) {
	case *models.AddressGroup:
		return applyAddressGroupDelta(r, delta)
	case *models.Host:
		return applyHostDelta(r, delta)
	case *models.Network:
		return applyNetworkDelta(r, delta)
	case *models.Service:
		return applyServiceDelta(r, delta)
	default:
		// If resource type doesn't support delta, just skip it
		// This is not an error - full resource sync will be used
		return nil
	}
}

// AddressGroupDelta represents incremental changes to an AddressGroup
type AddressGroupDelta struct {
	// AddHosts contains hosts to add to the address group
	AddHosts []string `json:"add_hosts,omitempty"`

	// RemoveHosts contains hosts to remove from the address group
	RemoveHosts []string `json:"remove_hosts,omitempty"`

	// AddNetworks contains networks to add
	AddNetworks []models.NetworkItem `json:"add_networks,omitempty"`

	// RemoveNetworks contains network names to remove
	RemoveNetworks []string `json:"remove_networks,omitempty"`

	// ReplaceDefaultAction replaces the default action if set
	ReplaceDefaultAction *models.RuleAction `json:"replace_default_action,omitempty"`

	// ReplaceLogs replaces the logs setting if set
	ReplaceLogs *bool `json:"replace_logs,omitempty"`

	// ReplaceTrace replaces the trace setting if set
	ReplaceTrace *bool `json:"replace_trace,omitempty"`
}

func applyAddressGroupDelta(ag *models.AddressGroup, delta json.RawMessage) error {
	var d AddressGroupDelta
	if err := json.Unmarshal(delta, &d); err != nil {
		return fmt.Errorf("invalid AddressGroup delta format: %w", err)
	}

	// Apply host additions
	if len(d.AddHosts) > 0 {
		// Create a set of existing host names to avoid duplicates
		existingHosts := make(map[string]bool)
		for _, hostRef := range ag.Hosts {
			existingHosts[hostRef.Name] = true
		}

		// Add new hosts that don't already exist
		for _, hostName := range d.AddHosts {
			if !existingHosts[hostName] {
				ag.Hosts = append(ag.Hosts, netguardv1beta1.ObjectReference{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "Host",
					Name:       hostName,
				})
			}
		}
	}

	// Apply host removals
	if len(d.RemoveHosts) > 0 {
		removeSet := make(map[string]bool)
		for _, hostName := range d.RemoveHosts {
			removeSet[hostName] = true
		}

		// Filter out removed hosts
		var filteredHosts []netguardv1beta1.ObjectReference
		for _, hostRef := range ag.Hosts {
			if !removeSet[hostRef.Name] {
				filteredHosts = append(filteredHosts, hostRef)
			}
		}
		ag.Hosts = filteredHosts
	}

	// Apply network additions
	if len(d.AddNetworks) > 0 {
		existingNetworks := make(map[string]bool)
		for _, net := range ag.Networks {
			existingNetworks[net.Name] = true
		}

		for _, net := range d.AddNetworks {
			if !existingNetworks[net.Name] {
				ag.Networks = append(ag.Networks, net)
			}
		}
	}

	// Apply network removals
	if len(d.RemoveNetworks) > 0 {
		removeSet := make(map[string]bool)
		for _, netName := range d.RemoveNetworks {
			removeSet[netName] = true
		}

		var filteredNetworks []models.NetworkItem
		for _, net := range ag.Networks {
			if !removeSet[net.Name] {
				filteredNetworks = append(filteredNetworks, net)
			}
		}
		ag.Networks = filteredNetworks
	}

	// Apply scalar replacements
	if d.ReplaceDefaultAction != nil {
		ag.DefaultAction = *d.ReplaceDefaultAction
	}

	if d.ReplaceLogs != nil {
		ag.Logs = *d.ReplaceLogs
	}

	if d.ReplaceTrace != nil {
		ag.Trace = *d.ReplaceTrace
	}

	return nil
}

// HostDelta represents incremental changes to a Host
type HostDelta struct {
	// ReplaceIpList replaces the IP list if set
	ReplaceIpList []string `json:"replace_ip_list,omitempty"`

	// AddIPs adds IPs to the list
	AddIPs []string `json:"add_ips,omitempty"`

	// RemoveIPs removes IPs from the list
	RemoveIPs []string `json:"remove_ips,omitempty"`
}

func applyHostDelta(host *models.Host, delta json.RawMessage) error {
	var d HostDelta
	if err := json.Unmarshal(delta, &d); err != nil {
		return fmt.Errorf("invalid Host delta format: %w", err)
	}

	// Replace IP list if specified
	if d.ReplaceIpList != nil {
		host.SetIpList(d.ReplaceIpList)
		return nil
	}

	// Apply IP additions
	if len(d.AddIPs) > 0 {
		currentIPs := host.GetIpList()
		existingIPs := make(map[string]bool)
		for _, ip := range currentIPs {
			existingIPs[ip] = true
		}

		for _, ip := range d.AddIPs {
			if !existingIPs[ip] {
				host.AddIP(ip)
			}
		}
	}

	// Apply IP removals
	if len(d.RemoveIPs) > 0 {
		removeSet := make(map[string]bool)
		for _, ip := range d.RemoveIPs {
			removeSet[ip] = true
		}

		currentIPs := host.GetIpList()
		var filteredIPs []string
		for _, ip := range currentIPs {
			if !removeSet[ip] {
				filteredIPs = append(filteredIPs, ip)
			}
		}
		host.SetIpList(filteredIPs)
	}

	return nil
}

// NetworkDelta represents incremental changes to a Network
type NetworkDelta struct {
	// ReplaceCIDR replaces the CIDR if set
	ReplaceCIDR *string `json:"replace_cidr,omitempty"`
}

func applyNetworkDelta(network *models.Network, delta json.RawMessage) error {
	var d NetworkDelta
	if err := json.Unmarshal(delta, &d); err != nil {
		return fmt.Errorf("invalid Network delta format: %w", err)
	}

	// Replace CIDR if specified
	if d.ReplaceCIDR != nil {
		network.CIDR = *d.ReplaceCIDR
	}

	return nil
}

// ServiceDelta represents incremental changes to a Service
type ServiceDelta struct {
	// AddIngressPorts adds ingress ports
	AddIngressPorts []models.IngressPort `json:"add_ingress_ports,omitempty"`

	// RemoveIngressPorts removes ingress ports (by port string)
	RemoveIngressPorts []string `json:"remove_ingress_ports,omitempty"`

	// ReplaceIngressPorts replaces all ingress ports if set
	ReplaceIngressPorts []models.IngressPort `json:"replace_ingress_ports,omitempty"`

	// ReplaceDescription replaces the description if set
	ReplaceDescription *string `json:"replace_description,omitempty"`
}

func applyServiceDelta(service *models.Service, delta json.RawMessage) error {
	var d ServiceDelta
	if err := json.Unmarshal(delta, &d); err != nil {
		return fmt.Errorf("invalid Service delta format: %w", err)
	}

	// Replace ingress ports if specified
	if d.ReplaceIngressPorts != nil {
		service.IngressPorts = d.ReplaceIngressPorts
		return nil
	}

	// Apply port additions
	if len(d.AddIngressPorts) > 0 {
		existingPorts := make(map[string]bool)
		for _, port := range service.IngressPorts {
			existingPorts[port.Port] = true
		}

		for _, port := range d.AddIngressPorts {
			if !existingPorts[port.Port] {
				service.IngressPorts = append(service.IngressPorts, port)
			}
		}
	}

	// Apply port removals
	if len(d.RemoveIngressPorts) > 0 {
		removeSet := make(map[string]bool)
		for _, portStr := range d.RemoveIngressPorts {
			removeSet[portStr] = true
		}

		var filteredPorts []models.IngressPort
		for _, port := range service.IngressPorts {
			if !removeSet[port.Port] {
				filteredPorts = append(filteredPorts, port)
			}
		}
		service.IngressPorts = filteredPorts
	}

	// Replace description if specified
	if d.ReplaceDescription != nil {
		service.Description = *d.ReplaceDescription
	}

	return nil
}

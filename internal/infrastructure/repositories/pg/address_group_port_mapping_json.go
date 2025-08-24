package pg

import (
	"encoding/json"
	"netguard-pg-backend/internal/domain/models"
)

// accessPortsEntry represents a single entry in the AccessPorts map for JSON serialization
type accessPortsEntry struct {
	ServiceRef   models.ServiceRef   `json:"serviceRef"`
	ServicePorts models.ServicePorts `json:"servicePorts"`
}

// marshalAccessPorts converts map[ServiceRef]ServicePorts to JSON
func marshalAccessPorts(accessPorts map[models.ServiceRef]models.ServicePorts) ([]byte, error) {
	// Convert map to slice for JSON marshaling
	var entries []accessPortsEntry
	for serviceRef, servicePorts := range accessPorts {
		entries = append(entries, accessPortsEntry{
			ServiceRef:   serviceRef,
			ServicePorts: servicePorts,
		})
	}
	return json.Marshal(entries)
}

// unmarshalAccessPorts converts JSON back to map[ServiceRef]ServicePorts
func unmarshalAccessPorts(data []byte) (map[models.ServiceRef]models.ServicePorts, error) {
	var entries []accessPortsEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}

	// Convert slice back to map
	accessPorts := make(map[models.ServiceRef]models.ServicePorts)
	for _, entry := range entries {
		accessPorts[entry.ServiceRef] = entry.ServicePorts
	}

	return accessPorts, nil
}

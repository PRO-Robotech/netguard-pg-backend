package utils

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
)

// BuildScopeFilter builds WHERE clause and arguments for scope filtering
// This is used across all resource readers for consistent scoping
func BuildScopeFilter(scope ports.Scope, tableAlias string) (string, []interface{}) {
	if scope == nil || scope.IsEmpty() {
		return "", nil
	}

	switch s := scope.(type) {
	case ports.ResourceIdentifierScope:
		if len(s.Identifiers) == 0 {
			return "", nil
		}

		// Build IN clauses for namespace and name pairs
		var conditions []string
		var args []interface{}
		argIndex := 1

		for _, id := range s.Identifiers {
			if id.Name == "" {
				condition := fmt.Sprintf("(%s.namespace = $%d)",
					tableAlias, argIndex)
				conditions = append(conditions, condition)
				args = append(args, id.Namespace)
				argIndex += 1
			} else {
				// Handle specific resource filtering (namespace + name)
				condition := fmt.Sprintf("(%s.namespace = $%d AND %s.name = $%d)",
					tableAlias, argIndex, tableAlias, argIndex+1)
				conditions = append(conditions, condition)
				args = append(args, id.Namespace, id.Name)
				argIndex += 2
			}
		}

		return "(" + strings.Join(conditions, " OR ") + ")", args

	default:
		return "", nil
	}
}

// MarshalLabelsAnnotations marshals labels and annotations to JSONB
func MarshalLabelsAnnotations(labels, annotations map[string]string) ([]byte, []byte, error) {
	var labelsJSON, annotationsJSON []byte
	var err error

	if labels != nil {
		labelsJSON, err = json.Marshal(labels)
		if err != nil {
			return nil, nil, errors.Wrap(err, "failed to marshal labels")
		}
	} else {
		labelsJSON = []byte("{}")
	}

	if annotations != nil {
		annotationsJSON, err = json.Marshal(annotations)
		if err != nil {
			return nil, nil, errors.Wrap(err, "failed to marshal annotations")
		}
	} else {
		annotationsJSON = []byte("{}")
	}

	return labelsJSON, annotationsJSON, nil
}

// UnmarshalLabelsAnnotations unmarshals JSONB labels and annotations
func UnmarshalLabelsAnnotations(labelsJSON, annotationsJSON []byte) (map[string]string, map[string]string, error) {
	var labels, annotations map[string]string

	if len(labelsJSON) > 0 {
		if err := json.Unmarshal(labelsJSON, &labels); err != nil {
			return nil, nil, errors.Wrap(err, "failed to unmarshal labels")
		}
	}

	if len(annotationsJSON) > 0 {
		if err := json.Unmarshal(annotationsJSON, &annotations); err != nil {
			return nil, nil, errors.Wrap(err, "failed to unmarshal annotations")
		}
	}

	return labels, annotations, nil
}

// ConvertK8sMetadata converts PostgreSQL K8s metadata to domain Meta
func ConvertK8sMetadata(resourceVersionStr string, labelsJSON, annotationsJSON []byte, conditionsJSON []byte, createdAt, updatedAt time.Time) (models.Meta, error) {
	meta := models.Meta{
		ResourceVersion: resourceVersionStr,
	}

	// Parse labels and annotations
	labels, annotations, err := UnmarshalLabelsAnnotations(labelsJSON, annotationsJSON)
	if err != nil {
		return meta, err
	}
	meta.Labels = labels
	meta.Annotations = annotations

	// Parse conditions
	if len(conditionsJSON) > 0 {
		var conditions []metav1.Condition
		if err := json.Unmarshal(conditionsJSON, &conditions); err != nil {
			return meta, errors.Wrap(err, "failed to unmarshal conditions")
		}
		meta.Conditions = conditions
	}
	// Convert timestamps
	meta.CreationTS = metav1.NewTime(createdAt)

	return meta, nil
}

// ParseIngressPorts converts JSONB ingress ports to domain IngressPort slice
func ParseIngressPorts(ingressPortsJSON []byte) ([]models.IngressPort, error) {
	if len(ingressPortsJSON) == 0 {
		return nil, nil
	}

	var ports []struct {
		Protocol    string `json:"protocol"`
		Port        string `json:"port"`
		Description string `json:"description"`
	}

	if err := json.Unmarshal(ingressPortsJSON, &ports); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal ingress ports")
	}

	result := make([]models.IngressPort, len(ports))
	for i, p := range ports {
		result[i] = models.IngressPort{
			Protocol:    models.TransportProtocol(p.Protocol),
			Port:        p.Port,
			Description: p.Description,
		}
	}

	return result, nil
}

// MarshalIngressPorts converts domain IngressPort slice to JSONB
func MarshalIngressPorts(ports []models.IngressPort) ([]byte, error) {
	if len(ports) == 0 {
		return []byte("[]"), nil
	}

	jsonPorts := make([]map[string]interface{}, len(ports))
	for i, p := range ports {
		jsonPorts[i] = map[string]interface{}{
			"protocol":    string(p.Protocol),
			"port":        p.Port,
			"description": p.Description,
		}
	}

	return json.Marshal(jsonPorts)
}

// ParseNetworkItems converts JSONB network items to domain NetworkItem slice
func ParseNetworkItems(networkItemsJSON []byte) ([]models.NetworkItem, error) {
	if len(networkItemsJSON) == 0 {
		return nil, nil
	}

	var items []models.NetworkItem
	if err := json.Unmarshal(networkItemsJSON, &items); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal network items")
	}

	return items, nil
}

// MarshalNetworkItems converts domain NetworkItem slice to JSONB
func MarshalNetworkItems(items []models.NetworkItem) ([]byte, error) {
	if len(items) == 0 {
		return []byte("[]"), nil
	}

	return json.Marshal(items)
}

// MarshalAccessPorts converts map[ServiceRef]ServicePorts to JSONB with custom keys
func MarshalAccessPorts(accessPorts map[models.ServiceRef]models.ServicePorts) ([]byte, error) {
	if len(accessPorts) == 0 {
		return []byte("{}"), nil
	}

	// Convert map to JSON-compatible structure with string keys
	jsonMap := make(map[string]interface{})
	for serviceRef, servicePorts := range accessPorts {
		// Create a composite key: namespace/name
		key := fmt.Sprintf("%s/%s", serviceRef.Namespace, serviceRef.Name)
		jsonMap[key] = servicePorts
	}

	return json.Marshal(jsonMap)
}

// UnmarshalAccessPorts converts JSONB to map[ServiceRef]ServicePorts
func UnmarshalAccessPorts(accessPortsJSON []byte) (map[models.ServiceRef]models.ServicePorts, error) {
	if len(accessPortsJSON) == 0 {
		return make(map[models.ServiceRef]models.ServicePorts), nil
	}

	// First, unmarshal into a generic map to handle the structure
	var rawMap map[string]interface{}
	if err := json.Unmarshal(accessPortsJSON, &rawMap); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal access ports raw data")
	}

	result := make(map[models.ServiceRef]models.ServicePorts)
	for key, value := range rawMap {
		// Parse the composite key: namespace/name
		parts := strings.SplitN(key, "/", 2)
		if len(parts) != 2 {
			return nil, errors.Errorf("invalid service ref key format: %s", key)
		}

		serviceRef := models.NewServiceRef(parts[1], models.WithNamespace(parts[0]))

		// Convert the value back to ServicePorts
		valueBytes, err := json.Marshal(value)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to marshal service ports for key %s", key)
		}

		var servicePorts models.ServicePorts
		if err := json.Unmarshal(valueBytes, &servicePorts); err != nil {
			return nil, errors.Wrapf(err, "failed to unmarshal service ports for key %s", key)
		}

		result[serviceRef] = servicePorts
	}

	return result, nil
}

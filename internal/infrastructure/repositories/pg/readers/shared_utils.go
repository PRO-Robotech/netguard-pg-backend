package readers

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

// buildScopeFilter builds WHERE clause and arguments for scope filtering
func (r *Reader) buildScopeFilter(scope ports.Scope, tableAlias string) (string, []interface{}) {
	if scope.IsEmpty() {
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
			condition := fmt.Sprintf("(%s.namespace = $%d AND %s.name = $%d)",
				tableAlias, argIndex, tableAlias, argIndex+1)
			conditions = append(conditions, condition)
			args = append(args, id.Namespace, id.Name)
			argIndex += 2
		}

		return "(" + strings.Join(conditions, " OR ") + ")", args

	default:
		// For other scope types, return empty filter
		return "", nil
	}
}

// parseIngressPorts converts JSONB ingress ports to domain IngressPort slice
func (r *Reader) parseIngressPorts(ingressPortsJSON []byte) ([]models.IngressPort, error) {
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

// convertK8sMetadata converts PostgreSQL K8s metadata to domain Meta
func (r *Reader) convertK8sMetadata(resourceVersionStr string, labelsJSON, annotationsJSON, conditionsJSON []byte, createdAt, updatedAt time.Time) (models.Meta, error) {
	meta := models.Meta{
		ResourceVersion: resourceVersionStr,
	}

	// Parse labels and annotations
	labels, annotations, err := r.unmarshalLabelsAnnotations(labelsJSON, annotationsJSON)
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
	// Note: updatedAt is tracked in PostgreSQL but not currently used in domain model

	return meta, nil
}

// unmarshalLabelsAnnotations unmarshals JSONB labels and annotations
func (r *Reader) unmarshalLabelsAnnotations(labelsJSON, annotationsJSON []byte) (map[string]string, map[string]string, error) {
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

// parseNetworkItems converts JSONB network items to domain NetworkItem slice
func (r *Reader) parseNetworkItems(networkItemsJSON []byte) ([]models.NetworkItem, error) {
	if len(networkItemsJSON) == 0 {
		return nil, nil
	}

	var items []models.NetworkItem
	if err := json.Unmarshal(networkItemsJSON, &items); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal network items")
	}

	return items, nil
}

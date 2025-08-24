package writers

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pkg/errors"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
)

// buildScopeFilter builds WHERE clause and arguments for scope filtering
func (w *Writer) buildScopeFilter(scope ports.Scope, tableAlias string) (string, []interface{}) {
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
			// Handle namespace-only filtering (when name is empty, filter only by namespace)
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
		// For other scope types, return empty filter
		return "", nil
	}
}

// marshalIngressPorts converts domain IngressPort slice to JSONB
func (w *Writer) marshalIngressPorts(ports []models.IngressPort) ([]byte, error) {
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

// marshalLabelsAnnotations marshals labels and annotations to JSONB
func (w *Writer) marshalLabelsAnnotations(labels, annotations map[string]string) ([]byte, []byte, error) {
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

// marshalNetworkItems converts domain NetworkItem slice to JSONB
func (w *Writer) marshalNetworkItems(items []models.NetworkItem) ([]byte, error) {
	if len(items) == 0 {
		return []byte("[]"), nil
	}

	return json.Marshal(items)
}

// marshalAccessPorts handles the complex AccessPorts map marshaling
func (w *Writer) marshalAccessPorts(accessPorts map[models.ServiceRef]models.ServicePorts) ([]byte, error) {
	if len(accessPorts) == 0 {
		return []byte("{}"), nil
	}

	// Convert to map[string]interface{} for JSON marshaling
	jsonMap := make(map[string]interface{})
	for serviceRef, servicePorts := range accessPorts {
		// Use ServiceRef as string key
		key := fmt.Sprintf("%s/%s", serviceRef.Namespace, serviceRef.Name)
		jsonMap[key] = servicePorts
	}

	return json.Marshal(jsonMap)
}

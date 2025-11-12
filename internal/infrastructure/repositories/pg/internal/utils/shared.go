package utils

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/infrastructure/repositories/pg/internal/sql_builder"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func BuildScopeFilter(scope ports.Scope, tableAlias string) (string, []interface{}) {
	if scope == nil || scope.IsEmpty() {
		return "", nil
	}
	switch s := scope.(type) {
	case ports.ResourceIdentifierScope:
		if len(s.Identifiers) == 0 {
			return "", nil
		}
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

// BuildScopeFilterWithTable builds a WHERE clause for the given scope using table name and alias
// This function supports FieldSelectorScope and uses the SQL Builder for field selectors
func BuildScopeFilterWithTable(scope ports.Scope, table string, tableAlias string) (string, []interface{}, error) {
	fmt.Printf("🔍 BuildScopeFilterWithTable CALLED: scope=%T, table=%s, alias=%s\n", scope, table, tableAlias)

	if scope == nil || scope.IsEmpty() {
		fmt.Printf("🔍 BuildScopeFilterWithTable: scope is nil or empty\n")
		return "", nil, nil
	}

	switch s := scope.(type) {
	case ports.ResourceIdentifierScope:
		fmt.Printf("🔍 BuildScopeFilterWithTable: ResourceIdentifierScope with %d identifiers\n", len(s.Identifiers))
		clause, args := BuildScopeFilter(s, tableAlias)
		return clause, args, nil

	case ports.FieldSelectorScope:
		fmt.Printf("🔍 BuildScopeFilterWithTable: FieldSelectorScope with %d fieldSelectors, %d labelSelectors\n",
			len(s.FieldSelectors), len(s.LabelSelectors))
		builder := sql_builder.NewSQLBuilder()
		whereClause, args, err := builder.BuildCombinedWHERE(
			table,
			tableAlias,
			s.Identifiers,
			s.FieldSelectors,
			s.LabelSelectors,
			1, // Start with $1
		)
		if err != nil {
			return "", nil, errors.Wrapf(err, "failed to build WHERE clause for field selectors")
		}
		return whereClause, args, nil

	default:
		// Fallback to old behavior
		clause, args := BuildScopeFilter(scope, tableAlias)
		return clause, args, nil
	}
}

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

func ConvertK8sMetadata(resourceVersionStr string, labelsJSON, annotationsJSON []byte, conditionsJSON []byte, createdAt, updatedAt time.Time, deletionTS *time.Time) (models.Meta, error) {
	meta := models.Meta{
		ResourceVersion: resourceVersionStr,
	}
	labels, annotations, err := UnmarshalLabelsAnnotations(labelsJSON, annotationsJSON)
	if err != nil {
		return meta, err
	}
	meta.Labels = labels
	meta.Annotations = annotations
	if len(conditionsJSON) > 0 {
		var conditions []metav1.Condition
		if err := json.Unmarshal(conditionsJSON, &conditions); err != nil {
			return meta, errors.Wrap(err, "failed to unmarshal conditions")
		}
		meta.Conditions = conditions
	}

	meta.DeduplicateConditions()
	models.SortConditions(meta.Conditions)
	models.SortFinalizers(meta.Finalizers)

	meta.CreationTS = metav1.NewTime(createdAt)
	if deletionTS != nil {
		meta.DeletionTS = &metav1.Time{Time: *deletionTS}
	}

	return meta, nil
}

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

	models.SortIngressPorts(result)
	return result, nil
}
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

func ParseNetworkItems(networkItemsJSON []byte) ([]models.NetworkItem, error) {
	if len(networkItemsJSON) == 0 {
		return nil, nil
	}
	var items []models.NetworkItem
	if err := json.Unmarshal(networkItemsJSON, &items); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal network items")
	}

	models.SortNetworkItems(items)
	return items, nil
}

func MarshalNetworkItems(items []models.NetworkItem) ([]byte, error) {
	if len(items) == 0 {
		return []byte("[]"), nil
	}
	return json.Marshal(items)
}

func MarshalAccessPorts(accessPorts map[models.ServiceRef]models.ServicePorts) ([]byte, error) {
	if len(accessPorts) == 0 {
		return []byte("{}"), nil
	}
	jsonMap := make(map[string]interface{})
	for serviceRef, servicePorts := range accessPorts {
		key := fmt.Sprintf("%s/%s", serviceRef.Namespace, serviceRef.Name)
		jsonMap[key] = servicePorts
	}
	return json.Marshal(jsonMap)
}

func UnmarshalAccessPorts(accessPortsJSON []byte) (map[models.ServiceRef]models.ServicePorts, error) {
	if len(accessPortsJSON) == 0 {
		return make(map[models.ServiceRef]models.ServicePorts), nil
	}
	var rawMap map[string]interface{}
	if err := json.Unmarshal(accessPortsJSON, &rawMap); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal access ports raw data")
	}
	result := make(map[models.ServiceRef]models.ServicePorts)
	for key, value := range rawMap {
		parts := strings.SplitN(key, "/", 2)
		if len(parts) != 2 {
			return nil, errors.Errorf("invalid service ref key format: %s", key)
		}
		serviceRef := models.NewServiceRef(parts[1], models.WithNamespace(parts[0]))
		valueBytes, err := json.Marshal(value)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to marshal service ports for key %s", key)
		}
		var servicePorts models.ServicePorts
		if err := json.Unmarshal(valueBytes, &servicePorts); err != nil {
			return nil, errors.Wrapf(err, "failed to unmarshal service ports for key %s", key)
		}
		result[serviceRef] = servicePorts
		models.NormalizeProtocolPorts(servicePorts.Ports)
	}
	return result, nil
}

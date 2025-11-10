package readers

import (
	"context"
	"encoding/json"
	"fmt"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/infrastructure/repositories/pg/internal/utils"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pkg/errors"
)

type addressGroupRefJSON struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
}
type aggregatedAddressGroupRefJSON struct {
	Ref    addressGroupRefJSON `json:"ref"`
	Source string              `json:"source"`
}
type svcFqdnRuleRefJSON struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
}

func (r *Reader) ListServices(ctx context.Context, consume func(models.Service) error, scope ports.Scope) error {
	query := `
	SELECT s.namespace, s.name, s.description, s.ingress_ports,
	       s.address_groups, s.aggregated_address_groups,
	       s.xsvcsvc_rules_as_from, s.xsvcsvc_rules_as_to,
	       s.xsvc_fqdn_rules,
		       m.resource_version, m.labels, m.annotations, m.conditions,
		       m.created_at, m.updated_at, m.deletion_timestamp
		FROM services s
		INNER JOIN k8s_metadata m ON s.resource_version = m.resource_version`
	whereClause, args := utils.BuildScopeFilter(scope, "s")
	if whereClause != "" {
		query += " WHERE " + whereClause
	} else {
	}
	query += " ORDER BY s.namespace, s.name"
	var rows pgx.Rows
	var err error
	maxRetries := 3
	for attempt := 0; attempt < maxRetries; attempt++ {
		rows, err = r.query(ctx, query, args...)
		if err == nil {
			break
		}
		if strings.Contains(err.Error(), "conn busy") && attempt < maxRetries-1 {
			time.Sleep(time.Duration(10*(1<<attempt)) * time.Millisecond)
			continue
		}
		return errors.Wrap(err, "failed to query services")
	}
	defer rows.Close()
	for rows.Next() {
		service, err := r.scanService(rows)
		if err != nil {
			return errors.Wrap(err, "failed to scan service")
		}
		if err := consume(service); err != nil {
			return err
		}
	}
	return rows.Err()
}
func (r *Reader) GetServiceByID(ctx context.Context, id models.ResourceIdentifier) (*models.Service, error) {
	query := `
	SELECT s.namespace, s.name, s.description, s.ingress_ports,
	       s.address_groups, s.aggregated_address_groups,
	       s.xsvcsvc_rules_as_from, s.xsvcsvc_rules_as_to,
	       s.xsvc_fqdn_rules,
		       m.resource_version, m.labels, m.annotations, m.conditions,
		       m.created_at, m.updated_at, m.deletion_timestamp
		FROM services s
		INNER JOIN k8s_metadata m ON s.resource_version = m.resource_version
		WHERE s.namespace = $1 AND s.name = $2`
	var service *models.Service
	var err error
	maxRetries := 3
	for attempt := 0; attempt < maxRetries; attempt++ {
		row := r.queryRow(ctx, query, id.Namespace, id.Name)
		service, err = r.scanServiceRow(row)
		if err == nil {
			break
		}
		if strings.Contains(err.Error(), "conn busy") && attempt < maxRetries-1 {
			time.Sleep(time.Duration(10*(1<<attempt)) * time.Millisecond)
			continue
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ports.ErrNotFound
		}
		return nil, errors.Wrap(err, "failed to scan service")
	}
	return service, nil
}
func (r *Reader) scanService(rows pgx.Rows) (models.Service, error) {
	var service models.Service
	var addressGroupsJSON, aggregatedAddressGroupsJSON []byte
	var ingressPortsJSON []byte
	var xsvcsvcRulesAsFromJSON, xsvcsvcRulesAsToJSON []byte
	var xsvcFqdnRulesJSON []byte
	var labelsJSON, annotationsJSON, conditionsJSON []byte
	var createdAt, updatedAt time.Time
	var deletionTS *time.Time
	var resourceVersion int64
	err := rows.Scan(
		&service.Namespace,
		&service.Name,
		&service.Description,
		&ingressPortsJSON,
		&addressGroupsJSON,
		&aggregatedAddressGroupsJSON,
		&xsvcsvcRulesAsFromJSON,
		&xsvcsvcRulesAsToJSON,
		&xsvcFqdnRulesJSON,
		&resourceVersion,
		&labelsJSON,
		&annotationsJSON,
		&conditionsJSON,
		&createdAt,
		&updatedAt,
		&deletionTS,
	)
	if err != nil {
		return service, err
	}
	service.IngressPorts, err = utils.ParseIngressPorts(ingressPortsJSON)
	if err != nil {
		return service, err
	}
	models.SortIngressPorts(service.IngressPorts)
	if len(addressGroupsJSON) > 0 && string(addressGroupsJSON) != "null" {
		var agRefs []addressGroupRefJSON
		if err := json.Unmarshal(addressGroupsJSON, &agRefs); err != nil {
			return service, errors.Wrap(err, "failed to parse address_groups JSON")
		}
		service.AddressGroups = make([]models.AddressGroupRef, len(agRefs))
		for i, ref := range agRefs {
			service.AddressGroups[i] = models.NewAddressGroupRef(ref.Name, models.WithNamespace(ref.Namespace))
		}
	}
	models.SortAddressGroupRefs(service.AddressGroups)
	if len(aggregatedAddressGroupsJSON) > 0 && string(aggregatedAddressGroupsJSON) != "null" {
		var aggregatedRefs []aggregatedAddressGroupRefJSON
		if err := json.Unmarshal(aggregatedAddressGroupsJSON, &aggregatedRefs); err != nil {
			return service, errors.Wrap(err, "failed to parse aggregated_address_groups JSON")
		}
		service.AggregatedAddressGroups = make([]models.AddressGroupReference, len(aggregatedRefs))
		for i, ref := range aggregatedRefs {
			domainRef := models.NewAddressGroupRef(ref.Ref.Name, models.WithNamespace(ref.Ref.Namespace))
			service.AggregatedAddressGroups[i] = models.AddressGroupReference{
				Ref:    domainRef,
				Source: models.AddressGroupRegistrationSource(ref.Source),
			}
		}
	}
	models.SortAggregatedAddressGroupRefs(service.AggregatedAddressGroups)
	if (len(xsvcsvcRulesAsFromJSON) > 0 && string(xsvcsvcRulesAsFromJSON) != "null") ||
		(len(xsvcsvcRulesAsToJSON) > 0 && string(xsvcsvcRulesAsToJSON) != "null") {
		service.XSvcSvcRules = &models.XSvcSvcRules{}
		if len(xsvcsvcRulesAsFromJSON) > 0 && string(xsvcsvcRulesAsFromJSON) != "null" {
			if err := json.Unmarshal(xsvcsvcRulesAsFromJSON, &service.XSvcSvcRules.AsServiceFrom); err != nil {
				return service, errors.Wrap(err, "failed to parse xsvcsvc_rules_as_from JSON")
			}
		}
		if len(xsvcsvcRulesAsToJSON) > 0 && string(xsvcsvcRulesAsToJSON) != "null" {
			if err := json.Unmarshal(xsvcsvcRulesAsToJSON, &service.XSvcSvcRules.AsServiceTo); err != nil {
				return service, errors.Wrap(err, "failed to parse xsvcsvc_rules_as_to JSON")
			}
		}
		models.SortNamespacedObjectReferences(service.XSvcSvcRules.AsServiceFrom)
		models.SortNamespacedObjectReferences(service.XSvcSvcRules.AsServiceTo)
	}
	if len(xsvcFqdnRulesJSON) > 0 && string(xsvcFqdnRulesJSON) != "null" {
		var ruleRefs []svcFqdnRuleRefJSON
		if err := json.Unmarshal(xsvcFqdnRulesJSON, &ruleRefs); err != nil {
			return service, errors.Wrap(err, "failed to parse xsvc_fqdn_rules JSON")
		}
		if len(ruleRefs) > 0 {
			service.XSvcFqdnRules = &models.XSvcFqdnRules{Rules: make([]models.ResourceIdentifier, len(ruleRefs))}
			for i, ref := range ruleRefs {
				service.XSvcFqdnRules.Rules[i] = models.NewResourceIdentifier(ref.Name, models.WithNamespace(ref.Namespace))
			}
			sort.Slice(service.XSvcFqdnRules.Rules, func(i, j int) bool {
				return service.XSvcFqdnRules.Rules[i].Key() < service.XSvcFqdnRules.Rules[j].Key()
			})
		}
	}
	service.Meta, err = utils.ConvertK8sMetadata(fmt.Sprintf("%d", resourceVersion), labelsJSON, annotationsJSON, conditionsJSON, createdAt, updatedAt, deletionTS)
	if err != nil {
		return service, err
	}
	service.SelfRef = models.NewSelfRef(models.NewResourceIdentifier(service.Name, models.WithNamespace(service.Namespace)))
	return service, nil
}
func (r *Reader) scanServiceRow(row pgx.Row) (*models.Service, error) {
	var service models.Service
	var addressGroupsJSON, aggregatedAddressGroupsJSON []byte
	var ingressPortsJSON []byte
	var xsvcsvcRulesAsFromJSON, xsvcsvcRulesAsToJSON []byte
	var xsvcFqdnRulesJSON []byte
	var labelsJSON, annotationsJSON, conditionsJSON []byte
	var createdAt, updatedAt time.Time
	var deletionTS *time.Time
	var resourceVersion int64
	err := row.Scan(
		&service.Namespace,
		&service.Name,
		&service.Description,
		&ingressPortsJSON,
		&addressGroupsJSON,
		&aggregatedAddressGroupsJSON,
		&xsvcsvcRulesAsFromJSON,
		&xsvcsvcRulesAsToJSON,
		&xsvcFqdnRulesJSON,
		&resourceVersion,
		&labelsJSON,
		&annotationsJSON,
		&conditionsJSON,
		&createdAt,
		&updatedAt,
		&deletionTS,
	)
	if err != nil {
		return nil, err
	}
	service.IngressPorts, err = utils.ParseIngressPorts(ingressPortsJSON)
	if err != nil {
		return nil, err
	}
	models.SortIngressPorts(service.IngressPorts)
	if len(addressGroupsJSON) > 0 && string(addressGroupsJSON) != "null" {
		var agRefs []addressGroupRefJSON
		if err := json.Unmarshal(addressGroupsJSON, &agRefs); err != nil {
			return nil, errors.Wrap(err, "failed to parse address_groups JSON")
		}
		service.AddressGroups = make([]models.AddressGroupRef, len(agRefs))
		for i, ref := range agRefs {
			service.AddressGroups[i] = models.NewAddressGroupRef(ref.Name, models.WithNamespace(ref.Namespace))
		}
	}
	models.SortAddressGroupRefs(service.AddressGroups)
	if len(aggregatedAddressGroupsJSON) > 0 && string(aggregatedAddressGroupsJSON) != "null" {
		var aggregatedRefs []aggregatedAddressGroupRefJSON
		if err := json.Unmarshal(aggregatedAddressGroupsJSON, &aggregatedRefs); err != nil {
			return nil, errors.Wrap(err, "failed to parse aggregated_address_groups JSON")
		}
		service.AggregatedAddressGroups = make([]models.AddressGroupReference, len(aggregatedRefs))
		for i, ref := range aggregatedRefs {
			domainRef := models.NewAddressGroupRef(ref.Ref.Name, models.WithNamespace(ref.Ref.Namespace))
			service.AggregatedAddressGroups[i] = models.AddressGroupReference{
				Ref:    domainRef,
				Source: models.AddressGroupRegistrationSource(ref.Source),
			}
		}
	}
	models.SortAggregatedAddressGroupRefs(service.AggregatedAddressGroups)
	if (len(xsvcsvcRulesAsFromJSON) > 0 && string(xsvcsvcRulesAsFromJSON) != "null") ||
		(len(xsvcsvcRulesAsToJSON) > 0 && string(xsvcsvcRulesAsToJSON) != "null") {
		service.XSvcSvcRules = &models.XSvcSvcRules{}
		if len(xsvcsvcRulesAsFromJSON) > 0 && string(xsvcsvcRulesAsFromJSON) != "null" {
			if err := json.Unmarshal(xsvcsvcRulesAsFromJSON, &service.XSvcSvcRules.AsServiceFrom); err != nil {
				return nil, errors.Wrap(err, "failed to parse xsvcsvc_rules_as_from JSON")
			}
		}
		if len(xsvcsvcRulesAsToJSON) > 0 && string(xsvcsvcRulesAsToJSON) != "null" {
			if err := json.Unmarshal(xsvcsvcRulesAsToJSON, &service.XSvcSvcRules.AsServiceTo); err != nil {
				return nil, errors.Wrap(err, "failed to parse xsvcsvc_rules_as_to JSON")
			}
		}
		models.SortNamespacedObjectReferences(service.XSvcSvcRules.AsServiceFrom)
		models.SortNamespacedObjectReferences(service.XSvcSvcRules.AsServiceTo)
	}
	if len(xsvcFqdnRulesJSON) > 0 && string(xsvcFqdnRulesJSON) != "null" {
		var ruleRefs []svcFqdnRuleRefJSON
		if err := json.Unmarshal(xsvcFqdnRulesJSON, &ruleRefs); err != nil {
			return nil, errors.Wrap(err, "failed to parse xsvc_fqdn_rules JSON")
		}
		if len(ruleRefs) > 0 {
			service.XSvcFqdnRules = &models.XSvcFqdnRules{Rules: make([]models.ResourceIdentifier, len(ruleRefs))}
			for i, ref := range ruleRefs {
				service.XSvcFqdnRules.Rules[i] = models.NewResourceIdentifier(ref.Name, models.WithNamespace(ref.Namespace))
			}
			sort.Slice(service.XSvcFqdnRules.Rules, func(i, j int) bool {
				return service.XSvcFqdnRules.Rules[i].Key() < service.XSvcFqdnRules.Rules[j].Key()
			})
		}
	}
	service.Meta, err = utils.ConvertK8sMetadata(fmt.Sprintf("%d", resourceVersion), labelsJSON, annotationsJSON, conditionsJSON, createdAt, updatedAt, deletionTS)
	if err != nil {
		return nil, err
	}
	service.SelfRef = models.NewSelfRef(models.NewResourceIdentifier(service.Name, models.WithNamespace(service.Namespace)))
	return &service, nil
}

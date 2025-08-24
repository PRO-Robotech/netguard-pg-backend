package pg

//
//import (
//	"context"
//	"encoding/json"
//
//	"github.com/jackc/pgx/v5"
//	"github.com/jackc/pgx/v5/pgxpool"
//	"github.com/pkg/errors"
//
//	"netguard-pg-backend/internal/domain/models"
//	"netguard-pg-backend/internal/domain/ports"
//)
//
//// ServiceAlias represents a record in the tbl_service_alias table
//type ServiceAlias struct {
//	Name             string
//	Namespace        string
//	ServiceName      string
//	ServiceNamespace string
//}
//
//// reader implements the ports.Reader interface for PostgreSQL
//type reader struct {
//	registry *Registry
//	conn     *pgxpool.Conn
//	ctx      context.Context
//}
//
//// Close releases the connection
//func (r *reader) Close() error {
//	r.conn.Release()
//	return nil
//}
//
//// ListServices lists services
//func (r *reader) ListServices(ctx context.Context, consume func(models.Service) error, scope ports.Scope) error {
//	query := `SELECT name, namespace, description, ingress_ports FROM netguard.tbl_service`
//
//	args := []interface{}{}
//	if !scope.IsEmpty() {
//		if ris, ok := scope.(ports.ResourceIdentifierScope); ok && !ris.IsEmpty() {
//			pairs := make([][]string, 0, len(ris.Identifiers))
//			for _, id := range ris.Identifiers {
//				pairs = append(pairs, []string{id.Name, id.Namespace})
//			}
//
//			query += ` WHERE (name, namespace) = ANY($1)`
//			args = append(args, pairs)
//		}
//	}
//
//	rows, err := r.conn.Query(ctx, query, args...)
//	if err != nil {
//		return errors.Wrap(err, "failed to query services")
//	}
//	defer rows.Close()
//
//	for rows.Next() {
//		var svc Service
//		var ingressPortsJSON []byte
//
//		if err := rows.Scan(&svc.Name, &svc.Namespace, &svc.Description, &ingressPortsJSON); err != nil {
//			return errors.Wrap(err, "failed to scan service")
//		}
//
//		// Convert JSON to struct
//		var ingressPorts []struct {
//			Protocol    string `json:"protocol"`
//			Port        string `json:"port"`
//			Description string `json:"description"`
//		}
//
//		if len(ingressPortsJSON) > 0 {
//			if err := json.Unmarshal(ingressPortsJSON, &ingressPorts); err != nil {
//				return errors.Wrap(err, "failed to unmarshal ingress ports")
//			}
//		}
//
//		// Convert to domain model
//		domainSvc := models.Service{
//			ResourceIdentifier: models.NewResourceIdentifier(svc.Name, svc.Namespace),
//			Description:        svc.Description,
//		}
//
//		for _, p := range ingressPorts {
//			domainSvc.IngressPorts = append(domainSvc.IngressPorts, models.IngressPort{
//				Protocol:    models.TransportProtocol(p.Protocol),
//				Port:        p.Port,
//				Description: p.Description,
//			})
//		}
//
//		// Get related address groups
//		addrGroupsQuery := `
//			SELECT ag.name, ag.namespace
//			FROM netguard.tbl_address_group_binding agb
//			JOIN netguard.tbl_address_group ag ON agb.address_group_name = ag.name AND agb.address_group_namespace = ag.namespace
//			WHERE agb.service_name = $1 AND agb.service_namespace = $2
//		`
//
//		addrGroupRows, err := r.conn.Query(ctx, addrGroupsQuery, svc.Name, svc.Namespace)
//		if err != nil {
//			return errors.Wrap(err, "failed to query address groups")
//		}
//
//		for addrGroupRows.Next() {
//			var name, namespace string
//			if err := addrGroupRows.Scan(&name, &namespace); err != nil {
//				addrGroupRows.Close()
//				return errors.Wrap(err, "failed to scan address group")
//			}
//
//			domainSvc.AddressGroups = append(domainSvc.AddressGroups, models.AddressGroupRef{
//				ResourceIdentifier: models.NewResourceIdentifier(name, namespace),
//			})
//		}
//		addrGroupRows.Close()
//
//		if err := consume(domainSvc); err != nil {
//			return err
//		}
//	}
//
//	return rows.Err()
//}
//
//// ListAddressGroups lists address groups
//func (r *reader) ListAddressGroups(ctx context.Context, consume func(models.AddressGroup) error, scope ports.Scope) error {
//	query := `SELECT name, namespace, description, addresses FROM netguard.tbl_address_group`
//
//	args := []interface{}{}
//	if !scope.IsEmpty() {
//		if ris, ok := scope.(ports.ResourceIdentifierScope); ok && !ris.IsEmpty() {
//			pairs := make([][]string, 0, len(ris.Identifiers))
//			for _, id := range ris.Identifiers {
//				pairs = append(pairs, []string{id.Name, id.Namespace})
//			}
//
//			query += ` WHERE (name, namespace) = ANY($1)`
//			args = append(args, pairs)
//		}
//	}
//
//	rows, err := r.conn.Query(ctx, query, args...)
//	if err != nil {
//		return errors.Wrap(err, "failed to query address groups")
//	}
//	defer rows.Close()
//
//	for rows.Next() {
//		var ag AddressGroup
//
//		if err := rows.Scan(&ag.Name, &ag.Namespace, &ag.Description, &ag.Addresses); err != nil {
//			return errors.Wrap(err, "failed to scan address group")
//		}
//
//		// Convert to domain model
//		domainAg := models.AddressGroup{
//			ResourceIdentifier: models.NewResourceIdentifier(ag.Name, ag.Namespace),
//			Description:        ag.Description,
//			Addresses:          ag.Addresses,
//		}
//
//		// Get related services
//		servicesQuery := `
//			SELECT s.name, s.namespace
//			FROM netguard.tbl_address_group_binding agb
//			JOIN netguard.tbl_service s ON agb.service_name = s.name AND agb.service_namespace = s.namespace
//			WHERE agb.address_group_name = $1 AND agb.address_group_namespace = $2
//		`
//
//		serviceRows, err := r.conn.Query(ctx, servicesQuery, ag.Name, ag.Namespace)
//		if err != nil {
//			return errors.Wrap(err, "failed to query services")
//		}
//
//		for serviceRows.Next() {
//			var name, namespace string
//			if err := serviceRows.Scan(&name, &namespace); err != nil {
//				serviceRows.Close()
//				return errors.Wrap(err, "failed to scan service")
//			}
//
//			domainAg.Services = append(domainAg.Services, models.ServiceRef{
//				ResourceIdentifier: models.NewResourceIdentifier(name, namespace),
//			})
//		}
//		serviceRows.Close()
//
//		if err := consume(domainAg); err != nil {
//			return err
//		}
//	}
//
//	return rows.Err()
//}
//
//// ListAddressGroupBindings lists address group bindings
//func (r *reader) ListAddressGroupBindings(ctx context.Context, consume func(models.AddressGroupBinding) error, scope ports.Scope) error {
//	query := `
//		SELECT name, namespace, service_name, service_namespace, address_group_name, address_group_namespace
//		FROM netguard.tbl_address_group_binding
//	`
//
//	args := []interface{}{}
//	if !scope.IsEmpty() {
//		if ris, ok := scope.(ports.ResourceIdentifierScope); ok && !ris.IsEmpty() {
//			pairs := make([][]string, 0, len(ris.Identifiers))
//			for _, id := range ris.Identifiers {
//				pairs = append(pairs, []string{id.Name, id.Namespace})
//			}
//
//			query += ` WHERE (name, namespace) = ANY($1)`
//			args = append(args, pairs)
//		}
//	}
//
//	rows, err := r.conn.Query(ctx, query, args...)
//	if err != nil {
//		return errors.Wrap(err, "failed to query address group bindings")
//	}
//	defer rows.Close()
//
//	for rows.Next() {
//		var binding AddressGroupBinding
//
//		if err := rows.Scan(
//			&binding.Name,
//			&binding.Namespace,
//			&binding.ServiceName,
//			&binding.ServiceNamespace,
//			&binding.AddressGroupName,
//			&binding.AddressGroupNamespace,
//		); err != nil {
//			return errors.Wrap(err, "failed to scan address group binding")
//		}
//
//		// Convert to domain model
//		domainBinding := models.AddressGroupBinding{
//			ResourceIdentifier: models.NewResourceIdentifier(binding.Name, binding.Namespace),
//			ServiceRef: models.ServiceRef{
//				ResourceIdentifier: models.NewResourceIdentifier(binding.ServiceName, binding.ServiceNamespace),
//			},
//			AddressGroupRef: models.AddressGroupRef{
//				ResourceIdentifier: models.NewResourceIdentifier(binding.AddressGroupName, binding.AddressGroupNamespace),
//			},
//		}
//
//		if err := consume(domainBinding); err != nil {
//			return err
//		}
//	}
//
//	return rows.Err()
//}
//
//// ListAddressGroupPortMappings lists address group port mappings
//func (r *reader) ListAddressGroupPortMappings(ctx context.Context, consume func(models.AddressGroupPortMapping) error, scope ports.Scope) error {
//	query := `SELECT name, namespace, access_ports FROM netguard.tbl_address_group_port_mapping`
//
//	args := []interface{}{}
//	if !scope.IsEmpty() {
//		if ris, ok := scope.(ports.ResourceIdentifierScope); ok && !ris.IsEmpty() {
//			pairs := make([][]string, 0, len(ris.Identifiers))
//			for _, id := range ris.Identifiers {
//				pairs = append(pairs, []string{id.Name, id.Namespace})
//			}
//
//			query += ` WHERE (name, namespace) = ANY($1)`
//			args = append(args, pairs)
//		}
//	}
//
//	rows, err := r.conn.Query(ctx, query, args...)
//	if err != nil {
//		return errors.Wrap(err, "failed to query address group port mappings")
//	}
//	defer rows.Close()
//
//	for rows.Next() {
//		var mapping struct {
//			Name        string
//			Namespace   string
//			AccessPorts []byte
//		}
//
//		if err := rows.Scan(&mapping.Name, &mapping.Namespace, &mapping.AccessPorts); err != nil {
//			return errors.Wrap(err, "failed to scan address group port mapping")
//		}
//
//		// Convert JSON to struct
//		var accessPorts []struct {
//			Name      string `json:"name"`
//			Namespace string `json:"namespace"`
//			Ports     map[string][]struct {
//				Start int `json:"start"`
//				End   int `json:"end"`
//			} `json:"ports"`
//		}
//
//		if len(mapping.AccessPorts) > 0 {
//			if err := json.Unmarshal(mapping.AccessPorts, &accessPorts); err != nil {
//				return errors.Wrap(err, "failed to unmarshal access ports")
//			}
//		}
//
//		// Convert to domain model
//		domainMapping := models.AddressGroupPortMapping{
//			ResourceIdentifier: models.NewResourceIdentifier(mapping.Name, mapping.Namespace),
//		}
//
//		for _, ap := range accessPorts {
//			spr := models.ServicePortsRef{
//				ResourceIdentifier: models.NewResourceIdentifier(ap.Name, ap.Namespace),
//				Ports:              make(models.ProtocolPorts),
//			}
//
//			for proto, ranges := range ap.Ports {
//				portRanges := make([]models.PortRange, 0, len(ranges))
//				for _, r := range ranges {
//					portRanges = append(portRanges, models.PortRange{
//						Start: r.Start,
//						End:   r.End,
//					})
//				}
//				spr.Ports[models.TransportProtocol(proto)] = portRanges
//			}
//
//			domainMapping.AccessPorts = append(domainMapping.AccessPorts, spr)
//		}
//
//		if err := consume(domainMapping); err != nil {
//			return err
//		}
//	}
//
//	return rows.Err()
//}
//
//// ListRuleS2S lists rule s2s
//func (r *reader) ListRuleS2S(ctx context.Context, consume func(models.RuleS2S) error, scope ports.Scope) error {
//	query := `
//		SELECT name, namespace, traffic, service_local_name, service_local_namespace, service_name, service_namespace
//		FROM netguard.tbl_rule_s2s
//	`
//
//	args := []interface{}{}
//	if !scope.IsEmpty() {
//		if ris, ok := scope.(ports.ResourceIdentifierScope); ok && !ris.IsEmpty() {
//			pairs := make([][]string, 0, len(ris.Identifiers))
//			for _, id := range ris.Identifiers {
//				pairs = append(pairs, []string{id.Name, id.Namespace})
//			}
//
//			query += ` WHERE (name, namespace) = ANY($1)`
//			args = append(args, pairs)
//		}
//	}
//
//	rows, err := r.conn.Query(ctx, query, args...)
//	if err != nil {
//		return errors.Wrap(err, "failed to query rule s2s")
//	}
//	defer rows.Close()
//
//	for rows.Next() {
//		var rule RuleS2S
//
//		if err := rows.Scan(
//			&rule.Name,
//			&rule.Namespace,
//			&rule.Traffic,
//			&rule.ServiceLocalName,
//			&rule.ServiceLocalNamespace,
//			&rule.ServiceName,
//			&rule.ServiceNamespace,
//		); err != nil {
//			return errors.Wrap(err, "failed to scan rule s2s")
//		}
//
//		// Convert to domain model
//		domainRule := models.RuleS2S{
//			ResourceIdentifier: models.NewResourceIdentifier(rule.Name, rule.Namespace),
//			Traffic:            models.Traffic(rule.Traffic),
//			ServiceLocalRef: models.ServiceRef{
//				ResourceIdentifier: models.NewResourceIdentifier(rule.ServiceLocalName, rule.ServiceLocalNamespace),
//			},
//			ServiceRef: models.ServiceRef{
//				ResourceIdentifier: models.NewResourceIdentifier(rule.ServiceName, rule.ServiceNamespace),
//			},
//		}
//
//		if err := consume(domainRule); err != nil {
//			return err
//		}
//	}
//
//	return rows.Err()
//}
//
//// ListServiceAliases lists service aliases
//func (r *reader) ListServiceAliases(ctx context.Context, consume func(models.ServiceAlias) error, scope ports.Scope) error {
//	query := `
//		SELECT name, namespace, service_name, service_namespace
//		FROM netguard.tbl_service_alias
//	`
//
//	args := []interface{}{}
//	if !scope.IsEmpty() {
//		if ris, ok := scope.(ports.ResourceIdentifierScope); ok && !ris.IsEmpty() {
//			pairs := make([][]string, 0, len(ris.Identifiers))
//			for _, id := range ris.Identifiers {
//				pairs = append(pairs, []string{id.Name, id.Namespace})
//			}
//
//			query += ` WHERE (name, namespace) = ANY($1)`
//			args = append(args, pairs)
//		}
//	}
//
//	rows, err := r.conn.Query(ctx, query, args...)
//	if err != nil {
//		return errors.Wrap(err, "failed to query service aliases")
//	}
//	defer rows.Close()
//
//	for rows.Next() {
//		var alias ServiceAlias
//
//		if err := rows.Scan(
//			&alias.Name,
//			&alias.Namespace,
//			&alias.ServiceName,
//			&alias.ServiceNamespace,
//		); err != nil {
//			return errors.Wrap(err, "failed to scan service alias")
//		}
//
//		// Convert to domain model
//		domainAlias := models.ServiceAlias{
//			ResourceIdentifier: models.NewResourceIdentifier(alias.Name, alias.Namespace),
//			ServiceRef: models.ServiceRef{
//				ResourceIdentifier: models.NewResourceIdentifier(alias.ServiceName, alias.ServiceNamespace),
//			},
//		}
//
//		if err := consume(domainAlias); err != nil {
//			return err
//		}
//	}
//
//	return rows.Err()
//}
//
//// GetServiceByID gets a service by ID
//func (r *reader) GetServiceByID(ctx context.Context, id models.ResourceIdentifier) (*models.Service, error) {
//	query := `SELECT name, namespace, description, ingress_ports FROM netguard.tbl_service WHERE name = $1 AND namespace = $2`
//
//	var svc Service
//	var ingressPortsJSON []byte
//
//	if err := r.conn.QueryRow(ctx, query, id.Name, id.Namespace).Scan(&svc.Name, &svc.Namespace, &svc.Description, &ingressPortsJSON); err != nil {
//		if errors.Is(err, pgx.ErrNoRows) {
//			return nil, nil // Return nil if not found
//		}
//		return nil, errors.Wrap(err, "failed to get service")
//	}
//
//	// Convert JSON to struct
//	var ingressPorts []struct {
//		Protocol    string `json:"protocol"`
//		Port        string `json:"port"`
//		Description string `json:"description"`
//	}
//
//	if len(ingressPortsJSON) > 0 {
//		if err := json.Unmarshal(ingressPortsJSON, &ingressPorts); err != nil {
//			return nil, errors.Wrap(err, "failed to unmarshal ingress ports")
//		}
//	}
//
//	// Convert to domain model
//	domainSvc := models.Service{
//		ResourceIdentifier: models.NewResourceIdentifier(svc.Name, svc.Namespace),
//		Description:        svc.Description,
//	}
//
//	for _, p := range ingressPorts {
//		domainSvc.IngressPorts = append(domainSvc.IngressPorts, models.IngressPort{
//			Protocol:    models.TransportProtocol(p.Protocol),
//			Port:        p.Port,
//			Description: p.Description,
//		})
//	}
//
//	// Get related address groups
//	addrGroupsQuery := `
//		SELECT ag.name, ag.namespace
//		FROM netguard.tbl_address_group_binding agb
//		JOIN netguard.tbl_address_group ag ON agb.address_group_name = ag.name AND agb.address_group_namespace = ag.namespace
//		WHERE agb.service_name = $1 AND agb.service_namespace = $2
//	`
//
//	addrGroupRows, err := r.conn.Query(ctx, addrGroupsQuery, svc.Name, svc.Namespace)
//	if err != nil {
//		return nil, errors.Wrap(err, "failed to query address groups")
//	}
//	defer addrGroupRows.Close()
//
//	for addrGroupRows.Next() {
//		var name, namespace string
//		if err := addrGroupRows.Scan(&name, &namespace); err != nil {
//			return nil, errors.Wrap(err, "failed to scan address group")
//		}
//
//		domainSvc.AddressGroups = append(domainSvc.AddressGroups, models.AddressGroupRef{
//			ResourceIdentifier: models.NewResourceIdentifier(name, namespace),
//		})
//	}
//
//	if err := addrGroupRows.Err(); err != nil {
//		return nil, errors.Wrap(err, "error iterating address groups")
//	}
//
//	return &domainSvc, nil
//}
//
//// GetAddressGroupByID gets an address group by ID
//func (r *reader) GetAddressGroupByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroup, error) {
//	query := `SELECT name, namespace, description, addresses FROM netguard.tbl_address_group WHERE name = $1 AND namespace = $2`
//
//	var ag AddressGroup
//
//	if err := r.conn.QueryRow(ctx, query, id.Name, id.Namespace).Scan(&ag.Name, &ag.Namespace, &ag.Description, &ag.Addresses); err != nil {
//		if errors.Is(err, pgx.ErrNoRows) {
//			return nil, nil // Return nil if not found
//		}
//		return nil, errors.Wrap(err, "failed to get address group")
//	}
//
//	// Convert to domain model
//	domainAg := models.AddressGroup{
//		ResourceIdentifier: models.NewResourceIdentifier(ag.Name, ag.Namespace),
//		Description:        ag.Description,
//		Addresses:          ag.Addresses,
//	}
//
//	// Get related services
//	servicesQuery := `
//		SELECT s.name, s.namespace
//		FROM netguard.tbl_address_group_binding agb
//		JOIN netguard.tbl_service s ON agb.service_name = s.name AND agb.service_namespace = s.namespace
//		WHERE agb.address_group_name = $1 AND agb.address_group_namespace = $2
//	`
//
//	serviceRows, err := r.conn.Query(ctx, servicesQuery, ag.Name, ag.Namespace)
//	if err != nil {
//		return nil, errors.Wrap(err, "failed to query services")
//	}
//	defer serviceRows.Close()
//
//	for serviceRows.Next() {
//		var name, namespace string
//		if err := serviceRows.Scan(&name, &namespace); err != nil {
//			return nil, errors.Wrap(err, "failed to scan service")
//		}
//
//		domainAg.Services = append(domainAg.Services, models.ServiceRef{
//			ResourceIdentifier: models.NewResourceIdentifier(name, namespace),
//		})
//	}
//
//	if err := serviceRows.Err(); err != nil {
//		return nil, errors.Wrap(err, "error iterating services")
//	}
//
//	return &domainAg, nil
//}
//
//// GetAddressGroupBindingByID gets an address group binding by ID
//func (r *reader) GetAddressGroupBindingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBinding, error) {
//	query := `
//		SELECT name, namespace, service_name, service_namespace, address_group_name, address_group_namespace
//		FROM netguard.tbl_address_group_binding
//		WHERE name = $1 AND namespace = $2
//	`
//
//	var binding AddressGroupBinding
//
//	if err := r.conn.QueryRow(ctx, query, id.Name, id.Namespace).Scan(
//		&binding.Name,
//		&binding.Namespace,
//		&binding.ServiceName,
//		&binding.ServiceNamespace,
//		&binding.AddressGroupName,
//		&binding.AddressGroupNamespace,
//	); err != nil {
//		if errors.Is(err, pgx.ErrNoRows) {
//			return nil, nil // Return nil if not found
//		}
//		return nil, errors.Wrap(err, "failed to get address group binding")
//	}
//
//	// Convert to domain model
//	domainBinding := models.AddressGroupBinding{
//		ResourceIdentifier: models.NewResourceIdentifier(binding.Name, binding.Namespace),
//		ServiceRef: models.ServiceRef{
//			ResourceIdentifier: models.NewResourceIdentifier(binding.ServiceName, binding.ServiceNamespace),
//		},
//		AddressGroupRef: models.AddressGroupRef{
//			ResourceIdentifier: models.NewResourceIdentifier(binding.AddressGroupName, binding.AddressGroupNamespace),
//		},
//	}
//
//	return &domainBinding, nil
//}
//
//// GetAddressGroupPortMappingByID gets an address group port mapping by ID
//func (r *reader) GetAddressGroupPortMappingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupPortMapping, error) {
//	query := `SELECT name, namespace, access_ports FROM netguard.tbl_address_group_port_mapping WHERE name = $1 AND namespace = $2`
//
//	var mapping struct {
//		Name        string
//		Namespace   string
//		AccessPorts []byte
//	}
//
//	if err := r.conn.QueryRow(ctx, query, id.Name, id.Namespace).Scan(&mapping.Name, &mapping.Namespace, &mapping.AccessPorts); err != nil {
//		if errors.Is(err, pgx.ErrNoRows) {
//			return nil, nil // Return nil if not found
//		}
//		return nil, errors.Wrap(err, "failed to get address group port mapping")
//	}
//
//	// Convert JSON to struct
//	var accessPorts []struct {
//		Name      string `json:"name"`
//		Namespace string `json:"namespace"`
//		Ports     map[string][]struct {
//			Start int `json:"start"`
//			End   int `json:"end"`
//		} `json:"ports"`
//	}
//
//	if len(mapping.AccessPorts) > 0 {
//		if err := json.Unmarshal(mapping.AccessPorts, &accessPorts); err != nil {
//			return nil, errors.Wrap(err, "failed to unmarshal access ports")
//		}
//	}
//
//	// Convert to domain model
//	domainMapping := models.AddressGroupPortMapping{
//		ResourceIdentifier: models.NewResourceIdentifier(mapping.Name, mapping.Namespace),
//	}
//
//	for _, ap := range accessPorts {
//		spr := models.ServicePortsRef{
//			ResourceIdentifier: models.NewResourceIdentifier(ap.Name, ap.Namespace),
//			Ports:              make(models.ProtocolPorts),
//		}
//
//		for proto, ranges := range ap.Ports {
//			portRanges := make([]models.PortRange, 0, len(ranges))
//			for _, r := range ranges {
//				portRanges = append(portRanges, models.PortRange{
//					Start: r.Start,
//					End:   r.End,
//				})
//			}
//			spr.Ports[models.TransportProtocol(proto)] = portRanges
//		}
//
//		domainMapping.AccessPorts = append(domainMapping.AccessPorts, spr)
//	}
//
//	return &domainMapping, nil
//}
//
//// GetRuleS2SByID gets a rule s2s by ID
//func (r *reader) GetRuleS2SByID(ctx context.Context, id models.ResourceIdentifier) (*models.RuleS2S, error) {
//	query := `
//		SELECT name, namespace, traffic, service_local_name, service_local_namespace, service_name, service_namespace
//		FROM netguard.tbl_rule_s2s
//		WHERE name = $1 AND namespace = $2
//	`
//
//	var rule RuleS2S
//
//	if err := r.conn.QueryRow(ctx, query, id.Name, id.Namespace).Scan(
//		&rule.Name,
//		&rule.Namespace,
//		&rule.Traffic,
//		&rule.ServiceLocalName,
//		&rule.ServiceLocalNamespace,
//		&rule.ServiceName,
//		&rule.ServiceNamespace,
//	); err != nil {
//		if errors.Is(err, pgx.ErrNoRows) {
//			return nil, nil // Return nil if not found
//		}
//		return nil, errors.Wrap(err, "failed to get rule s2s")
//	}
//
//	// Convert to domain model
//	domainRule := models.RuleS2S{
//		ResourceIdentifier: models.NewResourceIdentifier(rule.Name, rule.Namespace),
//		Traffic:            models.Traffic(rule.Traffic),
//		ServiceLocalRef: models.ServiceRef{
//			ResourceIdentifier: models.NewResourceIdentifier(rule.ServiceLocalName, rule.ServiceLocalNamespace),
//		},
//		ServiceRef: models.ServiceRef{
//			ResourceIdentifier: models.NewResourceIdentifier(rule.ServiceName, rule.ServiceNamespace),
//		},
//	}
//
//	return &domainRule, nil
//}
//
//// GetServiceAliasByID gets a service alias by ID
//func (r *reader) GetServiceAliasByID(ctx context.Context, id models.ResourceIdentifier) (*models.ServiceAlias, error) {
//	query := `
//		SELECT name, namespace, service_name, service_namespace
//		FROM netguard.tbl_service_alias
//		WHERE name = $1 AND namespace = $2
//	`
//
//	var alias ServiceAlias
//
//	if err := r.conn.QueryRow(ctx, query, id.Name, id.Namespace).Scan(
//		&alias.Name,
//		&alias.Namespace,
//		&alias.ServiceName,
//		&alias.ServiceNamespace,
//	); err != nil {
//		if errors.Is(err, pgx.ErrNoRows) {
//			return nil, nil // Return nil if not found
//		}
//		return nil, errors.Wrap(err, "failed to get service alias")
//	}
//
//	// Convert to domain model
//	domainAlias := models.ServiceAlias{
//		ResourceIdentifier: models.NewResourceIdentifier(alias.Name, alias.Namespace),
//		ServiceRef: models.ServiceRef{
//			ResourceIdentifier: models.NewResourceIdentifier(alias.ServiceName, alias.ServiceNamespace),
//		},
//	}
//
//	return &domainAlias, nil
//}
//
//// GetSyncStatus gets the sync status
//func (r *reader) GetSyncStatus(ctx context.Context) (*models.SyncStatus, error) {
//	var status SyncStatus
//
//	if err := status.Load(ctx, r.conn.Conn()); err != nil {
//		if errors.Is(err, pgx.ErrNoRows) {
//			// If no rows, return empty status
//			return &models.SyncStatus{}, nil
//		}
//		return nil, errors.Wrap(err, "failed to load sync status")
//	}
//
//	return &models.SyncStatus{
//		UpdatedAt: status.UpdatedAt,
//	}, nil
//}
//
//// ListIEAgAgRules lists IEAgAgRules
//func (r *reader) ListIEAgAgRules(ctx context.Context, consume func(models.IEAgAgRule) error, scope ports.Scope) error {
//	query := `
//		SELECT name, namespace, transport, traffic, 
//			   address_group_local_name, address_group_local_namespace,
//			   address_group_name, address_group_namespace,
//			   ports, action, logs, priority
//		FROM netguard.tbl_ieagag_rule
//	`
//
//	// Add scope conditions if needed
//	args := []interface{}{}
//	if scope != nil && !scope.IsEmpty() {
//		if ris, ok := scope.(ports.ResourceIdentifierScope); ok && !ris.IsEmpty() {
//			pairs := make([][]string, 0, len(ris.Identifiers))
//			for _, id := range ris.Identifiers {
//				pairs = append(pairs, []string{id.Name, id.Namespace})
//			}
//
//			query += ` WHERE (name, namespace) = ANY($1)`
//			args = append(args, pairs)
//		}
//	}
//
//	rows, err := r.conn.Query(ctx, query, args...)
//	if err != nil {
//		return errors.Wrap(err, "failed to query IEAgAgRules")
//	}
//	defer rows.Close()
//
//	for rows.Next() {
//		var rule IEAgAgRule
//		var portsJSON []byte
//
//		if err := rows.Scan(
//			&rule.Name,
//			&rule.Namespace,
//			&rule.Transport,
//			&rule.Traffic,
//			&rule.AddressGroupLocalName,
//			&rule.AddressGroupLocalNamespace,
//			&rule.AddressGroupName,
//			&rule.AddressGroupNamespace,
//			&portsJSON,
//			&rule.Action,
//			&rule.Logs,
//			&rule.Priority,
//		); err != nil {
//			return errors.Wrap(err, "failed to scan IEAgAgRule")
//		}
//
//		// Parse ports from JSON
//		var ports []struct {
//			Source      string `json:"source"`
//			Destination string `json:"destination"`
//		}
//
//		if len(portsJSON) > 0 {
//			if err := json.Unmarshal(portsJSON, &ports); err != nil {
//				return errors.Wrap(err, "failed to unmarshal ports")
//			}
//		}
//
//		// Convert to domain model
//		domainRule := models.IEAgAgRule{
//			SelfRef: models.SelfRef{
//				ResourceIdentifier: models.NewResourceIdentifier(rule.Name, rule.Namespace),
//			},
//			Transport: models.TransportProtocol(rule.Transport),
//			Traffic:   models.Traffic(rule.Traffic),
//			AddressGroupLocal: models.AddressGroupRef{
//				ResourceIdentifier: models.NewResourceIdentifier(rule.AddressGroupLocalName, rule.AddressGroupLocalNamespace),
//			},
//			AddressGroup: models.AddressGroupRef{
//				ResourceIdentifier: models.NewResourceIdentifier(rule.AddressGroupName, rule.AddressGroupNamespace),
//			},
//			Action:   models.RuleAction(rule.Action),
//			Logs:     rule.Logs,
//			Priority: rule.Priority,
//		}
//
//		// Add ports
//		for _, p := range ports {
//			domainRule.Ports = append(domainRule.Ports, models.PortSpec{
//				Source:      p.Source,
//				Destination: p.Destination,
//			})
//		}
//
//		if err := consume(domainRule); err != nil {
//			return err
//		}
//	}
//
//	if err := rows.Err(); err != nil {
//		return errors.Wrap(err, "error iterating IEAgAgRules")
//	}
//
//	return nil
//}
//
//// GetIEAgAgRuleByID gets an IEAgAgRule by ID
//func (r *reader) GetIEAgAgRuleByID(ctx context.Context, id models.ResourceIdentifier) (*models.IEAgAgRule, error) {
//	query := `
//		SELECT name, namespace, transport, traffic, 
//			   address_group_local_name, address_group_local_namespace,
//			   address_group_name, address_group_namespace,
//			   ports, action, logs, priority
//		FROM netguard.tbl_ieagag_rule
//		WHERE name = $1 AND namespace = $2
//	`
//
//	var rule IEAgAgRule
//	var portsJSON []byte
//
//	if err := r.conn.QueryRow(ctx, query, id.Name, id.Namespace).Scan(
//		&rule.Name,
//		&rule.Namespace,
//		&rule.Transport,
//		&rule.Traffic,
//		&rule.AddressGroupLocalName,
//		&rule.AddressGroupLocalNamespace,
//		&rule.AddressGroupName,
//		&rule.AddressGroupNamespace,
//		&portsJSON,
//		&rule.Action,
//		&rule.Logs,
//		&rule.Priority,
//	); err != nil {
//		if errors.Is(err, pgx.ErrNoRows) {
//			return nil, nil // Return nil if not found
//		}
//		return nil, errors.Wrap(err, "failed to get IEAgAgRule")
//	}
//
//	// Parse ports from JSON
//	var ports []struct {
//		Source      string `json:"source"`
//		Destination string `json:"destination"`
//	}
//
//	if len(portsJSON) > 0 {
//		if err := json.Unmarshal(portsJSON, &ports); err != nil {
//			return nil, errors.Wrap(err, "failed to unmarshal ports")
//		}
//	}
//
//	// Convert to domain model
//	domainRule := models.IEAgAgRule{
//		SelfRef: models.SelfRef{
//			ResourceIdentifier: models.NewResourceIdentifier(rule.Name, rule.Namespace),
//		},
//		Transport: models.TransportProtocol(rule.Transport),
//		Traffic:   models.Traffic(rule.Traffic),
//		AddressGroupLocal: models.AddressGroupRef{
//			ResourceIdentifier: models.NewResourceIdentifier(rule.AddressGroupLocalName, rule.AddressGroupLocalNamespace),
//		},
//		AddressGroup: models.AddressGroupRef{
//			ResourceIdentifier: models.NewResourceIdentifier(rule.AddressGroupName, rule.AddressGroupNamespace),
//		},
//		Action:   models.RuleAction(rule.Action),
//		Logs:     rule.Logs,
//		Priority: rule.Priority,
//	}
//
//	// Add ports
//	for _, p := range ports {
//		domainRule.Ports = append(domainRule.Ports, models.PortSpec{
//			Source:      p.Source,
//			Destination: p.Destination,
//		})
//	}
//
//	return &domainRule, nil
//}

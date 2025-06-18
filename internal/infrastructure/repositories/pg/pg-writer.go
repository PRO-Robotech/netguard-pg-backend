package pg

//
//import (
//	"context"
//	"encoding/json"
//	"time"
//
//	"github.com/jackc/pgx/v5"
//	"github.com/jackc/pgx/v5/pgxpool"
//	"github.com/pkg/errors"
//
//	"netguard-pg-backend/internal/domain/models"
//	"netguard-pg-backend/internal/domain/ports"
//)
//
//// writer implements the ports.Writer interface for PostgreSQL
//type writer struct {
//	registry *Registry
//	conn     *pgxpool.Conn
//	tx       pgx.Tx
//	ctx      context.Context
//
//	totalAffectedRows int64
//}
//
//// SyncServices syncs services
//func (w *writer) SyncServices(ctx context.Context, services []models.Service, scope ports.Scope, opts ...ports.Option) error {
//	// If scope is not empty, delete existing records
//	if !scope.IsEmpty() {
//		if rs, ok := scope.(ports.ResourceIdentifierScope); ok && len(rs.Identifiers) > 0 {
//			// Create array of name-namespace pairs
//			pairs := make([][]string, 0, len(rs.Identifiers))
//			for _, id := range rs.Identifiers {
//				pairs = append(pairs, []string{id.Name, id.Namespace})
//			}
//
//			query := `DELETE FROM netguard.tbl_service WHERE (name, namespace) = ANY($1)`
//			result, err := w.tx.Exec(ctx, query, pairs)
//			if err != nil {
//				return errors.Wrap(err, "failed to delete services")
//			}
//			w.totalAffectedRows += result.RowsAffected()
//		}
//	}
//
//	// Insert new records
//	for _, service := range services {
//		// Convert IngressPorts to JSON
//		ingressPorts := make([]map[string]interface{}, 0, len(service.IngressPorts))
//		for _, p := range service.IngressPorts {
//			ingressPorts = append(ingressPorts, map[string]interface{}{
//				"protocol":    string(p.Protocol),
//				"port":        p.Port,
//				"description": p.Description,
//			})
//		}
//
//		ingressPortsJSON, err := json.Marshal(ingressPorts)
//		if err != nil {
//			return errors.Wrap(err, "failed to marshal ingress ports")
//		}
//
//		// Insert service
//		query := `
//			INSERT INTO netguard.tbl_service (name, namespace, description, ingress_ports)
//			VALUES ($1, $2, $3, $4)
//			ON CONFLICT (name, namespace) DO UPDATE
//			SET description = $3, ingress_ports = $4
//		`
//
//		result, err := w.tx.Exec(ctx, query, service.Name, service.Namespace, service.Description, ingressPortsJSON)
//		if err != nil {
//			return errors.Wrap(err, "failed to insert service")
//		}
//		w.totalAffectedRows += result.RowsAffected()
//	}
//
//	return nil
//}
//
//// SyncAddressGroups syncs address groups
//func (w *writer) SyncAddressGroups(ctx context.Context, addressGroups []models.AddressGroup, scope ports.Scope, opts ...ports.Option) error {
//	// If scope is not empty, delete existing records
//	if !scope.IsEmpty() {
//		if rs, ok := scope.(ports.ResourceIdentifierScope); ok && len(rs.Identifiers) > 0 {
//			// Create array of name-namespace pairs
//			pairs := make([][]string, 0, len(rs.Identifiers))
//			for _, id := range rs.Identifiers {
//				pairs = append(pairs, []string{id.Name, id.Namespace})
//			}
//
//			query := `DELETE FROM netguard.tbl_address_group WHERE (name, namespace) = ANY($1)`
//			result, err := w.tx.Exec(ctx, query, pairs)
//			if err != nil {
//				return errors.Wrap(err, "failed to delete address groups")
//			}
//			w.totalAffectedRows += result.RowsAffected()
//		}
//	}
//
//	// Insert new records
//	for _, addressGroup := range addressGroups {
//		query := `
//			INSERT INTO netguard.tbl_address_group (name, namespace, description, addresses)
//			VALUES ($1, $2, $3, $4)
//			ON CONFLICT (name, namespace) DO UPDATE
//			SET description = $3, addresses = $4
//		`
//
//		result, err := w.tx.Exec(ctx, query, addressGroup.Name, addressGroup.Namespace, addressGroup.Description, addressGroup.Addresses)
//		if err != nil {
//			return errors.Wrap(err, "failed to insert address group")
//		}
//		w.totalAffectedRows += result.RowsAffected()
//	}
//
//	return nil
//}
//
//// SyncAddressGroupBindings syncs address group bindings
//func (w *writer) SyncAddressGroupBindings(ctx context.Context, bindings []models.AddressGroupBinding, scope ports.Scope, opts ...ports.Option) error {
//	// If scope is not empty, delete existing records
//	if !scope.IsEmpty() {
//		if rs, ok := scope.(ports.ResourceIdentifierScope); ok && len(rs.Identifiers) > 0 {
//			// Create array of name-namespace pairs
//			pairs := make([][]string, 0, len(rs.Identifiers))
//			for _, id := range rs.Identifiers {
//				pairs = append(pairs, []string{id.Name, id.Namespace})
//			}
//
//			query := `DELETE FROM netguard.tbl_address_group_binding WHERE (name, namespace) = ANY($1)`
//			result, err := w.tx.Exec(ctx, query, pairs)
//			if err != nil {
//				return errors.Wrap(err, "failed to delete address group bindings")
//			}
//			w.totalAffectedRows += result.RowsAffected()
//		}
//	}
//
//	// Insert new records
//	for _, binding := range bindings {
//		query := `
//			INSERT INTO netguard.tbl_address_group_binding (
//				name, namespace, service_name, service_namespace, address_group_name, address_group_namespace
//			)
//			VALUES ($1, $2, $3, $4, $5, $6)
//			ON CONFLICT (name, namespace) DO UPDATE
//			SET service_name = $3, service_namespace = $4, address_group_name = $5, address_group_namespace = $6
//		`
//
//		result, err := w.tx.Exec(ctx, query,
//			binding.Name,
//			binding.Namespace,
//			binding.ServiceRef.Name,
//			binding.ServiceRef.Namespace,
//			binding.AddressGroupRef.Name,
//			binding.AddressGroupRef.Namespace,
//		)
//		if err != nil {
//			return errors.Wrap(err, "failed to insert address group binding")
//		}
//		w.totalAffectedRows += result.RowsAffected()
//	}
//
//	return nil
//}
//
//// SyncAddressGroupPortMappings syncs address group port mappings
//func (w *writer) SyncAddressGroupPortMappings(ctx context.Context, mappings []models.AddressGroupPortMapping, scope ports.Scope, opts ...ports.Option) error {
//	// If scope is not empty, delete existing records
//	if !scope.IsEmpty() {
//		if rs, ok := scope.(ports.ResourceIdentifierScope); ok && len(rs.Identifiers) > 0 {
//			// Create array of name-namespace pairs
//			pairs := make([][]string, 0, len(rs.Identifiers))
//			for _, id := range rs.Identifiers {
//				pairs = append(pairs, []string{id.Name, id.Namespace})
//			}
//
//			query := `DELETE FROM netguard.tbl_address_group_port_mapping WHERE (name, namespace) = ANY($1)`
//			result, err := w.tx.Exec(ctx, query, pairs)
//			if err != nil {
//				return errors.Wrap(err, "failed to delete address group port mappings")
//			}
//			w.totalAffectedRows += result.RowsAffected()
//		}
//	}
//
//	// Insert new records
//	for _, mapping := range mappings {
//		// Convert AccessPorts to JSON
//		accessPorts := make([]map[string]interface{}, 0, len(mapping.AccessPorts))
//		for _, ap := range mapping.AccessPorts {
//			ports := make(map[string][]map[string]int)
//			for proto, ranges := range ap.Ports {
//				portRanges := make([]map[string]int, 0, len(ranges))
//				for _, r := range ranges {
//					portRanges = append(portRanges, map[string]int{
//						"start": r.Start,
//						"end":   r.End,
//					})
//				}
//				ports[string(proto)] = portRanges
//			}
//
//			accessPorts = append(accessPorts, map[string]interface{}{
//				"name":      ap.Name,
//				"namespace": ap.Namespace,
//				"ports":     ports,
//			})
//		}
//
//		accessPortsJSON, err := json.Marshal(accessPorts)
//		if err != nil {
//			return errors.Wrap(err, "failed to marshal access ports")
//		}
//
//		query := `
//			INSERT INTO netguard.tbl_address_group_port_mapping (name, namespace, access_ports)
//			VALUES ($1, $2, $3)
//			ON CONFLICT (name, namespace) DO UPDATE
//			SET access_ports = $3
//		`
//
//		result, err := w.tx.Exec(ctx, query, mapping.Name, mapping.Namespace, accessPortsJSON)
//		if err != nil {
//			return errors.Wrap(err, "failed to insert address group port mapping")
//		}
//		w.totalAffectedRows += result.RowsAffected()
//	}
//
//	return nil
//}
//
//// SyncRuleS2S syncs rule s2s
//func (w *writer) SyncRuleS2S(ctx context.Context, rules []models.RuleS2S, scope ports.Scope, opts ...ports.Option) error {
//	// If scope is not empty, delete existing records
//	if !scope.IsEmpty() {
//		if rs, ok := scope.(ports.ResourceIdentifierScope); ok && len(rs.Identifiers) > 0 {
//			// Create array of name-namespace pairs
//			pairs := make([][]string, 0, len(rs.Identifiers))
//			for _, id := range rs.Identifiers {
//				pairs = append(pairs, []string{id.Name, id.Namespace})
//			}
//
//			query := `DELETE FROM netguard.tbl_rule_s2s WHERE (name, namespace) = ANY($1)`
//			result, err := w.tx.Exec(ctx, query, pairs)
//			if err != nil {
//				return errors.Wrap(err, "failed to delete rule s2s")
//			}
//			w.totalAffectedRows += result.RowsAffected()
//		}
//	}
//
//	// Insert new records
//	for _, rule := range rules {
//		query := `
//			INSERT INTO netguard.tbl_rule_s2s (
//				name, namespace, traffic, service_local_name, service_local_namespace, service_name, service_namespace
//			)
//			VALUES ($1, $2, $3, $4, $5, $6, $7)
//			ON CONFLICT (name, namespace) DO UPDATE
//			SET traffic = $3, service_local_name = $4, service_local_namespace = $5, service_name = $6, service_namespace = $7
//		`
//
//		result, err := w.tx.Exec(ctx, query,
//			rule.Name,
//			rule.Namespace,
//			rule.Traffic,
//			rule.ServiceLocalRef.Name,
//			rule.ServiceLocalRef.Namespace,
//			rule.ServiceRef.Name,
//			rule.ServiceRef.Namespace,
//		)
//		if err != nil {
//			return errors.Wrap(err, "failed to insert rule s2s")
//		}
//		w.totalAffectedRows += result.RowsAffected()
//	}
//
//	return nil
//}
//
//// SyncServiceAliases syncs service aliases
//func (w *writer) SyncServiceAliases(ctx context.Context, aliases []models.ServiceAlias, scope ports.Scope, opts ...ports.Option) error {
//	// If scope is not empty, delete existing records
//	if !scope.IsEmpty() {
//		if rs, ok := scope.(ports.ResourceIdentifierScope); ok && len(rs.Identifiers) > 0 {
//			// Create array of name-namespace pairs
//			pairs := make([][]string, 0, len(rs.Identifiers))
//			for _, id := range rs.Identifiers {
//				pairs = append(pairs, []string{id.Name, id.Namespace})
//			}
//
//			query := `DELETE FROM netguard.tbl_service_alias WHERE (name, namespace) = ANY($1)`
//			result, err := w.tx.Exec(ctx, query, pairs)
//			if err != nil {
//				return errors.Wrap(err, "failed to delete service aliases")
//			}
//			w.totalAffectedRows += result.RowsAffected()
//		}
//	}
//
//	// Insert new records
//	for _, alias := range aliases {
//		query := `
//			INSERT INTO netguard.tbl_service_alias (
//				name, namespace, service_name, service_namespace
//			)
//			VALUES ($1, $2, $3, $4)
//			ON CONFLICT (name, namespace) DO UPDATE
//			SET service_name = $3, service_namespace = $4
//		`
//
//		result, err := w.tx.Exec(ctx, query,
//			alias.Name,
//			alias.Namespace,
//			alias.ServiceRef.Name,
//			alias.ServiceRef.Namespace,
//		)
//		if err != nil {
//			return errors.Wrap(err, "failed to insert service alias")
//		}
//		w.totalAffectedRows += result.RowsAffected()
//	}
//
//	return nil
//}
//
//// DeleteServicesByIDs deletes services by IDs
//func (w *writer) DeleteServicesByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
//	// Extract names and namespaces
//	names := make([]string, 0, len(ids))
//	namespaces := make([]string, 0, len(ids))
//	for _, id := range ids {
//		names = append(names, id.Name)
//		namespaces = append(namespaces, id.Namespace)
//	}
//
//	// Delete records
//	query := `DELETE FROM netguard.tbl_service WHERE (name, namespace) = ANY($1)`
//
//	// Create array of name-namespace pairs
//	pairs := make([][]string, 0, len(ids))
//	for i := range names {
//		pairs = append(pairs, []string{names[i], namespaces[i]})
//	}
//
//	result, err := w.tx.Exec(ctx, query, pairs)
//	if err != nil {
//		return errors.Wrap(err, "failed to delete services")
//	}
//	w.totalAffectedRows += result.RowsAffected()
//
//	return nil
//}
//
//// DeleteAddressGroupsByIDs deletes address groups by IDs
//func (w *writer) DeleteAddressGroupsByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
//	// Extract names and namespaces
//	names := make([]string, 0, len(ids))
//	namespaces := make([]string, 0, len(ids))
//	for _, id := range ids {
//		names = append(names, id.Name)
//		namespaces = append(namespaces, id.Namespace)
//	}
//
//	// Delete records
//	query := `DELETE FROM netguard.tbl_address_group WHERE (name, namespace) = ANY($1)`
//
//	// Create array of name-namespace pairs
//	pairs := make([][]string, 0, len(ids))
//	for i := range names {
//		pairs = append(pairs, []string{names[i], namespaces[i]})
//	}
//
//	result, err := w.tx.Exec(ctx, query, pairs)
//	if err != nil {
//		return errors.Wrap(err, "failed to delete address groups")
//	}
//	w.totalAffectedRows += result.RowsAffected()
//
//	return nil
//}
//
//// DeleteAddressGroupBindingsByIDs deletes address group bindings by IDs
//func (w *writer) DeleteAddressGroupBindingsByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
//	// Extract names and namespaces
//	names := make([]string, 0, len(ids))
//	namespaces := make([]string, 0, len(ids))
//	for _, id := range ids {
//		names = append(names, id.Name)
//		namespaces = append(namespaces, id.Namespace)
//	}
//
//	// Delete records
//	query := `DELETE FROM netguard.tbl_address_group_binding WHERE (name, namespace) = ANY($1)`
//
//	// Create array of name-namespace pairs
//	pairs := make([][]string, 0, len(ids))
//	for i := range names {
//		pairs = append(pairs, []string{names[i], namespaces[i]})
//	}
//
//	result, err := w.tx.Exec(ctx, query, pairs)
//	if err != nil {
//		return errors.Wrap(err, "failed to delete address group bindings")
//	}
//	w.totalAffectedRows += result.RowsAffected()
//
//	return nil
//}
//
//// DeleteAddressGroupPortMappingsByIDs deletes address group port mappings by IDs
//func (w *writer) DeleteAddressGroupPortMappingsByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
//	// Extract names and namespaces
//	names := make([]string, 0, len(ids))
//	namespaces := make([]string, 0, len(ids))
//	for _, id := range ids {
//		names = append(names, id.Name)
//		namespaces = append(namespaces, id.Namespace)
//	}
//
//	// Delete records
//	query := `DELETE FROM netguard.tbl_address_group_port_mapping WHERE (name, namespace) = ANY($1)`
//
//	// Create array of name-namespace pairs
//	pairs := make([][]string, 0, len(ids))
//	for i := range names {
//		pairs = append(pairs, []string{names[i], namespaces[i]})
//	}
//
//	result, err := w.tx.Exec(ctx, query, pairs)
//	if err != nil {
//		return errors.Wrap(err, "failed to delete address group port mappings")
//	}
//	w.totalAffectedRows += result.RowsAffected()
//
//	return nil
//}
//
//// DeleteRuleS2SByIDs deletes rule s2s by IDs
//func (w *writer) DeleteRuleS2SByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
//	// Extract names and namespaces
//	names := make([]string, 0, len(ids))
//	namespaces := make([]string, 0, len(ids))
//	for _, id := range ids {
//		names = append(names, id.Name)
//		namespaces = append(namespaces, id.Namespace)
//	}
//
//	// Delete records
//	query := `DELETE FROM netguard.tbl_rule_s2s WHERE (name, namespace) = ANY($1)`
//
//	// Create array of name-namespace pairs
//	pairs := make([][]string, 0, len(ids))
//	for i := range names {
//		pairs = append(pairs, []string{names[i], namespaces[i]})
//	}
//
//	result, err := w.tx.Exec(ctx, query, pairs)
//	if err != nil {
//		return errors.Wrap(err, "failed to delete rule s2s")
//	}
//	w.totalAffectedRows += result.RowsAffected()
//
//	return nil
//}
//
//// DeleteServiceAliasesByIDs deletes service aliases by IDs
//func (w *writer) DeleteServiceAliasesByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
//	// Extract names and namespaces
//	names := make([]string, 0, len(ids))
//	namespaces := make([]string, 0, len(ids))
//	for _, id := range ids {
//		names = append(names, id.Name)
//		namespaces = append(namespaces, id.Namespace)
//	}
//
//	// Delete records
//	query := `DELETE FROM netguard.tbl_service_alias WHERE (name, namespace) = ANY($1)`
//
//	// Create array of name-namespace pairs
//	pairs := make([][]string, 0, len(ids))
//	for i := range names {
//		pairs = append(pairs, []string{names[i], namespaces[i]})
//	}
//
//	result, err := w.tx.Exec(ctx, query, pairs)
//	if err != nil {
//		return errors.Wrap(err, "failed to delete service aliases")
//	}
//	w.totalAffectedRows += result.RowsAffected()
//
//	return nil
//}
//
//// Commit commits the transaction
//func (w *writer) Commit() error {
//	// Update sync status
//	status := SyncStatus{
//		UpdatedAt:         time.Now(),
//		TotalAffectedRows: w.totalAffectedRows,
//	}
//
//	if err := status.Store(w.ctx, w.conn.Conn()); err != nil {
//		w.tx.Rollback(w.ctx)
//		w.conn.Release()
//		return errors.Wrap(err, "failed to store sync status")
//	}
//
//	// Commit transaction
//	if err := w.tx.Commit(w.ctx); err != nil {
//		w.conn.Release()
//		return errors.Wrap(err, "failed to commit transaction")
//	}
//
//	// Notify observers
//	w.registry.subj.Notify(status)
//
//	// Release connection
//	w.conn.Release()
//
//	return nil
//}
//
//// Abort aborts the transaction
//func (w *writer) Abort() {
//	w.tx.Rollback(w.ctx)
//	w.conn.Release()
//}
//
//// SyncIEAgAgRules syncs IEAgAgRules
//func (w *writer) SyncIEAgAgRules(ctx context.Context, rules []models.IEAgAgRule, scope ports.Scope, opts ...ports.Option) error {
//	// Determine sync operation (default is FullSync)
//	syncOp := models.SyncOpFullSync
//	for _, opt := range opts {
//		if so, ok := opt.(ports.SyncOption); ok {
//			syncOp = so.Operation
//		}
//	}
//
//	// Handle different sync operations
//	switch syncOp {
//	case models.SyncOpFullSync:
//		// If scope is not empty, delete existing records within scope
//		if !scope.IsEmpty() {
//			if rs, ok := scope.(ports.ResourceIdentifierScope); ok && len(rs.Identifiers) > 0 {
//				// Create array of name-namespace pairs
//				pairs := make([][]string, 0, len(rs.Identifiers))
//				for _, id := range rs.Identifiers {
//					pairs = append(pairs, []string{id.Name, id.Namespace})
//				}
//
//				query := `DELETE FROM netguard.tbl_ieagag_rule WHERE (name, namespace) = ANY($1)`
//				result, err := w.tx.Exec(ctx, query, pairs)
//				if err != nil {
//					return errors.Wrap(err, "failed to delete IEAgAgRules")
//				}
//				w.totalAffectedRows += result.RowsAffected()
//			}
//		}
//
//		// Insert or update rules
//		for _, rule := range rules {
//			if err := w.upsertIEAgAgRule(ctx, rule); err != nil {
//				return err
//			}
//		}
//
//	case models.SyncOpUpsert:
//		// Only insert or update
//		for _, rule := range rules {
//			if err := w.upsertIEAgAgRule(ctx, rule); err != nil {
//				return err
//			}
//		}
//
//	case models.SyncOpDelete:
//		// Only delete
//		for _, rule := range rules {
//			query := `DELETE FROM netguard.tbl_ieagag_rule WHERE name = $1 AND namespace = $2`
//			result, err := w.tx.Exec(ctx, query, rule.Name, rule.Namespace)
//			if err != nil {
//				return errors.Wrapf(err, "failed to delete IEAgAgRule %s/%s", rule.Namespace, rule.Name)
//			}
//			w.totalAffectedRows += result.RowsAffected()
//		}
//	}
//
//	return nil
//}
//
//// upsertIEAgAgRule inserts or updates an IEAgAgRule
//func (w *writer) upsertIEAgAgRule(ctx context.Context, rule models.IEAgAgRule) error {
//	// Convert ports to JSON
//	ports := make([]map[string]interface{}, 0, len(rule.Ports))
//	for _, p := range rule.Ports {
//		ports = append(ports, map[string]interface{}{
//			"source":      p.Source,
//			"destination": p.Destination,
//		})
//	}
//
//	portsJSON, err := json.Marshal(ports)
//	if err != nil {
//		return errors.Wrap(err, "failed to marshal ports")
//	}
//
//	// Insert or update rule
//	query := `
//		INSERT INTO netguard.tbl_ieagag_rule (
//			name, namespace, transport, traffic,
//			address_group_local_name, address_group_local_namespace,
//			address_group_name, address_group_namespace,
//			ports, action, logs, priority
//		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
//		ON CONFLICT (name, namespace) DO UPDATE
//		SET transport = $3, traffic = $4,
//			address_group_local_name = $5, address_group_local_namespace = $6,
//			address_group_name = $7, address_group_namespace = $8,
//			ports = $9, action = $10, logs = $11, priority = $12
//	`
//
//	result, err := w.tx.Exec(ctx, query,
//		rule.Name, rule.Namespace, string(rule.Transport), string(rule.Traffic),
//		rule.AddressGroupLocal.Name, rule.AddressGroupLocal.Namespace,
//		rule.AddressGroup.Name, rule.AddressGroup.Namespace,
//		portsJSON, string(rule.Action), rule.Logs, rule.Priority,
//	)
//	if err != nil {
//		return errors.Wrapf(err, "failed to upsert IEAgAgRule %s/%s", rule.Namespace, rule.Name)
//	}
//	w.totalAffectedRows += result.RowsAffected()
//
//	return nil
//}
//
//// DeleteIEAgAgRulesByIDs deletes IEAgAgRules by IDs
//func (w *writer) DeleteIEAgAgRulesByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
//	// Create array of name-namespace pairs
//	pairs := make([][]string, 0, len(ids))
//	for _, id := range ids {
//		pairs = append(pairs, []string{id.Name, id.Namespace})
//	}
//
//	query := `DELETE FROM netguard.tbl_ieagag_rule WHERE (name, namespace) = ANY($1)`
//	result, err := w.tx.Exec(ctx, query, pairs)
//	if err != nil {
//		return errors.Wrap(err, "failed to delete IEAgAgRules")
//	}
//	w.totalAffectedRows += result.RowsAffected()
//
//	return nil
//}

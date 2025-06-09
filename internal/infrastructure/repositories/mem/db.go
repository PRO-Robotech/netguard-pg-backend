package mem

import (
	"sync"

	"netguard-pg-backend/internal/domain/models"
)

// MemDB in-memory database
type MemDB struct {
	services                 map[string]models.Service
	addressGroups            map[string]models.AddressGroup
	addressGroupBindings     map[string]models.AddressGroupBinding
	addressGroupPortMappings map[string]models.AddressGroupPortMapping
	ruleS2S                  map[string]models.RuleS2S
	syncStatus               models.SyncStatus
	mu                       sync.RWMutex
}

// NewMemDB creates a new in-memory database
func NewMemDB() *MemDB {
	return &MemDB{
		services:                 make(map[string]models.Service),
		addressGroups:            make(map[string]models.AddressGroup),
		addressGroupBindings:     make(map[string]models.AddressGroupBinding),
		addressGroupPortMappings: make(map[string]models.AddressGroupPortMapping),
		ruleS2S:                  make(map[string]models.RuleS2S),
	}
}

// GetServices returns all services
func (db *MemDB) GetServices() map[string]models.Service {
	db.mu.RLock()
	defer db.mu.RUnlock()
	result := make(map[string]models.Service, len(db.services))
	for k, v := range db.services {
		result[k] = v
	}
	return result
}

// GetAddressGroups returns all address groups
func (db *MemDB) GetAddressGroups() map[string]models.AddressGroup {
	db.mu.RLock()
	defer db.mu.RUnlock()
	result := make(map[string]models.AddressGroup, len(db.addressGroups))
	for k, v := range db.addressGroups {
		result[k] = v
	}
	return result
}

// GetAddressGroupBindings returns all address group bindings
func (db *MemDB) GetAddressGroupBindings() map[string]models.AddressGroupBinding {
	db.mu.RLock()
	defer db.mu.RUnlock()
	result := make(map[string]models.AddressGroupBinding, len(db.addressGroupBindings))
	for k, v := range db.addressGroupBindings {
		result[k] = v
	}
	return result
}

// GetAddressGroupPortMappings returns all address group port mappings
func (db *MemDB) GetAddressGroupPortMappings() map[string]models.AddressGroupPortMapping {
	db.mu.RLock()
	defer db.mu.RUnlock()
	result := make(map[string]models.AddressGroupPortMapping, len(db.addressGroupPortMappings))
	for k, v := range db.addressGroupPortMappings {
		result[k] = v
	}
	return result
}

// GetRuleS2S returns all rule s2s
func (db *MemDB) GetRuleS2S() map[string]models.RuleS2S {
	db.mu.RLock()
	defer db.mu.RUnlock()
	result := make(map[string]models.RuleS2S, len(db.ruleS2S))
	for k, v := range db.ruleS2S {
		result[k] = v
	}
	return result
}

// GetSyncStatus returns the sync status
func (db *MemDB) GetSyncStatus() models.SyncStatus {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.syncStatus
}

// SetSyncStatus sets the sync status
func (db *MemDB) SetSyncStatus(status models.SyncStatus) {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.syncStatus = status
}

// SetServices sets the services
func (db *MemDB) SetServices(services map[string]models.Service) {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.services = services
}

// SetAddressGroups sets the address groups
func (db *MemDB) SetAddressGroups(addressGroups map[string]models.AddressGroup) {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.addressGroups = addressGroups
}

// SetAddressGroupBindings sets the address group bindings
func (db *MemDB) SetAddressGroupBindings(bindings map[string]models.AddressGroupBinding) {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.addressGroupBindings = bindings
}

// SetAddressGroupPortMappings sets the address group port mappings
func (db *MemDB) SetAddressGroupPortMappings(mappings map[string]models.AddressGroupPortMapping) {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.addressGroupPortMappings = mappings
}

// SetRuleS2S sets the rule s2s
func (db *MemDB) SetRuleS2S(rules map[string]models.RuleS2S) {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.ruleS2S = rules
}

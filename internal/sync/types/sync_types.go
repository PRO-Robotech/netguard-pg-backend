package types

// SyncOperation defines the type of synchronization operation
type SyncOperation string

const (
	// SyncOperationNoOp - no operation
	SyncOperationNoOp SyncOperation = "NoOp"

	// SyncOperationFullSync - full synchronization (delete + insert + update)
	SyncOperationFullSync SyncOperation = "FullSync"

	// SyncOperationUpsert - insert and update only
	SyncOperationUpsert SyncOperation = "Upsert"

	// SyncOperationDelete - delete only
	SyncOperationDelete SyncOperation = "Delete"
)

// SyncSubjectType defines the type of entity being synchronized
type SyncSubjectType string

const (
	// Existing types from provider
	SyncSubjectTypeGroups        SyncSubjectType = "Groups"
	SyncSubjectTypeNetworks      SyncSubjectType = "Networks"
	SyncSubjectTypeRules         SyncSubjectType = "Rules"
	SyncSubjectTypeIEAgAgRules   SyncSubjectType = "IEAgAgRules"
	SyncSubjectTypeIECidrAgRules SyncSubjectType = "IECidrAgRules"

	// New types for backend
	SyncSubjectTypeServices             SyncSubjectType = "Services"
	SyncSubjectTypeServiceAliases       SyncSubjectType = "ServiceAliases"
	SyncSubjectTypeRulesS2S             SyncSubjectType = "RulesS2S"
	SyncSubjectTypeAddressGroupBindings SyncSubjectType = "AddressGroupBindings"
	SyncSubjectTypeNetworkBindings      SyncSubjectType = "NetworkBindings"
	SyncSubjectTypeAgents               SyncSubjectType = "Agents"
	SyncSubjectTypeAgentBindings        SyncSubjectType = "AgentBindings"
)

// SyncRequest represents a synchronization request
type SyncRequest struct {
	Operation   SyncOperation   `json:"operation"`
	SubjectType SyncSubjectType `json:"subjectType"`
	Data        interface{}     `json:"data"`
}

// SyncResponse represents a synchronization response
type SyncResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

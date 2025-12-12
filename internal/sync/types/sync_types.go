package types

// SyncOperation defines the type of synchronization operation
type SyncOperation string

const (
	// SyncOperationNoOp - no operation
	SyncOperationNoOp SyncOperation = "NoOp"

	// SyncOperationFullSync - full synchronization (delete + insert + update)
	SyncOperationFullSync SyncOperation = "FullSync"

	// SyncOperationUpsert - insert and update only (for creation operations)
	SyncOperationUpsert SyncOperation = "Upsert"

	// SyncOperationUpdate - update only (for existing resources)
	SyncOperationUpdate SyncOperation = "Update"

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
	SyncSubjectTypeIECidrAgRules SyncSubjectType = "IECidrAgRules"

	// New types for backend
	SyncSubjectTypeServices             SyncSubjectType = "Services"
	SyncSubjectTypeAddressGroupBindings SyncSubjectType = "AddressGroupBindings"
	SyncSubjectTypeNetworkBindings      SyncSubjectType = "NetworkBindings"
	SyncSubjectTypeHosts                SyncSubjectType = "Hosts"
	SyncSubjectTypeHostBindings         SyncSubjectType = "HostBindings"
	SyncSubjectTypeSvcSvcRules          SyncSubjectType = "SvcSvcRules"
	SyncSubjectTypeSvcFqdnRules         SyncSubjectType = "SvcFqdnRules"
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

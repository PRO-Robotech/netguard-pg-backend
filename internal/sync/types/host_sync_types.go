package types

// HostSyncRequest represents a request to synchronize hosts
type HostSyncRequest struct {
	// HostUUIDs contains the UUIDs of hosts to synchronize
	HostUUIDs []string `json:"host_uuids"`

	// Namespace specifies the namespace to synchronize (empty means all namespaces)
	Namespace string `json:"namespace,omitempty"`

	// ForceSync bypasses debouncing and forces immediate synchronization
	ForceSync bool `json:"force_sync,omitempty"`

	// BatchSize specifies the number of hosts to process in each batch
	BatchSize int `json:"batch_size,omitempty"`
}

// HostSyncResult represents the result of host synchronization
type HostSyncResult struct {
	// SyncedHostUUIDs contains the UUIDs of hosts that were successfully synchronized
	SyncedHostUUIDs []string `json:"synced_host_uuids"`

	// FailedUUIDs contains the UUIDs of hosts that failed to synchronize
	FailedUUIDs []string `json:"failed_uuids"`

	// Errors maps host UUIDs to their synchronization error messages
	Errors map[string]string `json:"errors,omitempty"`

	// TotalRequested is the total number of hosts requested for synchronization
	TotalRequested int `json:"total_requested"`

	// TotalSynced is the total number of hosts successfully synchronized
	TotalSynced int `json:"total_synced"`

	// TotalFailed is the total number of hosts that failed to synchronize
	TotalFailed int `json:"total_failed"`

	// Details contains additional information about the synchronization
	Details map[string]interface{} `json:"details,omitempty"`
}

// HostIPSetUpdate represents an update to a host's IPSet
type HostIPSetUpdate struct {
	// HostUUID is the UUID of the host to update
	HostUUID string `json:"host_uuid"`

	// HostID is the internal ID of the host in NETGUARD
	HostID string `json:"host_id"`

	// Namespace is the namespace of the host
	Namespace string `json:"namespace"`

	// Name is the name of the host
	Name string `json:"name"`

	// IPSet contains the new IP addresses for the host
	IPSet []string `json:"ip_set"`

	// SGName is the security group name from SGROUP
	SGName string `json:"sg_name,omitempty"`
}

// NewHostSyncRequest creates a new HostSyncRequest with default values
func NewHostSyncRequest(namespace string, hostUUIDs []string) *HostSyncRequest {
	return &HostSyncRequest{
		HostUUIDs: hostUUIDs,
		Namespace: namespace,
		ForceSync: false,
		BatchSize: 50, // Default batch size
	}
}

// NewHostSyncResult creates a new HostSyncResult
func NewHostSyncResult() *HostSyncResult {
	return &HostSyncResult{
		SyncedHostUUIDs: make([]string, 0),
		FailedUUIDs:     make([]string, 0),
		Errors:          make(map[string]string),
		Details:         make(map[string]interface{}),
	}
}

// AddSyncedHost adds a successfully synchronized host UUID to the result
func (r *HostSyncResult) AddSyncedHost(hostUUID string) {
	r.SyncedHostUUIDs = append(r.SyncedHostUUIDs, hostUUID)
	r.TotalSynced++
}

// AddFailedHost adds a failed host to the result
func (r *HostSyncResult) AddFailedHost(uuid string, errorMsg string) {
	r.FailedUUIDs = append(r.FailedUUIDs, uuid)
	if errorMsg != "" {
		if r.Errors == nil {
			r.Errors = make(map[string]string)
		}
		r.Errors[uuid] = errorMsg
	}
	r.TotalFailed++
}

// SetTotalRequested sets the total number of hosts requested for synchronization
func (r *HostSyncResult) SetTotalRequested(total int) {
	r.TotalRequested = total
}

// HasErrors returns true if there are any synchronization errors
func (r *HostSyncResult) HasErrors() bool {
	return r.TotalFailed > 0
}

// GetError returns the error message for a specific host UUID
func (r *HostSyncResult) GetError(uuid string) string {
	if r.Errors == nil {
		return ""
	}
	return r.Errors[uuid]
}

// SetDetail sets a detail value
func (r *HostSyncResult) SetDetail(key string, value interface{}) {
	if r.Details == nil {
		r.Details = make(map[string]interface{})
	}
	r.Details[key] = value
}

// GetDetail gets a detail value
func (r *HostSyncResult) GetDetail(key string) interface{} {
	if r.Details == nil {
		return nil
	}
	return r.Details[key]
}

// IsEmpty returns true if no hosts were processed
func (r *HostSyncResult) IsEmpty() bool {
	return r.TotalSynced == 0 && r.TotalFailed == 0
}

// SuccessRate returns the success rate as a percentage (0-100)
func (r *HostSyncResult) SuccessRate() float64 {
	if r.TotalRequested == 0 {
		return 100.0
	}
	return float64(r.TotalSynced) / float64(r.TotalRequested) * 100.0
}

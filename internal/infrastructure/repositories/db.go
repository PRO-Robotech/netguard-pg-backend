package repositories

// TableID memory table ID
type TableID int

const (
	// TblServices table 'services'
	TblServices TableID = iota

	// TblAddressGroups table 'address groups'
	TblAddressGroups

	// TblAddressGroupBindings table 'address group bindings'
	TblAddressGroupBindings

	// TblAddressGroupPortMappings table 'address group port mappings'
	TblAddressGroupPortMappings

	// TblRuleS2S table 'rule s2s'
	TblRuleS2S

	// TblSyncStatus table 'sync-status'
	TblSyncStatus

	// TblServiceAliases table 'service aliases'
	TblServiceAliases

	// TblNetworks table 'networks'
	TblNetworks

	// TblNetworkBindings table 'network bindings'
	TblNetworkBindings
)

// SchemaName database scheme name
const SchemaName = "netguard"

// String stringer interface impl
func (tid TableID) String() string {
	return tableID2string[tid]
}

var tableID2string = map[TableID]string{
	TblServices:                 "tbl_service",
	TblServiceAliases:           "tbl_service_alias",
	TblAddressGroups:            "tbl_address_group",
	TblAddressGroupBindings:     "tbl_address_group_binding",
	TblAddressGroupPortMappings: "tbl_address_group_port_mapping",
	TblRuleS2S:                  "tbl_rule_s2s",
	TblSyncStatus:               "tbl_sync_status",
	TblNetworks:                 "tbl_network",
	TblNetworkBindings:          "tbl_network_binding",
}

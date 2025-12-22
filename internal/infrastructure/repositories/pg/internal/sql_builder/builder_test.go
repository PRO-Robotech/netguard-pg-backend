package sql_builder

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"netguard-pg-backend/internal/domain/models"
	netguardpb "netguard-pg-backend/protos/pkg/api/netguard"
)

func TestBuildCombinedWHERE(t *testing.T) {
	builder := NewSQLBuilder()

	tests := []struct {
		name           string
		table          string
		tableAlias     string
		identifiers    []models.ResourceIdentifier
		fieldSelectors []*netguardpb.FieldSelector
		startParamNum  int
		wantClause     string
		wantArgs       []interface{}
		wantErr        bool
	}{
		{
			name:       "empty - no filters",
			table:      "hosts",
			tableAlias: "h",
			wantClause: "",
			wantArgs:   nil,
			wantErr:    false,
		},
		{
			name:       "only identifiers - single",
			table:      "hosts",
			tableAlias: "h",
			identifiers: []models.ResourceIdentifier{
				{Name: "host-1", Namespace: "prod"},
			},
			startParamNum: 1,
			wantClause:    "((h.namespace = $1 AND h.name = $2))",
			wantArgs:      []interface{}{"prod", "host-1"},
			wantErr:       false,
		},
		{
			name:       "only identifiers - multiple (OR logic)",
			table:      "hosts",
			tableAlias: "h",
			identifiers: []models.ResourceIdentifier{
				{Name: "host-1", Namespace: "prod"},
				{Name: "host-2", Namespace: "dev"},
			},
			startParamNum: 1,
			wantClause:    "((h.namespace = $1 AND h.name = $2) OR (h.namespace = $3 AND h.name = $4))",
			wantArgs:      []interface{}{"prod", "host-1", "dev", "host-2"},
			wantErr:       false,
		},
		{
			name:       "only field selectors - single",
			table:      "hosts",
			tableAlias: "h",
			fieldSelectors: []*netguardpb.FieldSelector{
				{
					Field:    "status.addressGroupRef.name",
					Operator: netguardpb.FieldOperator_FIELD_OPERATOR_EQUALS,
					Value:    "prod-ag",
				},
			},
			startParamNum: 1,
			wantClause:    "(h.address_group_ref_name = $1)",
			wantArgs:      []interface{}{"prod-ag"},
			wantErr:       false,
		},
		{
			name:       "only field selectors - multiple (AND logic)",
			table:      "hosts",
			tableAlias: "h",
			fieldSelectors: []*netguardpb.FieldSelector{
				{
					Field:    "metadata.namespace",
					Operator: netguardpb.FieldOperator_FIELD_OPERATOR_EQUALS,
					Value:    "prod",
				},
				{
					Field:    "status.isBound",
					Operator: netguardpb.FieldOperator_FIELD_OPERATOR_EQUALS,
					Value:    "true",
				},
			},
			startParamNum: 1,
			wantClause:    "(h.namespace = $1 AND h.is_bound = $2)",
			wantArgs:      []interface{}{"prod", "true"},
			wantErr:       false,
		},
		{
			name:       "combined - identifiers AND field selectors",
			table:      "hosts",
			tableAlias: "h",
			identifiers: []models.ResourceIdentifier{
				{Name: "host-1", Namespace: "prod"},
			},
			fieldSelectors: []*netguardpb.FieldSelector{
				{
					Field:    "status.isBound",
					Operator: netguardpb.FieldOperator_FIELD_OPERATOR_EQUALS,
					Value:    "true",
				},
			},
			startParamNum: 1,
			wantClause:    "((h.namespace = $1 AND h.name = $2)) AND (h.is_bound = $3)",
			wantArgs:      []interface{}{"prod", "host-1", "true"},
			wantErr:       false,
		},
		{
			name:       "combined - multiple identifiers + multiple field selectors",
			table:      "hosts",
			tableAlias: "h",
			identifiers: []models.ResourceIdentifier{
				{Name: "host-1", Namespace: "prod"},
				{Name: "host-2", Namespace: "prod"},
			},
			fieldSelectors: []*netguardpb.FieldSelector{
				{
					Field:    "status.addressGroupRef.name",
					Operator: netguardpb.FieldOperator_FIELD_OPERATOR_EQUALS,
					Value:    "prod-ag",
				},
				{
					Field:    "status.isBound",
					Operator: netguardpb.FieldOperator_FIELD_OPERATOR_EQUALS,
					Value:    "true",
				},
			},
			startParamNum: 1,
			wantClause:    "((h.namespace = $1 AND h.name = $2) OR (h.namespace = $3 AND h.name = $4)) AND (h.address_group_ref_name = $5 AND h.is_bound = $6)",
			wantArgs:      []interface{}{"prod", "host-1", "prod", "host-2", "prod-ag", "true"},
			wantErr:       false,
		},
		{
			name:       "namespace-only identifier",
			table:      "hosts",
			tableAlias: "h",
			identifiers: []models.ResourceIdentifier{
				{Name: "", Namespace: "prod"},
			},
			startParamNum: 1,
			wantClause:    "(h.namespace = $1)",
			wantArgs:      []interface{}{"prod"},
			wantErr:       false,
		},
		{
			name:       "not equals operator",
			table:      "hosts",
			tableAlias: "h",
			fieldSelectors: []*netguardpb.FieldSelector{
				{
					Field:    "metadata.namespace",
					Operator: netguardpb.FieldOperator_FIELD_OPERATOR_NOT_EQUALS,
					Value:    "dev",
				},
			},
			startParamNum: 1,
			wantClause:    "(h.namespace != $1)",
			wantArgs:      []interface{}{"dev"},
			wantErr:       false,
		},
		{
			name:       "address_groups table",
			table:      "address_groups",
			tableAlias: "ag",
			fieldSelectors: []*netguardpb.FieldSelector{
				{
					Field:    "spec.defaultAction",
					Operator: netguardpb.FieldOperator_FIELD_OPERATOR_EQUALS,
					Value:    "ACCEPT",
				},
			},
			startParamNum: 1,
			wantClause:    "(ag.default_action = $1)",
			wantArgs:      []interface{}{"ACCEPT"},
			wantErr:       false,
		},
		{
			name:       "services table",
			table:      "services",
			tableAlias: "s",
			fieldSelectors: []*netguardpb.FieldSelector{
				{
					Field:    "metadata.namespace",
					Operator: netguardpb.FieldOperator_FIELD_OPERATOR_EQUALS,
					Value:    "prod",
				},
				{
					Field:    "spec.description",
					Operator: netguardpb.FieldOperator_FIELD_OPERATOR_CONTAINS,
					Value:    "web",
				},
			},
			startParamNum: 1,
			wantClause:    "(s.namespace = $1 AND s.description LIKE $2)",
			wantArgs:      []interface{}{"prod", "%web%"},
			wantErr:       false,
		},
		{
			name:       "unsupported field",
			table:      "hosts",
			tableAlias: "h",
			fieldSelectors: []*netguardpb.FieldSelector{
				{
					Field:    "status.unknown.field",
					Operator: netguardpb.FieldOperator_FIELD_OPERATOR_EQUALS,
					Value:    "value",
				},
			},
			startParamNum: 1,
			wantErr:       true,
		},
		{
			name:       "unsupported table",
			table:      "unknown_table",
			tableAlias: "t",
			fieldSelectors: []*netguardpb.FieldSelector{
				{
					Field:    "metadata.name",
					Operator: netguardpb.FieldOperator_FIELD_OPERATOR_EQUALS,
					Value:    "test",
				},
			},
			startParamNum: 1,
			wantErr:       true,
		},
		{
			name:       "svc_svc_rules table",
			table:      "svc_svc_rules",
			tableAlias: "sr",
			fieldSelectors: []*netguardpb.FieldSelector{
				{
					Field:    "spec.serviceFrom.name",
					Operator: netguardpb.FieldOperator_FIELD_OPERATOR_EQUALS,
					Value:    "frontend",
				},
			},
			startParamNum: 1,
			wantClause:    "(sr.service_from_ref->>'name' = $1)",
			wantArgs:      []interface{}{"frontend"},
			wantErr:       false,
		},
		{
			name:       "svc_fqdn_rules table",
			table:      "svc_fqdn_rules",
			tableAlias: "fr",
			fieldSelectors: []*netguardpb.FieldSelector{
				{
					Field:    "spec.serviceFrom.namespace",
					Operator: netguardpb.FieldOperator_FIELD_OPERATOR_EQUALS,
					Value:    "prod",
				},
			},
			startParamNum: 1,
			wantClause:    "(fr.service_from_ref->>'namespace' = $1)",
			wantArgs:      []interface{}{"prod"},
			wantErr:       false,
		},
		{
			name:       "ie_cidr_svc_rules table - spec.svc.name",
			table:      "ie_cidr_svc_rules",
			tableAlias: "icr",
			fieldSelectors: []*netguardpb.FieldSelector{
				{
					Field:    "spec.svc.name",
					Operator: netguardpb.FieldOperator_FIELD_OPERATOR_EQUALS,
					Value:    "frontend",
				},
			},
			startParamNum: 1,
			wantClause:    "(icr.service_ref->>'name' = $1)",
			wantArgs:      []interface{}{"frontend"},
			wantErr:       false,
		},
		{
			name:       "ie_cidr_svc_rules table - spec.cidr",
			table:      "ie_cidr_svc_rules",
			tableAlias: "icr",
			fieldSelectors: []*netguardpb.FieldSelector{
				{
					Field:    "spec.cidr",
					Operator: netguardpb.FieldOperator_FIELD_OPERATOR_EQUALS,
					Value:    "10.0.0.0/8",
				},
			},
			startParamNum: 1,
			wantClause:    "(icr.cidr = $1)",
			wantArgs:      []interface{}{"10.0.0.0/8"},
			wantErr:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clause, args, err := builder.BuildCombinedWHERE(
				tt.table,
				tt.tableAlias,
				tt.identifiers,
				tt.fieldSelectors,
				nil, // no label selectors
				tt.startParamNum,
			)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantClause, clause)
			assert.Equal(t, tt.wantArgs, args)
		})
	}
}

func TestGetFieldMapping(t *testing.T) {
	tests := []struct {
		name       string
		table      string
		field      string
		wantExists bool
		wantType   FieldType
	}{
		{
			name:       "hosts - metadata.name",
			table:      "hosts",
			field:      "metadata.name",
			wantExists: true,
			wantType:   FieldTypeColumn,
		},
		{
			name:       "hosts - status.addressGroupRef.name",
			table:      "hosts",
			field:      "status.addressGroupRef.name",
			wantExists: true,
			wantType:   FieldTypeColumn,
		},
		{
			name:       "address_groups - spec.defaultAction",
			table:      "address_groups",
			field:      "spec.defaultAction",
			wantExists: true,
			wantType:   FieldTypeColumn,
		},
		{
			name:       "non-existent field",
			table:      "hosts",
			field:      "nonexistent",
			wantExists: false,
		},
		{
			name:       "non-existent table",
			table:      "nonexistent",
			field:      "metadata.name",
			wantExists: false,
		},
		{
			name:       "svc_svc_rules - spec.serviceFrom.name",
			table:      "svc_svc_rules",
			field:      "spec.serviceFrom.name",
			wantExists: true,
			wantType:   FieldTypeColumn,
		},
		{
			name:       "svc_fqdn_rules - spec.fqdn",
			table:      "svc_fqdn_rules",
			field:      "spec.fqdn",
			wantExists: true,
			wantType:   FieldTypeColumn,
		},
		{
			name:       "ie_cidr_svc_rules - spec.cidr",
			table:      "ie_cidr_svc_rules",
			field:      "spec.cidr",
			wantExists: true,
			wantType:   FieldTypeColumn,
		},
		{
			name:       "ie_cidr_svc_rules - spec.svc.name",
			table:      "ie_cidr_svc_rules",
			field:      "spec.svc.name",
			wantExists: true,
			wantType:   FieldTypeColumn,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapping, exists := GetFieldMapping(tt.table, tt.field)
			assert.Equal(t, tt.wantExists, exists)
			if tt.wantExists {
				assert.Equal(t, tt.wantType, mapping.Type)
			}
		})
	}
}

func TestGetSupportedFields(t *testing.T) {
	tests := []struct {
		name       string
		table      string
		wantFields []string
	}{
		{
			name:  "hosts table",
			table: "hosts",
			wantFields: []string{
				"metadata.name",
				"metadata.namespace",
				"status.addressGroupRef.name",
				"status.addressGroupRef.namespace",
				"status.bindingRef.name",
				"status.bindingRef.namespace",
				"status.isBound",
			},
		},
		{
			name:  "address_groups table",
			table: "address_groups",
			wantFields: []string{
				"metadata.name",
				"metadata.namespace",
				"spec.defaultAction",
				"spec.logs",
				"spec.trace",
			},
		},
		{
			name:  "services table",
			table: "services",
			wantFields: []string{
				"metadata.name",
				"metadata.namespace",
				"spec.description",
			},
		},
		{
			name:  "networks table",
			table: "networks",
			wantFields: []string{
				"metadata.name",
				"metadata.namespace",
				"status.isBound",
				"status.bindingRef.name",
				"status.bindingRef.namespace",
				"status.addressGroupRef.name",
				"status.addressGroupRef.namespace",
			},
		},
		{
			name:       "non-existent table",
			table:      "nonexistent",
			wantFields: nil,
		},
		{
			name:  "svc_svc_rules table",
			table: "svc_svc_rules",
			wantFields: []string{
				"metadata.name",
				"metadata.namespace",
				"spec.serviceFrom.name",
				"spec.serviceFrom.namespace",
				"spec.serviceTo.name",
				"spec.serviceTo.namespace",
			},
		},
		{
			name:  "svc_fqdn_rules table",
			table: "svc_fqdn_rules",
			wantFields: []string{
				"metadata.name",
				"metadata.namespace",
				"spec.serviceFrom.name",
				"spec.serviceFrom.namespace",
				"spec.fqdn",
				"spec.description",
			},
		},
		{
			name:  "ie_cidr_svc_rules table",
			table: "ie_cidr_svc_rules",
			wantFields: []string{
				"metadata.name",
				"metadata.namespace",
				"spec.svc.name",
				"spec.svc.namespace",
				"spec.cidr",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fields := GetSupportedFields(tt.table)
			if tt.wantFields == nil {
				assert.Nil(t, fields)
				return
			}
			assert.ElementsMatch(t, tt.wantFields, fields)
		})
	}
}

func TestBuildConditionForOperator(t *testing.T) {
	builder := NewSQLBuilder()

	tests := []struct {
		name          string
		tableAlias    string
		columnName    string
		operator      netguardpb.FieldOperator
		value         string
		startParamNum int
		wantCondition string
		wantArgs      []interface{}
		wantParamNum  int
	}{
		{
			name:          "EQUALS operator",
			tableAlias:    "h",
			columnName:    "name",
			operator:      netguardpb.FieldOperator_FIELD_OPERATOR_EQUALS,
			value:         "test",
			startParamNum: 1,
			wantCondition: "h.name = $1",
			wantArgs:      []interface{}{"test"},
			wantParamNum:  2,
		},
		{
			name:          "NOT_EQUALS operator",
			tableAlias:    "h",
			columnName:    "namespace",
			operator:      netguardpb.FieldOperator_FIELD_OPERATOR_NOT_EQUALS,
			value:         "dev",
			startParamNum: 1,
			wantCondition: "h.namespace != $1",
			wantArgs:      []interface{}{"dev"},
			wantParamNum:  2,
		},
		{
			name:          "IN operator - single value",
			tableAlias:    "h",
			columnName:    "name",
			operator:      netguardpb.FieldOperator_FIELD_OPERATOR_IN,
			value:         "host-1",
			startParamNum: 1,
			wantCondition: "h.name IN ($1)",
			wantArgs:      []interface{}{"host-1"},
			wantParamNum:  2,
		},
		{
			name:          "IN operator - multiple values",
			tableAlias:    "h",
			columnName:    "name",
			operator:      netguardpb.FieldOperator_FIELD_OPERATOR_IN,
			value:         "host-1,host-2,host-3",
			startParamNum: 1,
			wantCondition: "h.name IN ($1, $2, $3)",
			wantArgs:      []interface{}{"host-1", "host-2", "host-3"},
			wantParamNum:  4,
		},
		{
			name:          "IN operator - with spaces",
			tableAlias:    "h",
			columnName:    "name",
			operator:      netguardpb.FieldOperator_FIELD_OPERATOR_IN,
			value:         "host-1, host-2, host-3",
			startParamNum: 1,
			wantCondition: "h.name IN ($1, $2, $3)",
			wantArgs:      []interface{}{"host-1", "host-2", "host-3"},
			wantParamNum:  4,
		},
		{
			name:          "NOT_IN operator",
			tableAlias:    "h",
			columnName:    "namespace",
			operator:      netguardpb.FieldOperator_FIELD_OPERATOR_NOT_IN,
			value:         "dev,test",
			startParamNum: 1,
			wantCondition: "h.namespace NOT IN ($1, $2)",
			wantArgs:      []interface{}{"dev", "test"},
			wantParamNum:  3,
		},
		{
			name:          "CONTAINS operator",
			tableAlias:    "h",
			columnName:    "name",
			operator:      netguardpb.FieldOperator_FIELD_OPERATOR_CONTAINS,
			value:         "prod",
			startParamNum: 1,
			wantCondition: "h.name LIKE $1",
			wantArgs:      []interface{}{"%prod%"},
			wantParamNum:  2,
		},
		{
			name:          "STARTS_WITH operator",
			tableAlias:    "h",
			columnName:    "name",
			operator:      netguardpb.FieldOperator_FIELD_OPERATOR_STARTS_WITH,
			value:         "prod",
			startParamNum: 1,
			wantCondition: "h.name LIKE $1",
			wantArgs:      []interface{}{"prod%"},
			wantParamNum:  2,
		},
		{
			name:          "ENDS_WITH operator",
			tableAlias:    "h",
			columnName:    "name",
			operator:      netguardpb.FieldOperator_FIELD_OPERATOR_ENDS_WITH,
			value:         "prod",
			startParamNum: 1,
			wantCondition: "h.name LIKE $1",
			wantArgs:      []interface{}{"%prod"},
			wantParamNum:  2,
		},
		{
			name:          "EXISTS operator",
			tableAlias:    "h",
			columnName:    "address_group_ref_name",
			operator:      netguardpb.FieldOperator_FIELD_OPERATOR_EXISTS,
			value:         "", // value ignored for EXISTS
			startParamNum: 1,
			wantCondition: "h.address_group_ref_name IS NOT NULL",
			wantArgs:      []interface{}{},
			wantParamNum:  1, // no increment
		},
		{
			name:          "NOT_EXISTS operator",
			tableAlias:    "h",
			columnName:    "address_group_ref_name",
			operator:      netguardpb.FieldOperator_FIELD_OPERATOR_NOT_EXISTS,
			value:         "", // value ignored for NOT_EXISTS
			startParamNum: 1,
			wantCondition: "h.address_group_ref_name IS NULL",
			wantArgs:      []interface{}{},
			wantParamNum:  1, // no increment
		},
		{
			name:          "Multiple param numbers start",
			tableAlias:    "h",
			columnName:    "name",
			operator:      netguardpb.FieldOperator_FIELD_OPERATOR_EQUALS,
			value:         "test",
			startParamNum: 5,
			wantCondition: "h.name = $5",
			wantArgs:      []interface{}{"test"},
			wantParamNum:  6,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			condition, args, paramNum := builder.buildConditionForOperator(
				tt.tableAlias,
				tt.columnName,
				tt.operator,
				tt.value,
				[]interface{}{}, // start with empty args
				tt.startParamNum,
			)

			assert.Equal(t, tt.wantCondition, condition)
			assert.Equal(t, tt.wantArgs, args)
			assert.Equal(t, tt.wantParamNum, paramNum)
		})
	}
}

func TestBuildCombinedWHERE_NewOperators(t *testing.T) {
	builder := NewSQLBuilder()

	tests := []struct {
		name           string
		table          string
		tableAlias     string
		fieldSelectors []*netguardpb.FieldSelector
		wantClause     string
		wantArgs       []interface{}
	}{
		{
			name:       "IN operator integration",
			table:      "hosts",
			tableAlias: "h",
			fieldSelectors: []*netguardpb.FieldSelector{
				{
					Field:    "metadata.namespace",
					Operator: netguardpb.FieldOperator_FIELD_OPERATOR_IN,
					Value:    "prod,dev,staging",
				},
			},
			wantClause: "(h.namespace IN ($1, $2, $3))",
			wantArgs:   []interface{}{"prod", "dev", "staging"},
		},
		{
			name:       "CONTAINS operator integration",
			table:      "hosts",
			tableAlias: "h",
			fieldSelectors: []*netguardpb.FieldSelector{
				{
					Field:    "metadata.name",
					Operator: netguardpb.FieldOperator_FIELD_OPERATOR_CONTAINS,
					Value:    "prod",
				},
			},
			wantClause: "(h.name LIKE $1)",
			wantArgs:   []interface{}{"%prod%"},
		},
		{
			name:       "EXISTS operator integration",
			table:      "hosts",
			tableAlias: "h",
			fieldSelectors: []*netguardpb.FieldSelector{
				{
					Field:    "status.addressGroupRef.name",
					Operator: netguardpb.FieldOperator_FIELD_OPERATOR_EXISTS,
					Value:    "",
				},
			},
			wantClause: "(h.address_group_ref_name IS NOT NULL)",
			wantArgs:   nil,
		},
		{
			name:       "Mixed operators - IN AND CONTAINS",
			table:      "hosts",
			tableAlias: "h",
			fieldSelectors: []*netguardpb.FieldSelector{
				{
					Field:    "metadata.namespace",
					Operator: netguardpb.FieldOperator_FIELD_OPERATOR_IN,
					Value:    "prod,dev",
				},
				{
					Field:    "metadata.name",
					Operator: netguardpb.FieldOperator_FIELD_OPERATOR_CONTAINS,
					Value:    "web",
				},
			},
			wantClause: "(h.namespace IN ($1, $2) AND h.name LIKE $3)",
			wantArgs:   []interface{}{"prod", "dev", "%web%"},
		},
		{
			name:       "Mixed operators - EXISTS AND EQUALS",
			table:      "hosts",
			tableAlias: "h",
			fieldSelectors: []*netguardpb.FieldSelector{
				{
					Field:    "status.addressGroupRef.name",
					Operator: netguardpb.FieldOperator_FIELD_OPERATOR_EXISTS,
					Value:    "",
				},
				{
					Field:    "status.isBound",
					Operator: netguardpb.FieldOperator_FIELD_OPERATOR_EQUALS,
					Value:    "true",
				},
			},
			wantClause: "(h.address_group_ref_name IS NOT NULL AND h.is_bound = $1)",
			wantArgs:   []interface{}{"true"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clause, args, err := builder.BuildCombinedWHERE(
				tt.table,
				tt.tableAlias,
				nil, // no identifiers
				tt.fieldSelectors,
				nil, // no label selectors
				1,
			)

			require.NoError(t, err)
			assert.Equal(t, tt.wantClause, clause)
			assert.Equal(t, tt.wantArgs, args)
		})
	}
}

func TestBuildLabelSelectorsWHERE(t *testing.T) {
	builder := NewSQLBuilder()

	tests := []struct {
		name           string
		labelSelectors []*netguardpb.LabelSelector
		startParamNum  int
		wantClause     string
		wantArgs       []interface{}
		wantParamNum   int
		wantErr        bool
	}{
		{
			name:           "empty label selectors",
			labelSelectors: nil,
			startParamNum:  1,
			wantClause:     "",
			wantArgs:       nil,
			wantParamNum:   1,
			wantErr:        false,
		},
		{
			name: "EQUALS operator",
			labelSelectors: []*netguardpb.LabelSelector{
				{
					Key:      "env",
					Operator: netguardpb.LabelOperator_LABEL_OPERATOR_EQUALS,
					Values:   []string{"prod"},
				},
			},
			startParamNum: 1,
			wantClause:    "m.labels->>$1 = $2",
			wantArgs:      []interface{}{"env", "prod"},
			wantParamNum:  3,
			wantErr:       false,
		},
		{
			name: "EXISTS operator",
			labelSelectors: []*netguardpb.LabelSelector{
				{
					Key:      "version",
					Operator: netguardpb.LabelOperator_LABEL_OPERATOR_EXISTS,
					Values:   []string{},
				},
			},
			startParamNum: 1,
			wantClause:    "m.labels ? $1",
			wantArgs:      []interface{}{"version"},
			wantParamNum:  2,
			wantErr:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clause, args, paramNum, err := builder.buildLabelSelectorsWHERE(
				"h",
				tt.labelSelectors,
				tt.startParamNum,
			)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantClause, clause)
			assert.Equal(t, tt.wantArgs, args)
			assert.Equal(t, tt.wantParamNum, paramNum)
		})
	}
}

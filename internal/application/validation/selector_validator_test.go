package validation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	netguardpb "netguard-pg-backend/protos/pkg/api/netguard"
)

func TestValidateFieldSelectors(t *testing.T) {
	validator := NewSelectorValidator()

	tests := []struct {
		name         string
		resourceType string
		selectors    []*netguardpb.FieldSelector
		wantErr      bool
		errContains  string
	}{
		{
			name:         "empty selectors",
			resourceType: "hosts",
			selectors:    nil,
			wantErr:      false,
		},
		{
			name:         "valid selector",
			resourceType: "hosts",
			selectors: []*netguardpb.FieldSelector{
				{
					Field:    "metadata.name",
					Operator: netguardpb.FieldOperator_FIELD_OPERATOR_EQUALS,
					Value:    "test",
				},
			},
			wantErr: false,
		},
		{
			name:         "missing field",
			resourceType: "hosts",
			selectors: []*netguardpb.FieldSelector{
				{
					Field:    "",
					Operator: netguardpb.FieldOperator_FIELD_OPERATOR_EQUALS,
					Value:    "test",
				},
			},
			wantErr:     true,
			errContains: "field is required",
		},
		{
			name:         "invalid field path",
			resourceType: "hosts",
			selectors: []*netguardpb.FieldSelector{
				{
					Field:    "metadata..name",
					Operator: netguardpb.FieldOperator_FIELD_OPERATOR_EQUALS,
					Value:    "test",
				},
			},
			wantErr:     true,
			errContains: "invalid field path",
		},
		{
			name:         "missing operator",
			resourceType: "hosts",
			selectors: []*netguardpb.FieldSelector{
				{
					Field:    "metadata.name",
					Operator: netguardpb.FieldOperator_FIELD_OPERATOR_UNSPECIFIED,
					Value:    "test",
				},
			},
			wantErr:     true,
			errContains: "operator is required",
		},
		{
			name:         "missing value for EQUALS",
			resourceType: "hosts",
			selectors: []*netguardpb.FieldSelector{
				{
					Field:    "metadata.name",
					Operator: netguardpb.FieldOperator_FIELD_OPERATOR_EQUALS,
					Value:    "",
				},
			},
			wantErr:     true,
			errContains: "value is required",
		},
		{
			name:         "EXISTS doesn't need value",
			resourceType: "hosts",
			selectors: []*netguardpb.FieldSelector{
				{
					Field:    "status.addressGroupRef.name",
					Operator: netguardpb.FieldOperator_FIELD_OPERATOR_EXISTS,
					Value:    "",
				},
			},
			wantErr: false,
		},
		{
			name:         "IN with valid values",
			resourceType: "hosts",
			selectors: []*netguardpb.FieldSelector{
				{
					Field:    "metadata.namespace",
					Operator: netguardpb.FieldOperator_FIELD_OPERATOR_IN,
					Value:    "prod,dev,staging",
				},
			},
			wantErr: false,
		},
		{
			name:         "IN with empty value",
			resourceType: "hosts",
			selectors: []*netguardpb.FieldSelector{
				{
					Field:    "metadata.namespace",
					Operator: netguardpb.FieldOperator_FIELD_OPERATOR_IN,
					Value:    "",
				},
			},
			wantErr:     true,
			errContains: "value is required",
		},
		{
			name:         "too many selectors",
			resourceType: "hosts",
			selectors: func() []*netguardpb.FieldSelector {
				selectors := make([]*netguardpb.FieldSelector, 11)
				for i := range selectors {
					selectors[i] = &netguardpb.FieldSelector{
						Field:    "metadata.name",
						Operator: netguardpb.FieldOperator_FIELD_OPERATOR_EQUALS,
						Value:    "test",
					}
				}
				return selectors
			}(),
			wantErr:     true,
			errContains: "too many field selectors",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateFieldSelectors(tt.resourceType, tt.selectors)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateLabelSelectors(t *testing.T) {
	validator := NewSelectorValidator()

	tests := []struct {
		name        string
		selectors   []*netguardpb.LabelSelector
		wantErr     bool
		errContains string
	}{
		{
			name:      "empty selectors",
			selectors: nil,
			wantErr:   false,
		},
		{
			name: "valid EQUALS selector",
			selectors: []*netguardpb.LabelSelector{
				{
					Key:      "env",
					Operator: netguardpb.LabelOperator_LABEL_OPERATOR_EQUALS,
					Values:   []string{"prod"},
				},
			},
			wantErr: false,
		},
		{
			name: "valid IN selector",
			selectors: []*netguardpb.LabelSelector{
				{
					Key:      "tier",
					Operator: netguardpb.LabelOperator_LABEL_OPERATOR_IN,
					Values:   []string{"frontend", "backend"},
				},
			},
			wantErr: false,
		},
		{
			name: "valid EXISTS selector",
			selectors: []*netguardpb.LabelSelector{
				{
					Key:      "version",
					Operator: netguardpb.LabelOperator_LABEL_OPERATOR_EXISTS,
					Values:   []string{},
				},
			},
			wantErr: false,
		},
		{
			name: "missing key",
			selectors: []*netguardpb.LabelSelector{
				{
					Key:      "",
					Operator: netguardpb.LabelOperator_LABEL_OPERATOR_EQUALS,
					Values:   []string{"prod"},
				},
			},
			wantErr:     true,
			errContains: "key is required",
		},
		{
			name: "missing operator",
			selectors: []*netguardpb.LabelSelector{
				{
					Key:      "env",
					Operator: netguardpb.LabelOperator_LABEL_OPERATOR_UNSPECIFIED,
					Values:   []string{"prod"},
				},
			},
			wantErr:     true,
			errContains: "operator is required",
		},
		{
			name: "EQUALS without value",
			selectors: []*netguardpb.LabelSelector{
				{
					Key:      "env",
					Operator: netguardpb.LabelOperator_LABEL_OPERATOR_EQUALS,
					Values:   []string{},
				},
			},
			wantErr:     true,
			errContains: "value is required",
		},
		{
			name: "EQUALS with multiple values",
			selectors: []*netguardpb.LabelSelector{
				{
					Key:      "env",
					Operator: netguardpb.LabelOperator_LABEL_OPERATOR_EQUALS,
					Values:   []string{"prod", "dev"},
				},
			},
			wantErr:     true,
			errContains: "accepts only one value",
		},
		{
			name: "IN without values",
			selectors: []*netguardpb.LabelSelector{
				{
					Key:      "tier",
					Operator: netguardpb.LabelOperator_LABEL_OPERATOR_IN,
					Values:   []string{},
				},
			},
			wantErr:     true,
			errContains: "at least one value is required",
		},
		{
			name: "invalid key - too long",
			selectors: []*netguardpb.LabelSelector{
				{
					Key:      "this-is-a-very-long-key-that-exceeds-the-maximum-length-of-63-characters",
					Operator: netguardpb.LabelOperator_LABEL_OPERATOR_EQUALS,
					Values:   []string{"prod"},
				},
			},
			wantErr:     true,
			errContains: "too long",
		},
		{
			name: "invalid value - too long",
			selectors: []*netguardpb.LabelSelector{
				{
					Key:      "env",
					Operator: netguardpb.LabelOperator_LABEL_OPERATOR_EQUALS,
					Values:   []string{"this-is-a-very-long-value-that-exceeds-the-maximum-length-of-63-characters"},
				},
			},
			wantErr:     true,
			errContains: "too long",
		},
		{
			name: "invalid key - special characters",
			selectors: []*netguardpb.LabelSelector{
				{
					Key:      "env@prod",
					Operator: netguardpb.LabelOperator_LABEL_OPERATOR_EQUALS,
					Values:   []string{"prod"},
				},
			},
			wantErr:     true,
			errContains: "invalid characters",
		},
		{
			name: "too many selectors",
			selectors: func() []*netguardpb.LabelSelector {
				selectors := make([]*netguardpb.LabelSelector, 11)
				for i := range selectors {
					selectors[i] = &netguardpb.LabelSelector{
						Key:      "env",
						Operator: netguardpb.LabelOperator_LABEL_OPERATOR_EQUALS,
						Values:   []string{"prod"},
					}
				}
				return selectors
			}(),
			wantErr:     true,
			errContains: "too many label selectors",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateLabelSelectors(tt.selectors)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateLabelKey(t *testing.T) {
	tests := []struct {
		name        string
		key         string
		wantErr     bool
		errContains string
	}{
		{
			name:    "valid key",
			key:     "env",
			wantErr: false,
		},
		{
			name:    "valid key with dashes",
			key:     "app-name",
			wantErr: false,
		},
		{
			name:    "valid key with underscores",
			key:     "app_name",
			wantErr: false,
		},
		{
			name:    "valid key with dots",
			key:     "app.kubernetes.io_name",
			wantErr: false,
		},
		{
			name:        "empty key",
			key:         "",
			wantErr:     true,
			errContains: "cannot be empty",
		},
		{
			name:        "key too long",
			key:         "this-is-a-very-long-key-name-that-exceeds-the-maximum-allowed-length-of-63-characters",
			wantErr:     true,
			errContains: "too long",
		},
		{
			name:        "key with invalid characters",
			key:         "app@name",
			wantErr:     true,
			errContains: "invalid characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateLabelKey(tt.key)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateLabelValue(t *testing.T) {
	tests := []struct {
		name        string
		value       string
		wantErr     bool
		errContains string
	}{
		{
			name:    "valid value",
			value:   "prod",
			wantErr: false,
		},
		{
			name:    "empty value (allowed)",
			value:   "",
			wantErr: false,
		},
		{
			name:    "value with dashes",
			value:   "prod-environment",
			wantErr: false,
		},
		{
			name:        "value too long",
			value:       "this-is-a-very-long-value-that-exceeds-the-maximum-allowed-length-of-63-characters",
			wantErr:     true,
			errContains: "too long",
		},
		{
			name:        "value with invalid characters",
			value:       "prod@123",
			wantErr:     true,
			errContains: "invalid characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateLabelValue(tt.value)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

package converters

import (
	"testing"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	netguardpb "netguard-pg-backend/protos/pkg/api/netguard"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestConvertSvcSvcRule_NilSafety(t *testing.T) {
	result := ConvertSvcSvcRule(nil)
	assert.Equal(t, models.SvcSvcRule{}, result, "should return empty struct for nil input")
}

func TestConvertSvcSvcRule_FullConversion(t *testing.T) {
	// Arrange
	pbRule := &netguardpb.SvcSvcRule{
		SelfRef: &netguardpb.ResourceIdentifier{
			Name:      "rule-001",
			Namespace: "test-ns",
		},
		ServiceFrom: &netguardpb.ResourceIdentifier{
			Name:      "service-a",
			Namespace: "test-ns",
		},
		ServiceTo: &netguardpb.ResourceIdentifier{
			Name:      "service-b",
			Namespace: "test-ns",
		},
		Action:      netguardpb.RuleAction_ACCEPT,
		Description: "Test rule description",
		Priority:    100,
		Logs:        true,
		Trace:       false,
		Meta: &netguardpb.Meta{
			Uid:             "test-uid",
			ResourceVersion: "12345",
			Generation:      1,
		},
	}

	// Act
	result := ConvertSvcSvcRule(pbRule)

	// Assert
	assert.Equal(t, "rule-001", result.Name)
	assert.Equal(t, "test-ns", result.Namespace)

	// Check ServiceFrom reference
	assert.Equal(t, "service-a", result.ServiceFromRef.Name)
	assert.Equal(t, "test-ns", result.ServiceFromRef.Namespace)
	assert.Equal(t, NetguardAPIVersion, result.ServiceFromRef.APIVersion)
	assert.Equal(t, KindService, result.ServiceFromRef.Kind)

	// Check ServiceTo reference
	assert.Equal(t, "service-b", result.ServiceToRef.Name)
	assert.Equal(t, "test-ns", result.ServiceToRef.Namespace)
	assert.Equal(t, NetguardAPIVersion, result.ServiceToRef.APIVersion)
	assert.Equal(t, KindService, result.ServiceToRef.Kind)

	// Check other fields
	assert.Equal(t, models.ActionAccept, result.Action)
	assert.Equal(t, "Test rule description", result.Description)
	assert.Equal(t, int32(100), result.Priority)
	assert.True(t, result.Logs)
	assert.False(t, result.Trace)

	// Check meta
	assert.Equal(t, "test-uid", result.Meta.UID)
	assert.Equal(t, "12345", result.Meta.ResourceVersion)
	assert.Equal(t, int64(1), result.Meta.Generation)
}

func TestConvertSvcSvcRule_MissingServiceRefs(t *testing.T) {
	// Arrange - proto with missing service references
	pbRule := &netguardpb.SvcSvcRule{
		SelfRef: &netguardpb.ResourceIdentifier{
			Name:      "rule-001",
			Namespace: "test-ns",
		},
		Action:   netguardpb.RuleAction_DROP,
		Priority: 50,
	}

	// Act
	result := ConvertSvcSvcRule(pbRule)

	// Assert
	assert.Equal(t, "rule-001", result.Name)
	assert.Equal(t, models.ActionDrop, result.Action)

	// Service refs should be empty but struct fields should exist
	assert.Equal(t, "", result.ServiceFromRef.Name)
	assert.Equal(t, "", result.ServiceToRef.Name)
}

func TestConvertSvcSvcRuleToPB_FullConversion(t *testing.T) {
	// Arrange
	domainRule := models.SvcSvcRule{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      "rule-001",
				Namespace: "test-ns",
			},
		},
		ServiceFromRef: v1beta1.NamespacedObjectReference{
			ObjectReference: v1beta1.ObjectReference{
				APIVersion: NetguardAPIVersion,
				Kind:       KindService,
				Name:       "service-a",
			},
			Namespace: "test-ns",
		},
		ServiceToRef: v1beta1.NamespacedObjectReference{
			ObjectReference: v1beta1.ObjectReference{
				APIVersion: NetguardAPIVersion,
				Kind:       KindService,
				Name:       "service-b",
			},
			Namespace: "test-ns",
		},
		Action:      models.ActionAccept,
		Description: "Domain rule description",
		Priority:    100,
		Logs:        true,
		Trace:       false,
		Meta: models.Meta{
			UID:             "test-uid",
			ResourceVersion: "12345",
			Generation:      1,
			CreationTS:      metav1.Now(),
		},
	}

	// Act
	result := ConvertSvcSvcRuleToPB(domainRule)

	// Assert
	require.NotNil(t, result)

	// Check SelfRef
	assert.Equal(t, "rule-001", result.SelfRef.Name)
	assert.Equal(t, "test-ns", result.SelfRef.Namespace)

	// Check ServiceFrom
	assert.Equal(t, "service-a", result.ServiceFrom.Name)
	assert.Equal(t, "test-ns", result.ServiceFrom.Namespace)

	// Check ServiceTo
	assert.Equal(t, "service-b", result.ServiceTo.Name)
	assert.Equal(t, "test-ns", result.ServiceTo.Namespace)

	// Check other fields
	assert.Equal(t, netguardpb.RuleAction_ACCEPT, result.Action)
	assert.Equal(t, "Domain rule description", result.Description)
	assert.Equal(t, int32(100), result.Priority)
	assert.True(t, result.Logs)
	assert.False(t, result.Trace)

	// Check meta
	require.NotNil(t, result.Meta)
	assert.Equal(t, "test-uid", result.Meta.Uid)
	assert.Equal(t, "12345", result.Meta.ResourceVersion)
	assert.Equal(t, int64(1), result.Meta.Generation)
	assert.NotNil(t, result.Meta.CreationTs)
}

func TestConvertSvcSvcRule_BidirectionalConversion(t *testing.T) {
	// Arrange - original proto
	original := &netguardpb.SvcSvcRule{
		SelfRef: &netguardpb.ResourceIdentifier{
			Name:      "rule-001",
			Namespace: "test-ns",
		},
		ServiceFrom: &netguardpb.ResourceIdentifier{
			Name:      "service-a",
			Namespace: "test-ns",
		},
		ServiceTo: &netguardpb.ResourceIdentifier{
			Name:      "service-b",
			Namespace: "test-ns",
		},
		Action:      netguardpb.RuleAction_DROP,
		Description: "Bidirectional test description",
		Priority:    500,
		Logs:        false,
		Trace:       true,
		Meta: &netguardpb.Meta{
			Uid:             "test-uid",
			ResourceVersion: "99999",
		},
	}

	// Act - bidirectional conversion
	domain := ConvertSvcSvcRule(original)
	backToProto := ConvertSvcSvcRuleToPB(domain)

	// Assert - should match original
	assert.Equal(t, original.SelfRef.Name, backToProto.SelfRef.Name)
	assert.Equal(t, original.SelfRef.Namespace, backToProto.SelfRef.Namespace)
	assert.Equal(t, original.ServiceFrom.Name, backToProto.ServiceFrom.Name)
	assert.Equal(t, original.ServiceFrom.Namespace, backToProto.ServiceFrom.Namespace)
	assert.Equal(t, original.ServiceTo.Name, backToProto.ServiceTo.Name)
	assert.Equal(t, original.ServiceTo.Namespace, backToProto.ServiceTo.Namespace)
	assert.Equal(t, original.Action, backToProto.Action)
	assert.Equal(t, original.Description, backToProto.Description)
	assert.Equal(t, original.Priority, backToProto.Priority)
	assert.Equal(t, original.Logs, backToProto.Logs)
	assert.Equal(t, original.Trace, backToProto.Trace)
	assert.Equal(t, original.Meta.Uid, backToProto.Meta.Uid)
	assert.Equal(t, original.Meta.ResourceVersion, backToProto.Meta.ResourceVersion)
}

func TestConvertSvcSvcRule_ActionConversion(t *testing.T) {
	tests := []struct {
		name           string
		pbAction       netguardpb.RuleAction
		expectedAction models.RuleAction
	}{
		{
			name:           "ACCEPT action",
			pbAction:       netguardpb.RuleAction_ACCEPT,
			expectedAction: models.ActionAccept,
		},
		{
			name:           "DROP action",
			pbAction:       netguardpb.RuleAction_DROP,
			expectedAction: models.ActionDrop,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			pbRule := &netguardpb.SvcSvcRule{
				SelfRef: &netguardpb.ResourceIdentifier{
					Name:      "test-rule",
					Namespace: "test-ns",
				},
				Action: tt.pbAction,
			}

			// Act
			result := ConvertSvcSvcRule(pbRule)

			// Assert
			assert.Equal(t, tt.expectedAction, result.Action)
		})
	}
}

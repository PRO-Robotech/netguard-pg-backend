package convert

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"netguard-pg-backend/internal/domain/models"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
)

func TestServiceConverter_ToDomain(t *testing.T) {
	ctx := context.Background()
	converter := NewServiceConverter()

	testCases := []struct {
		name        string
		input       *netguardv1beta1.Service
		expected    *models.Service
		expectError bool
	}{
		{
			name: "valid service with minimal fields",
			input: &netguardv1beta1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "default",
				},
				Spec: netguardv1beta1.ServiceSpec{
					Description: "Test service",
				},
			},
			expected: &models.Service{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-service",
						Namespace: "default",
					},
				},
				Description: "Test service",
				Meta: models.Meta{
					UID:                "",
					ResourceVersion:    "",
					Generation:         0,
					CreationTS:         metav1.Time{},
					ObservedGeneration: 0,
					Conditions:         nil,
				},
			},
		},
		{
			name:        "nil input",
			input:       nil,
			expected:    nil,
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := converter.ToDomain(ctx, tc.input)

			if tc.expectError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expected, result)
			}
		})
	}
}

func TestServiceConverter_FromDomain(t *testing.T) {
	ctx := context.Background()
	converter := NewServiceConverter()

	testCases := []struct {
		name        string
		input       *models.Service
		expected    *netguardv1beta1.Service
		expectError bool
	}{
		{
			name: "valid service with minimal fields",
			input: &models.Service{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.ResourceIdentifier{
						Name:      "test-service",
						Namespace: "default",
					},
				},
				Description: "Test service",
			},
			expected: &netguardv1beta1.Service{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "Service",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "default",
				},
				Spec: netguardv1beta1.ServiceSpec{
					Description: "Test service",
				},
				Status: netguardv1beta1.ServiceStatus{
					ObservedGeneration: 0,
					Conditions:         nil,
				},
			},
		},
		{
			name:        "nil input",
			input:       nil,
			expected:    nil,
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := converter.FromDomain(ctx, tc.input)

			if tc.expectError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expected, result)
			}
		})
	}
}

func TestServiceConverter_CommentField(t *testing.T) {
	ctx := context.Background()
	converter := NewServiceConverter()

	t.Run("ToDomain carries comment", func(t *testing.T) {
		k8sService := &netguardv1beta1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-service",
				Namespace: "default",
			},
			Spec: netguardv1beta1.ServiceSpec{
				Comment: "K8s service comment",
			},
		}

		result, err := converter.ToDomain(ctx, k8sService)
		require.NoError(t, err)
		assert.Equal(t, "K8s service comment", result.Comment)
	})

	t.Run("ToDomain with empty comment", func(t *testing.T) {
		k8sService := &netguardv1beta1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-service",
				Namespace: "default",
			},
			Spec: netguardv1beta1.ServiceSpec{},
		}

		result, err := converter.ToDomain(ctx, k8sService)
		require.NoError(t, err)
		assert.Equal(t, "", result.Comment)
	})

	t.Run("FromDomain carries comment", func(t *testing.T) {
		domainService := &models.Service{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.ResourceIdentifier{
					Name:      "test-service",
					Namespace: "default",
				},
			},
			Comment: "Domain service comment",
		}

		result, err := converter.FromDomain(ctx, domainService)
		require.NoError(t, err)
		assert.Equal(t, "Domain service comment", result.Spec.Comment)
	})

	t.Run("bidirectional comment conversion", func(t *testing.T) {
		originalComment := "Bidirectional comment"
		k8sService := &netguardv1beta1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-service",
				Namespace: "default",
			},
			Spec: netguardv1beta1.ServiceSpec{
				Comment: originalComment,
			},
		}

		domain, err := converter.ToDomain(ctx, k8sService)
		require.NoError(t, err)

		backToK8s, err := converter.FromDomain(ctx, domain)
		require.NoError(t, err)

		assert.Equal(t, originalComment, backToK8s.Spec.Comment)
	})
}

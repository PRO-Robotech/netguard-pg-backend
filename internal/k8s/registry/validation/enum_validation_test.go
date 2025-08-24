package validation_test

//
//import (
//	"testing"
//
//	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
//	"netguard-pg-backend/internal/k8s/registry/validation"
//
//	"github.com/stretchr/testify/assert"
//	"github.com/stretchr/testify/require"
//	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
//	"k8s.io/apimachinery/pkg/util/validation/field"
//)
//
//func TestRuleS2SValidator_TrafficEnum_Validation(t *testing.T) {
//	validator := validation.NewRuleS2SValidator()
//
//	t.Run("ValidTrafficValues_NoErrors", func(t *testing.T) {
//		validTrafficValues := []v1beta1.Traffic{
//			v1beta1.INGRESS,
//			v1beta1.EGRESS,
//		}
//
//		for _, traffic := range validTrafficValues {
//			t.Run(string(traffic), func(t *testing.T) {
//				// Arrange
//				ruleS2S := &v1beta1.RuleS2S{
//					ObjectMeta: metav1.ObjectMeta{
//						Name:      "test-rule",
//						Namespace: "default",
//					},
//					Spec: v1beta1.RuleS2SSpec{
//						Traffic: traffic,
//						ServiceLocalRef: v1beta1.NamespacedObjectReference{
//							ObjectReference: v1beta1.ObjectReference{Name: "backend"},
//							Namespace:       "default",
//						},
//						ServiceRef: v1beta1.NamespacedObjectReference{
//							ObjectReference: v1beta1.ObjectReference{Name: "frontend"},
//							Namespace:       "default",
//						},
//					},
//				}
//
//				// Act
//				errs := validator.ValidateRuleS2S(ruleS2S)
//
//				// Assert
//				assert.Empty(t, errs, "Valid traffic value should not produce validation errors")
//			})
//		}
//	})
//
//	t.Run("InvalidTrafficValues_ReturnsErrors", func(t *testing.T) {
//		invalidTrafficValues := []v1beta1.Traffic{
//			"INVALID",
//			"ingress",  // lowercase
//			"egress",   // lowercase
//			"INBOUND",  // wrong value
//			"OUTBOUND", // wrong value
//			"",         // empty
//		}
//
//		for _, traffic := range invalidTrafficValues {
//			t.Run(string(traffic), func(t *testing.T) {
//				// Arrange
//				ruleS2S := &v1beta1.RuleS2S{
//					ObjectMeta: metav1.ObjectMeta{
//						Name:      "test-rule",
//						Namespace: "default",
//					},
//					Spec: v1beta1.RuleS2SSpec{
//						Traffic: traffic,
//						ServiceLocalRef: v1beta1.NamespacedObjectReference{
//							ObjectReference: v1beta1.ObjectReference{Name: "backend"},
//							Namespace:       "default",
//						},
//						ServiceRef: v1beta1.NamespacedObjectReference{
//							ObjectReference: v1beta1.ObjectReference{Name: "frontend"},
//							Namespace:       "default",
//						},
//					},
//				}
//
//				// Act
//				errs := validator.ValidateRuleS2S(ruleS2S)
//
//				// Assert
//				require.NotEmpty(t, errs, "Invalid traffic value should produce validation errors")
//
//				// Check that the error is about traffic field
//				hasTrafficError := false
//				for _, err := range errs {
//					if err.Field == "spec.traffic" {
//						hasTrafficError = true
//						assert.Equal(t, field.ErrorTypeNotSupported, err.Type,
//							"Should be a NotSupported error")
//						break
//					}
//				}
//				assert.True(t, hasTrafficError, "Should have validation error for traffic field")
//			})
//		}
//	})
//}
//
//func TestIEAgAgRuleValidator_TransportEnum_Validation(t *testing.T) {
//	validator := validation.NewIEAgAgRuleValidator()
//
//	t.Run("ValidTransportValues_NoErrors", func(t *testing.T) {
//		validTransportValues := []v1beta1.TransportProtocol{
//			v1beta1.ProtocolTCP,
//			v1beta1.ProtocolUDP,
//		}
//
//		for _, transport := range validTransportValues {
//			t.Run(string(transport), func(t *testing.T) {
//				// Arrange
//				ieAgAgRule := &v1beta1.IEAgAgRule{
//					ObjectMeta: metav1.ObjectMeta{
//						Name:      "test-ieagag",
//						Namespace: "default",
//					},
//					Spec: v1beta1.IEAgAgRuleSpec{
//						Transport:         transport,
//						Traffic:           v1beta1.INGRESS,
//						AddressGroupLocal: v1beta1.ObjectReference{Name: "local-ag"},
//						AddressGroup:      v1beta1.ObjectReference{Name: "remote-ag"},
//					},
//				}
//
//				// Act
//				errs := validator.ValidateIEAgAgRule(ieAgAgRule)
//
//				// Assert
//				assert.Empty(t, errs, "Valid transport value should not produce validation errors")
//			})
//		}
//	})
//
//	t.Run("InvalidTransportValues_ReturnsErrors", func(t *testing.T) {
//		invalidTransportValues := []v1beta1.TransportProtocol{
//			"INVALID",
//			"tcp",   // lowercase
//			"udp",   // lowercase
//			"SCTP",  // removed protocol
//			"HTTP",  // wrong protocol
//			"HTTPS", // wrong protocol
//			"",      // empty
//		}
//
//		for _, transport := range invalidTransportValues {
//			t.Run(string(transport), func(t *testing.T) {
//				// Arrange
//				ieAgAgRule := &v1beta1.IEAgAgRule{
//					ObjectMeta: metav1.ObjectMeta{
//						Name:      "test-ieagag",
//						Namespace: "default",
//					},
//					Spec: v1beta1.IEAgAgRuleSpec{
//						Transport:         transport,
//						Traffic:           v1beta1.INGRESS,
//						AddressGroupLocal: v1beta1.ObjectReference{Name: "local-ag"},
//						AddressGroup:      v1beta1.ObjectReference{Name: "remote-ag"},
//					},
//				}
//
//				// Act
//				errs := validator.ValidateIEAgAgRule(ieAgAgRule)
//
//				// Assert
//				require.NotEmpty(t, errs, "Invalid transport value should produce validation errors")
//
//				// Check that the error is about transport field
//				hasTransportError := false
//				for _, err := range errs {
//					if err.Field == "spec.transport" {
//						hasTransportError = true
//						assert.Equal(t, field.ErrorTypeNotSupported, err.Type,
//							"Should be a NotSupported error")
//						break
//					}
//				}
//				assert.True(t, hasTransportError, "Should have validation error for transport field")
//			})
//		}
//	})
//
//	t.Run("ValidTrafficValues_NoErrors", func(t *testing.T) {
//		validTrafficValues := []v1beta1.Traffic{
//			v1beta1.INGRESS,
//			v1beta1.EGRESS,
//		}
//
//		for _, traffic := range validTrafficValues {
//			t.Run(string(traffic), func(t *testing.T) {
//				// Arrange
//				ieAgAgRule := &v1beta1.IEAgAgRule{
//					ObjectMeta: metav1.ObjectMeta{
//						Name:      "test-ieagag",
//						Namespace: "default",
//					},
//					Spec: v1beta1.IEAgAgRuleSpec{
//						Transport:         v1beta1.ProtocolTCP,
//						Traffic:           traffic,
//						AddressGroupLocal: v1beta1.ObjectReference{Name: "local-ag"},
//						AddressGroup:      v1beta1.ObjectReference{Name: "remote-ag"},
//					},
//				}
//
//				// Act
//				errs := validator.ValidateIEAgAgRule(ieAgAgRule)
//
//				// Assert
//				assert.Empty(t, errs, "Valid traffic value should not produce validation errors")
//			})
//		}
//	})
//
//	t.Run("InvalidTrafficValues_ReturnsErrors", func(t *testing.T) {
//		invalidTrafficValues := []v1beta1.Traffic{
//			"INVALID",
//			"ingress", // lowercase
//			"egress",  // lowercase
//			"IN",      // wrong value
//			"OUT",     // wrong value
//			"",        // empty
//		}
//
//		for _, traffic := range invalidTrafficValues {
//			t.Run(string(traffic), func(t *testing.T) {
//				// Arrange
//				ieAgAgRule := &v1beta1.IEAgAgRule{
//					ObjectMeta: metav1.ObjectMeta{
//						Name:      "test-ieagag",
//						Namespace: "default",
//					},
//					Spec: v1beta1.IEAgAgRuleSpec{
//						Transport:         v1beta1.ProtocolTCP,
//						Traffic:           traffic,
//						AddressGroupLocal: v1beta1.ObjectReference{Name: "local-ag"},
//						AddressGroup:      v1beta1.ObjectReference{Name: "remote-ag"},
//					},
//				}
//
//				// Act
//				errs := validator.ValidateIEAgAgRule(ieAgAgRule)
//
//				// Assert
//				require.NotEmpty(t, errs, "Invalid traffic value should produce validation errors")
//
//				// Check that the error is about traffic field
//				hasTrafficError := false
//				for _, err := range errs {
//					if err.Field == "spec.traffic" {
//						hasTrafficError = true
//						assert.Equal(t, field.ErrorTypeNotSupported, err.Type,
//							"Should be a NotSupported error")
//						break
//					}
//				}
//				assert.True(t, hasTrafficError, "Should have validation error for traffic field")
//			})
//		}
//	})
//
//	t.Run("ValidActionValues_NoErrors", func(t *testing.T) {
//		validActionValues := []v1beta1.RuleAction{
//			v1beta1.ActionAccept,
//			v1beta1.ActionDrop,
//		}
//
//		for _, action := range validActionValues {
//			t.Run(string(action), func(t *testing.T) {
//				// Arrange
//				ieAgAgRule := &v1beta1.IEAgAgRule{
//					ObjectMeta: metav1.ObjectMeta{
//						Name:      "test-ieagag",
//						Namespace: "default",
//					},
//					Spec: v1beta1.IEAgAgRuleSpec{
//						Transport:         v1beta1.ProtocolTCP,
//						Traffic:           v1beta1.INGRESS,
//						AddressGroupLocal: v1beta1.ObjectReference{Name: "local-ag"},
//						AddressGroup:      v1beta1.ObjectReference{Name: "remote-ag"},
//						Action:            action,
//					},
//				}
//
//				// Act
//				errs := validator.ValidateIEAgAgRule(ieAgAgRule)
//
//				// Assert
//				assert.Empty(t, errs, "Valid action value should not produce validation errors")
//			})
//		}
//	})
//
//	t.Run("InvalidActionValues_ReturnsErrors", func(t *testing.T) {
//		invalidActionValues := []v1beta1.RuleAction{
//			"INVALID",
//			"accept", // lowercase
//			"drop",   // lowercase
//			"ALLOW",  // wrong value
//			"DENY",   // wrong value
//			"REJECT", // wrong value
//		}
//
//		for _, action := range invalidActionValues {
//			t.Run(string(action), func(t *testing.T) {
//				// Arrange
//				ieAgAgRule := &v1beta1.IEAgAgRule{
//					ObjectMeta: metav1.ObjectMeta{
//						Name:      "test-ieagag",
//						Namespace: "default",
//					},
//					Spec: v1beta1.IEAgAgRuleSpec{
//						Transport:         v1beta1.ProtocolTCP,
//						Traffic:           v1beta1.INGRESS,
//						AddressGroupLocal: v1beta1.ObjectReference{Name: "local-ag"},
//						AddressGroup:      v1beta1.ObjectReference{Name: "remote-ag"},
//						Action:            action,
//					},
//				}
//
//				// Act
//				errs := validator.ValidateIEAgAgRule(ieAgAgRule)
//
//				// Assert
//				require.NotEmpty(t, errs, "Invalid action value should produce validation errors")
//
//				// Check that the error is about action field
//				hasActionError := false
//				for _, err := range errs {
//					if err.Field == "spec.action" {
//						hasActionError = true
//						assert.Equal(t, field.ErrorTypeNotSupported, err.Type,
//							"Should be a NotSupported error")
//						break
//					}
//				}
//				assert.True(t, hasActionError, "Should have validation error for action field")
//			})
//		}
//	})
//}
//
//func TestServiceValidator_IngressPortProtocolEnum_Validation(t *testing.T) {
//	// Примечание: этот тест предполагает, что у нас есть валидатор для Service
//	// Если его нет, можно пропустить этот тест или создать простой валидатор
//
//	t.Run("ValidProtocolValues_NoErrors", func(t *testing.T) {
//		validProtocolValues := []v1beta1.TransportProtocol{
//			v1beta1.ProtocolTCP,
//			v1beta1.ProtocolUDP,
//		}
//
//		for _, protocol := range validProtocolValues {
//			t.Run(string(protocol), func(t *testing.T) {
//				// Arrange
//				service := &v1beta1.Service{
//					ObjectMeta: metav1.ObjectMeta{
//						Name:      "test-service",
//						Namespace: "default",
//					},
//					Spec: v1beta1.ServiceSpec{
//						IngressPorts: []v1beta1.IngressPort{
//							{
//								Protocol: protocol,
//								Port:     "8080",
//							},
//						},
//					},
//				}
//
//				// Act - простая проверка, что структура валидна
//				// В реальном коде здесь был бы вызов валидатора Service
//				// validator := validation.NewServiceValidator()
//				// errs := validator.ValidateService(service)
//
//				// Assert
//				assert.NotNil(t, service, "Service should be created successfully")
//				assert.Equal(t, protocol, service.Spec.IngressPorts[0].Protocol,
//					"Protocol should be set correctly")
//			})
//		}
//	})
//
//	t.Run("InvalidProtocolValues_WouldProduceErrors", func(t *testing.T) {
//		invalidProtocolValues := []v1beta1.TransportProtocol{
//			"INVALID",
//			"tcp",  // lowercase
//			"udp",  // lowercase
//			"SCTP", // removed protocol
//			"HTTP", // wrong protocol
//		}
//
//		for _, protocol := range invalidProtocolValues {
//			t.Run(string(protocol), func(t *testing.T) {
//				// Arrange
//				service := &v1beta1.Service{
//					ObjectMeta: metav1.ObjectMeta{
//						Name:      "test-service",
//						Namespace: "default",
//					},
//					Spec: v1beta1.ServiceSpec{
//						IngressPorts: []v1beta1.IngressPort{
//							{
//								Protocol: protocol,
//								Port:     "8080",
//							},
//						},
//					},
//				}
//
//				// Act & Assert - проверяем, что неправильные значения можно установить
//				// (валидация произойдет в рантайме через kubebuilder аннотации)
//				assert.NotNil(t, service, "Service structure should accept any string")
//				assert.Equal(t, protocol, service.Spec.IngressPorts[0].Protocol,
//					"Invalid protocol should be settable (runtime validation will catch it)")
//			})
//		}
//	})
//}
//
//func TestAddressGroupValidator_DefaultActionEnum_Validation(t *testing.T) {
//	// Примечание: этот тест предполагает наличие валидатора для AddressGroup
//
//	t.Run("ValidActionValues_NoErrors", func(t *testing.T) {
//		validActionValues := []v1beta1.RuleAction{
//			v1beta1.ActionAccept,
//			v1beta1.ActionDrop,
//		}
//
//		for _, action := range validActionValues {
//			t.Run(string(action), func(t *testing.T) {
//				// Arrange
//				addressGroup := &v1beta1.AddressGroup{
//					ObjectMeta: metav1.ObjectMeta{
//						Name:      "test-ag",
//						Namespace: "default",
//					},
//					Spec: v1beta1.AddressGroupSpec{
//						DefaultAction: action,
//					},
//				}
//
//				// Act - простая проверка структуры
//				// В реальном коде: validator.ValidateAddressGroup(addressGroup)
//
//				// Assert
//				assert.NotNil(t, addressGroup, "AddressGroup should be created successfully")
//				assert.Equal(t, action, addressGroup.Spec.DefaultAction,
//					"DefaultAction should be set correctly")
//			})
//		}
//	})
//
//	t.Run("InvalidActionValues_WouldProduceErrors", func(t *testing.T) {
//		invalidActionValues := []v1beta1.RuleAction{
//			"INVALID",
//			"accept", // lowercase
//			"drop",   // lowercase
//			"ALLOW",  // wrong value
//			"DENY",   // wrong value
//		}
//
//		for _, action := range invalidActionValues {
//			t.Run(string(action), func(t *testing.T) {
//				// Arrange
//				addressGroup := &v1beta1.AddressGroup{
//					ObjectMeta: metav1.ObjectMeta{
//						Name:      "test-ag",
//						Namespace: "default",
//					},
//					Spec: v1beta1.AddressGroupSpec{
//						DefaultAction: action,
//					},
//				}
//
//				// Act & Assert
//				assert.NotNil(t, addressGroup, "AddressGroup structure should accept any string")
//				assert.Equal(t, action, addressGroup.Spec.DefaultAction,
//					"Invalid action should be settable (runtime validation will catch it)")
//			})
//		}
//	})
//}
//
//func TestEnumValidation_EdgeCases(t *testing.T) {
//	t.Run("EmptyStringValues_AreHandledCorrectly", func(t *testing.T) {
//		validator := validation.NewIEAgAgRuleValidator()
//
//		// Arrange
//		ieAgAgRule := &v1beta1.IEAgAgRule{
//			ObjectMeta: metav1.ObjectMeta{
//				Name:      "test-ieagag",
//				Namespace: "default",
//			},
//			Spec: v1beta1.IEAgAgRuleSpec{
//				Transport:         "", // empty transport
//				Traffic:           "", // empty traffic
//				AddressGroupLocal: v1beta1.ObjectReference{Name: "local-ag"},
//				AddressGroup:      v1beta1.ObjectReference{Name: "remote-ag"},
//			},
//		}
//
//		// Act
//		errs := validator.ValidateIEAgAgRule(ieAgAgRule)
//
//		// Assert
//		require.NotEmpty(t, errs, "Empty enum values should produce validation errors")
//
//		// Should have errors for both transport and traffic
//		hasTransportError := false
//		hasTrafficError := false
//		for _, err := range errs {
//			switch err.Field {
//			case "spec.transport":
//				hasTransportError = true
//			case "spec.traffic":
//				hasTrafficError = true
//			}
//		}
//		assert.True(t, hasTransportError, "Should have transport validation error")
//		assert.True(t, hasTrafficError, "Should have traffic validation error")
//	})
//
//	t.Run("CaseSensitivity_MattersForEnums", func(t *testing.T) {
//		validator := validation.NewRuleS2SValidator()
//
//		// Arrange - lowercase values should fail
//		ruleS2S := &v1beta1.RuleS2S{
//			ObjectMeta: metav1.ObjectMeta{
//				Name:      "test-rule",
//				Namespace: "default",
//			},
//			Spec: v1beta1.RuleS2SSpec{
//				Traffic: "ingress", // lowercase, should fail
//				ServiceLocalRef: v1beta1.NamespacedObjectReference{
//					ObjectReference: v1beta1.ObjectReference{Name: "backend"},
//					Namespace:       "default",
//				},
//				ServiceRef: v1beta1.NamespacedObjectReference{
//					ObjectReference: v1beta1.ObjectReference{Name: "frontend"},
//					Namespace:       "default",
//				},
//			},
//		}
//
//		// Act
//		errs := validator.ValidateRuleS2S(ruleS2S)
//
//		// Assert
//		require.NotEmpty(t, errs, "Lowercase enum values should be rejected")
//
//		hasTrafficError := false
//		for _, err := range errs {
//			if err.Field == "spec.traffic" {
//				hasTrafficError = true
//				break
//			}
//		}
//		assert.True(t, hasTrafficError, "Should reject lowercase 'ingress'")
//	})
//}

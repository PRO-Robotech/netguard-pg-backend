package validation

import (
	"context"
	"log"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"

	"github.com/pkg/errors"
)

// IEAgAgRuleValidator валидирует правила IEAgAgRule
type IEAgAgRuleValidator struct {
	BaseValidator
}

// NewIEAgAgRuleValidator создает новый валидатор IEAgAgRule
func NewIEAgAgRuleValidator(reader ports.Reader) *IEAgAgRuleValidator {
	listFunction := func(ctx context.Context, consume func(entity interface{}) error, scope ports.Scope) error {
		return reader.ListIEAgAgRules(ctx, func(rule models.IEAgAgRule) error {
			return consume(&rule) // Передаем указатель вместо значения
		}, scope)
	}
	return &IEAgAgRuleValidator{
		BaseValidator: *NewBaseValidator(reader, "IEAgAgRule", listFunction),
	}
}

// ValidateExists проверяет существование правила
func (v *IEAgAgRuleValidator) ValidateExists(ctx context.Context, id models.ResourceIdentifier) error {
	return v.BaseValidator.ValidateExists(ctx, id, func(entity interface{}) string {
		return entity.(*models.IEAgAgRule).Key() // Используем указатель вместо значения
	})
}

// ValidateReferences проверяет ссылки в правиле
func (v *IEAgAgRuleValidator) ValidateReferences(ctx context.Context, rule models.IEAgAgRule) error {
	addressGroupValidator := NewAddressGroupValidator(v.reader)

	// Create ResourceIdentifier from NamespacedObjectReference for local address group
	localAGID := models.NewResourceIdentifier(rule.AddressGroupLocal.Name, models.WithNamespace(rule.AddressGroupLocal.Namespace))
	if err := addressGroupValidator.ValidateExists(ctx, localAGID); err != nil {
		return errors.Wrapf(err, "invalid local address group reference in rule %s", rule.Key())
	}

	// Create ResourceIdentifier from NamespacedObjectReference for address group
	agID := models.NewResourceIdentifier(rule.AddressGroup.Name, models.WithNamespace(rule.AddressGroup.Namespace))
	if err := addressGroupValidator.ValidateExists(ctx, agID); err != nil {
		return errors.Wrapf(err, "invalid address group reference in rule %s", rule.Key())
	}

	return nil
}

// ValidatePortSpec проверяет спецификацию портов
func (v *IEAgAgRuleValidator) ValidatePortSpec(ctx context.Context, portSpec models.PortSpec) error {
	if err := validatePort(portSpec.Destination); err != nil {
		return errors.Wrapf(err, "invalid destination port")
	}

	if portSpec.Source != "" {
		if err := validatePort(portSpec.Source); err != nil {
			return errors.Wrapf(err, "invalid source port")
		}
	}

	return nil
}

// ValidateForCreation валидирует правило перед созданием
func (v *IEAgAgRuleValidator) ValidateForCreation(ctx context.Context, rule models.IEAgAgRule) error {
	// PHASE 1: Check for duplicate entity (CRITICAL FIX for overwrite issue)
	// This prevents creation of entities with the same namespace/name combination
	keyExtractor := func(entity interface{}) string {
		if ieagag, ok := entity.(*models.IEAgAgRule); ok {
			return ieagag.Key()
		}
		return ""
	}

	if err := v.BaseValidator.ValidateEntityDoesNotExistForCreation(ctx, rule.ResourceIdentifier, keyExtractor); err != nil {
		return err // Return the detailed EntityAlreadyExistsError with logging and context
	}

	// PHASE 2: Validate references (existing validation)
	if err := v.ValidateReferences(ctx, rule); err != nil {
		return err
	}

	// PHASE 3: Validate ports (existing validation)
	for _, port := range rule.Ports {
		if err := v.ValidatePortSpec(ctx, port); err != nil {
			return errors.Wrapf(err, "invalid port specification in rule %s", rule.Key())
		}
	}

	return nil
}

// ValidateForPostCommit validates an IEAgAgRule after it has been committed to database
// This skips duplicate checking since the entity already exists in the database
func (v *IEAgAgRuleValidator) ValidateForPostCommit(ctx context.Context, rule models.IEAgAgRule) error {
	// PHASE 1: Skip duplicate entity check (entity is already committed)
	// This method is called AFTER the entity is saved to database, so existence is expected

	// PHASE 2: Validate references (existing validation)
	if err := v.ValidateReferences(ctx, rule); err != nil {
		return err
	}

	// PHASE 3: Validate ports (existing validation)
	for _, port := range rule.Ports {
		if err := v.ValidatePortSpec(ctx, port); err != nil {
			return errors.Wrapf(err, "invalid port specification in rule %s", rule.Key())
		}
	}

	return nil
}

// isRuleReady проверяет, находится ли правило в состоянии Ready
func (v *IEAgAgRuleValidator) isRuleReady(rule models.IEAgAgRule) bool {
	for _, condition := range rule.Meta.Conditions {
		if condition.Type == "Ready" && condition.Status == "True" {
			return true
		}
	}
	return false
}

// ValidateForUpdate валидирует правило перед обновлением
func (v *IEAgAgRuleValidator) ValidateForUpdate(ctx context.Context, oldRule, newRule models.IEAgAgRule) error {
	// Проверяем immutable поля только если правило в состоянии Ready (проверяем ПЕРВЫМИ)
	if v.isRuleReady(oldRule) {
		if oldRule.Transport != newRule.Transport {
			return errors.New("Transport field is immutable when rule is in Ready state")
		}

		if oldRule.Traffic != newRule.Traffic {
			return errors.New("Traffic field is immutable when rule is in Ready state")
		}

		if oldRule.AddressGroupLocalKey() != newRule.AddressGroupLocalKey() {
			return errors.New("AddressGroupLocal field is immutable when rule is in Ready state")
		}

		if oldRule.AddressGroupKey() != newRule.AddressGroupKey() {
			return errors.New("AddressGroup field is immutable when rule is in Ready state")
		}

		if oldRule.Action != newRule.Action {
			return errors.New("Action field is immutable when rule is in Ready state")
		}
	}

	// Валидация ссылок
	if err := v.ValidateReferences(ctx, newRule); err != nil {
		return err
	}

	// Валидация портов
	for _, port := range newRule.Ports {
		if err := v.ValidatePortSpec(ctx, port); err != nil {
			return errors.Wrapf(err, "invalid port specification in rule %s", newRule.Key())
		}
	}

	return nil
}

// CheckDependencies checks if there are dependencies before deleting an IEAgAg rule
func (v *IEAgAgRuleValidator) CheckDependencies(ctx context.Context, id models.ResourceIdentifier) error {
	// IEAgAgRule is a generated rule derived from RuleS2S and AddressGroup relationships
	// It can be safely deleted as nothing should depend on it directly

	// Log the dependency check for consistency with other validators
	log.Printf("CheckDependencies: IEAgAgRule %s can be safely deleted (no dependents)", id.Key())
	return nil
}

package validation

import (
	"context"
	"fmt"

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
			return consume(rule)
		}, scope)
	}
	return &IEAgAgRuleValidator{
		BaseValidator: *NewBaseValidator(reader, "IEAgAgRule", listFunction),
	}
}

// ValidateExists проверяет существование правила
func (v *IEAgAgRuleValidator) ValidateExists(ctx context.Context, id models.ResourceIdentifier) error {
	return v.BaseValidator.ValidateExists(ctx, id, func(entity interface{}) string {
		return entity.(models.IEAgAgRule).Key()
	})
}

// ValidateReferences проверяет ссылки в правиле
func (v *IEAgAgRuleValidator) ValidateReferences(ctx context.Context, rule models.IEAgAgRule) error {
	addressGroupValidator := NewAddressGroupValidator(v.reader)

	if err := addressGroupValidator.ValidateExists(ctx, rule.AddressGroupLocal.ResourceIdentifier); err != nil {
		return errors.Wrapf(err, "invalid local address group reference in rule %s", rule.Key())
	}

	if err := addressGroupValidator.ValidateExists(ctx, rule.AddressGroup.ResourceIdentifier); err != nil {
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
	// Валидация ссылок
	if err := v.ValidateReferences(ctx, rule); err != nil {
		return err
	}

	// Валидация портов
	for _, port := range rule.Ports {
		if err := v.ValidatePortSpec(ctx, port); err != nil {
			return errors.Wrapf(err, "invalid port specification in rule %s", rule.Key())
		}
	}

	return nil
}

// ValidateForUpdate валидирует правило перед обновлением
func (v *IEAgAgRuleValidator) ValidateForUpdate(ctx context.Context, oldRule, newRule models.IEAgAgRule) error {
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

	// Проверка, что транспортный протокол не изменился
	if oldRule.Transport != newRule.Transport {
		return fmt.Errorf("cannot change transport protocol after creation")
	}

	// Проверка, что направление трафика не изменилось
	if oldRule.Traffic != newRule.Traffic {
		return fmt.Errorf("cannot change traffic direction after creation")
	}

	// Проверка, что ссылка на локальную группу адресов не изменилась
	if oldRule.AddressGroupLocal.Key() != newRule.AddressGroupLocal.Key() {
		return fmt.Errorf("cannot change local address group reference after creation")
	}

	// Проверка, что ссылка на группу адресов не изменилась
	if oldRule.AddressGroup.Key() != newRule.AddressGroup.Key() {
		return fmt.Errorf("cannot change target address group reference after creation")
	}

	return nil
}

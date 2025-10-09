package mem

import (
	"context"
	"fmt"
	"time"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
)

type writer struct {
	registry                    *Registry
	ctx                         context.Context
	services                    map[string]models.Service
	addressGroups               map[string]models.AddressGroup
	addressGroupBindings        map[string]models.AddressGroupBinding
	addressGroupPortMappings    map[string]models.AddressGroupPortMapping
	addressGroupBindingPolicies map[string]models.AddressGroupBindingPolicy
	ruleS2S                     map[string]models.RuleS2S
	serviceAliases              map[string]models.ServiceAlias
	ieAgAgRules                 map[string]models.IEAgAgRule
	networks                    map[string]models.Network
	networkBindings             map[string]models.NetworkBinding
	hosts                       map[string]models.Host
	hostBindings                map[string]models.HostBinding
}

func (w *writer) SyncServices(ctx context.Context, services []models.Service, scope ports.Scope, opts ...ports.Option) error {
	// Определение операции (по умолчанию FullSync)
	syncOp := models.SyncOpFullSync

	// Извлечение опций
	for _, opt := range opts {
		if so, ok := opt.(ports.SyncOption); ok {
			syncOp = so.Operation
		}
	}

	// Инициализация карты, если она еще не создана
	if w.services == nil {
		w.services = make(map[string]models.Service)
		// Всегда копируем существующие сервисы, чтобы иметь полную карту для работы
		for k, v := range w.registry.db.GetServices() {
			w.services[k] = v
		}
	}

	// Обработка в зависимости от типа операции
	switch syncOp {
	case models.SyncOpFullSync:
		// Если scope не пустой, удаляем только сервисы в указанной области
		if scope != nil && !scope.IsEmpty() {
			// Проверяем, что scope имеет тип ResourceIdentifierScope
			if ris, ok := scope.(ports.ResourceIdentifierScope); ok && !ris.IsEmpty() {
				// Создаем временную карту для хранения сервисов вне области видимости
				tempServices := make(map[string]models.Service)

				// Создаем карту идентификаторов в области видимости для быстрого поиска
				scopeIds := make(map[string]bool)
				for _, id := range ris.Identifiers {
					scopeIds[id.Key()] = true
				}

				// Сохраняем сервисы вне области видимости
				for k, v := range w.services {
					// Проверяем, входит ли сервис в область видимости
					serviceKey := v.Key()
					if !scopeIds[serviceKey] {
						// Сохраняем сервисы, которые не входят в область видимости
						tempServices[k] = v
					}
				}

				// Очищаем карту и восстанавливаем сервисы вне области видимости
				w.services = make(map[string]models.Service)
				for k, v := range tempServices {
					w.services[k] = v
				}
			} else {
				// Если scope не ResourceIdentifierScope, но не пустой,
				// то мы не знаем, как его обрабатывать, поэтому не удаляем ничего
			}
		} else {
			// Если область пуста, очищаем всю карту
			w.services = make(map[string]models.Service)
		}

		// Добавляем новые сервисы
		for _, svc := range services {
			if existing, ok := w.services[svc.Key()]; ok {
				if svc.Meta.CreationTS.IsZero() {
					svc.Meta.CreationTS = existing.Meta.CreationTS
				}
				if svc.Meta.UID == "" {
					svc.Meta.UID = existing.Meta.UID
				}
			}
			ensureMetaFill(&svc.Meta)
			w.services[svc.Key()] = svc
		}

	case models.SyncOpUpsert:
		// Только добавление и обновление
		for _, svc := range services {
			if existing, ok := w.services[svc.Key()]; ok {
				if svc.Meta.CreationTS.IsZero() {
					svc.Meta.CreationTS = existing.Meta.CreationTS
				}
				if svc.Meta.UID == "" {
					svc.Meta.UID = existing.Meta.UID
				}
			}

			ensureMetaFill(&svc.Meta)

			w.services[svc.Key()] = svc
		}

	case models.SyncOpDelete:
		// Только удаление
		for _, service := range services {
			delete(w.services, service.Key())
		}
	}

	return nil
}

func (w *writer) SyncAddressGroups(ctx context.Context, addressGroups []models.AddressGroup, scope ports.Scope, opts ...ports.Option) error {
	// Определение операции (по умолчанию FullSync)
	syncOp := models.SyncOpFullSync

	// Извлечение опций
	for _, opt := range opts {
		if so, ok := opt.(ports.SyncOption); ok {
			syncOp = so.Operation
		}
	}

	// Инициализация карты, если она еще не создана
	if w.addressGroups == nil {
		w.addressGroups = make(map[string]models.AddressGroup)
		// Всегда копируем существующие группы адресов, чтобы иметь полную карту для работы
		for k, v := range w.registry.db.GetAddressGroups() {
			w.addressGroups[k] = v
		}
	}

	// Обработка в зависимости от типа операции
	switch syncOp {
	case models.SyncOpFullSync:
		// Если scope не пустой, удаляем только группы адресов в указанной области
		if scope != nil && !scope.IsEmpty() {
			// Проверяем, что scope имеет тип ResourceIdentifierScope
			if ris, ok := scope.(ports.ResourceIdentifierScope); ok && !ris.IsEmpty() {
				// Создаем временную карту для хранения групп адресов вне области видимости
				tempAddressGroups := make(map[string]models.AddressGroup)

				// Создаем карту идентификаторов в области видимости для быстрого поиска
				scopeIds := make(map[string]bool)
				for _, id := range ris.Identifiers {
					scopeIds[id.Key()] = true
				}

				// Сохраняем группы адресов вне области видимости
				for k, v := range w.addressGroups {
					if !scopeIds[k] {
						tempAddressGroups[k] = v
					}
				}

				// Очищаем карту и восстанавливаем группы адресов вне области видимости
				w.addressGroups = make(map[string]models.AddressGroup)
				for k, v := range tempAddressGroups {
					w.addressGroups[k] = v
				}
			} else {
				// Если scope не ResourceIdentifierScope, но не пустой,
				// то мы не знаем, как его обрабатывать, поэтому не удаляем ничего
			}
		} else {
			// Если область пуста, очищаем всю карту
			w.addressGroups = make(map[string]models.AddressGroup)
		}

		// Добавляем новые группы адресов
		for _, addressGroup := range addressGroups {
			key := addressGroup.Key()
			if existing, ok := w.addressGroups[key]; ok {
				if addressGroup.Meta.CreationTS.IsZero() {
					addressGroup.Meta.CreationTS = existing.Meta.CreationTS
				}
				if addressGroup.Meta.UID == "" {
					addressGroup.Meta.UID = existing.Meta.UID
				}
			}
			ensureMetaFill(&addressGroup.Meta)
			w.addressGroups[key] = addressGroup
		}

	case models.SyncOpUpsert:
		// Только добавление и обновление
		for _, addressGroup := range addressGroups {
			if existing, ok := w.addressGroups[addressGroup.Key()]; ok {
				if addressGroup.Meta.CreationTS.IsZero() {
					addressGroup.Meta.CreationTS = existing.Meta.CreationTS
				}
				if addressGroup.Meta.UID == "" {
					addressGroup.Meta.UID = existing.Meta.UID
				}
			}
			ensureMetaFill(&addressGroup.Meta)
			w.addressGroups[addressGroup.Key()] = addressGroup
		}

	case models.SyncOpDelete:
		// Только удаление
		for _, addressGroup := range addressGroups {
			delete(w.addressGroups, addressGroup.Key())
		}
	}

	return nil
}

func (w *writer) SyncAddressGroupBindings(ctx context.Context, bindings []models.AddressGroupBinding, scope ports.Scope, opts ...ports.Option) error {
	// Определение операции (по умолчанию FullSync)
	syncOp := models.SyncOpFullSync

	// Извлечение опций
	for _, opt := range opts {
		if so, ok := opt.(ports.SyncOption); ok {
			syncOp = so.Operation
		}
	}

	// Инициализация карты, если она еще не создана
	if w.addressGroupBindings == nil {
		w.addressGroupBindings = make(map[string]models.AddressGroupBinding)
		// Всегда копируем существующие привязки групп адресов, чтобы иметь полную карту для работы
		for k, v := range w.registry.db.GetAddressGroupBindings() {
			w.addressGroupBindings[k] = v
		}
	}

	// Обработка в зависимости от типа операции
	switch syncOp {
	case models.SyncOpFullSync:
		// Если scope не пустой, удаляем только привязки групп адресов в указанной области
		if scope != nil && !scope.IsEmpty() {
			// Проверяем, что scope имеет тип ResourceIdentifierScope
			if ris, ok := scope.(ports.ResourceIdentifierScope); ok && !ris.IsEmpty() {
				// Создаем временную карту для хранения привязок групп адресов вне области видимости
				tempBindings := make(map[string]models.AddressGroupBinding)

				// Создаем карту идентификаторов в области видимости для быстрого поиска
				scopeIds := make(map[string]bool)
				for _, id := range ris.Identifiers {
					scopeIds[id.Key()] = true
				}

				// Сохраняем привязки групп адресов вне области видимости
				for k, v := range w.addressGroupBindings {
					if !scopeIds[k] {
						tempBindings[k] = v
					}
				}

				// Очищаем карту и восстанавливаем привязки групп адресов вне области видимости
				w.addressGroupBindings = make(map[string]models.AddressGroupBinding)
				for k, v := range tempBindings {
					w.addressGroupBindings[k] = v
				}
			} else {
				// Если scope не ResourceIdentifierScope, но не пустой,
				// то мы не знаем, как его обрабатывать, поэтому не удаляем ничего
			}
		} else {
			// Если область пуста, очищаем всю карту
			w.addressGroupBindings = make(map[string]models.AddressGroupBinding)
		}

		// Добавляем новые привязки групп адресов
		for _, binding := range bindings {
			key := binding.Key()
			if existing, ok := w.addressGroupBindings[key]; ok {
				if binding.Meta.CreationTS.IsZero() {
					binding.Meta.CreationTS = existing.Meta.CreationTS
				}
				if binding.Meta.UID == "" {
					binding.Meta.UID = existing.Meta.UID
				}
			}
			ensureMetaFill(&binding.Meta)
			w.addressGroupBindings[key] = binding
		}

	case models.SyncOpUpsert:
		// Только добавление и обновление
		for _, binding := range bindings {
			key := binding.Key()
			if existing, ok := w.addressGroupBindings[key]; ok {
				if binding.Meta.CreationTS.IsZero() {
					binding.Meta.CreationTS = existing.Meta.CreationTS
				}
				if binding.Meta.UID == "" {
					binding.Meta.UID = existing.Meta.UID
				}
			}
			ensureMetaFill(&binding.Meta)
			w.addressGroupBindings[key] = binding
		}

	case models.SyncOpDelete:
		// Только удаление
		for _, binding := range bindings {
			key := binding.Key()
			delete(w.addressGroupBindings, key)
		}
	}

	return nil
}

func (w *writer) SyncAddressGroupPortMappings(ctx context.Context, mappings []models.AddressGroupPortMapping, scope ports.Scope, opts ...ports.Option) error {
	// Определение операции (по умолчанию FullSync)
	syncOp := models.SyncOpFullSync

	// Извлечение опций
	for _, opt := range opts {
		if so, ok := opt.(ports.SyncOption); ok {
			syncOp = so.Operation
		}
	}

	// Инициализация карты, если она еще не создана
	if w.addressGroupPortMappings == nil {
		w.addressGroupPortMappings = make(map[string]models.AddressGroupPortMapping)
		// Всегда копируем существующие маппинги портов групп адресов, чтобы иметь полную карту для работы
		for k, v := range w.registry.db.GetAddressGroupPortMappings() {
			w.addressGroupPortMappings[k] = v
		}
	}

	// Обработка в зависимости от типа операции
	switch syncOp {
	case models.SyncOpFullSync:
		// Если scope не пустой, удаляем только маппинги портов групп адресов в указанной области
		if scope != nil && !scope.IsEmpty() {
			// Проверяем, что scope имеет тип ResourceIdentifierScope
			if ris, ok := scope.(ports.ResourceIdentifierScope); ok && !ris.IsEmpty() {
				// Создаем временную карту для хранения маппингов портов групп адресов вне области видимости
				tempMappings := make(map[string]models.AddressGroupPortMapping)

				// Создаем карту идентификаторов в области видимости для быстрого поиска
				scopeIds := make(map[string]bool)
				for _, id := range ris.Identifiers {
					scopeIds[id.Key()] = true
				}

				// Сохраняем маппинги портов групп адресов вне области видимости
				for k, v := range w.addressGroupPortMappings {
					if !scopeIds[k] {
						tempMappings[k] = v
					}
				}

				// Очищаем карту и восстанавливаем маппинги портов групп адресов вне области видимости
				w.addressGroupPortMappings = make(map[string]models.AddressGroupPortMapping)
				for k, v := range tempMappings {
					w.addressGroupPortMappings[k] = v
				}
			} else {
				// Если scope не ResourceIdentifierScope, но не пустой,
				// то мы не знаем, как его обрабатывать, поэтому не удаляем ничего
			}
		} else {
			// Если область пуста, очищаем всю карту
			w.addressGroupPortMappings = make(map[string]models.AddressGroupPortMapping)
		}

		// Добавляем новые маппинги портов групп адресов
		for _, mapping := range mappings {
			key := mapping.Key()
			if existing, ok := w.addressGroupPortMappings[key]; ok {
				if mapping.Meta.CreationTS.IsZero() {
					mapping.Meta.CreationTS = existing.Meta.CreationTS
				}
				if mapping.Meta.UID == "" {
					mapping.Meta.UID = existing.Meta.UID
				}
			}
			ensureMetaFill(&mapping.Meta)
			w.addressGroupPortMappings[key] = mapping
		}

	case models.SyncOpUpsert:
		// Только добавление и обновление
		for _, mapping := range mappings {
			key := mapping.Key()
			if existing, ok := w.addressGroupPortMappings[key]; ok {
				if mapping.Meta.CreationTS.IsZero() {
					mapping.Meta.CreationTS = existing.Meta.CreationTS
				}
				if mapping.Meta.UID == "" {
					mapping.Meta.UID = existing.Meta.UID
				}
			}
			ensureMetaFill(&mapping.Meta)
			w.addressGroupPortMappings[key] = mapping
		}

	case models.SyncOpDelete:
		// Только удаление
		for _, mapping := range mappings {
			delete(w.addressGroupPortMappings, mapping.Key())
		}
	}

	return nil
}

func (w *writer) SyncRuleS2S(ctx context.Context, rules []models.RuleS2S, scope ports.Scope, opts ...ports.Option) error {
	// Определение операции (по умолчанию FullSync)
	syncOp := models.SyncOpFullSync

	// Извлечение опций
	for _, opt := range opts {
		if so, ok := opt.(ports.SyncOption); ok {
			syncOp = so.Operation
		}
	}

	// Инициализация карты, если она еще не создана
	if w.ruleS2S == nil {
		w.ruleS2S = make(map[string]models.RuleS2S)
		// Всегда копируем существующие правила, чтобы иметь полную карту для работы
		for k, v := range w.registry.db.GetRuleS2S() {
			w.ruleS2S[k] = v
		}
	}

	// Обработка в зависимости от типа операции
	switch syncOp {
	case models.SyncOpFullSync:
		// Если scope не пустой, удаляем только правила в указанной области
		if scope != nil && !scope.IsEmpty() {
			// Проверяем, что scope имеет тип ResourceIdentifierScope
			if ris, ok := scope.(ports.ResourceIdentifierScope); ok && !ris.IsEmpty() {
				// Создаем временную карту для хранения правил вне области видимости
				tempRules := make(map[string]models.RuleS2S)

				// Создаем карту идентификаторов в области видимости для быстрого поиска
				scopeIds := make(map[string]bool)
				for _, id := range ris.Identifiers {
					scopeIds[id.Key()] = true
				}

				// Сохраняем правила вне области видимости
				for k, v := range w.ruleS2S {
					if !scopeIds[k] {
						tempRules[k] = v
					}
				}

				// Очищаем карту и восстанавливаем правила вне области видимости
				w.ruleS2S = make(map[string]models.RuleS2S)
				for k, v := range tempRules {
					w.ruleS2S[k] = v
				}
			} else {
				// Если scope не ResourceIdentifierScope, но не пустой,
				// то мы не знаем, как его обрабатывать, поэтому не удаляем ничего
			}
		} else {
			// Если область пуста, очищаем всю карту
			w.ruleS2S = make(map[string]models.RuleS2S)
		}

		// Добавляем новые правила
		for _, rule := range rules {
			key := rule.Key()
			if existing, ok := w.ruleS2S[key]; ok {
				if rule.Meta.CreationTS.IsZero() {
					rule.Meta.CreationTS = existing.Meta.CreationTS
				}
				if rule.Meta.UID == "" {
					rule.Meta.UID = existing.Meta.UID
				}
			}
			ensureMetaFill(&rule.Meta)
			w.ruleS2S[key] = rule
		}

	case models.SyncOpUpsert:
		// Только добавление и обновление
		for _, rule := range rules {
			key := rule.Key()
			if existing, ok := w.ruleS2S[key]; ok {
				if rule.Meta.CreationTS.IsZero() {
					rule.Meta.CreationTS = existing.Meta.CreationTS
				}
				if rule.Meta.UID == "" {
					rule.Meta.UID = existing.Meta.UID
				}
			}
			ensureMetaFill(&rule.Meta)
			w.ruleS2S[key] = rule
		}

	case models.SyncOpDelete:
		// Только удаление
		for _, rule := range rules {
			delete(w.ruleS2S, rule.Key())
		}
	}

	return nil
}

func (w *writer) SyncServiceAliases(ctx context.Context, aliases []models.ServiceAlias, scope ports.Scope, opts ...ports.Option) error {
	// Определение операции (по умолчанию FullSync)
	syncOp := models.SyncOpFullSync

	// Извлечение опций
	for _, opt := range opts {
		if so, ok := opt.(ports.SyncOption); ok {
			syncOp = so.Operation
		}
	}

	// Инициализация карты, если она еще не создана
	if w.serviceAliases == nil {
		w.serviceAliases = make(map[string]models.ServiceAlias)
		// Всегда копируем существующие алиасы сервисов, чтобы иметь полную карту для работы
		for k, v := range w.registry.db.GetServiceAliases() {
			w.serviceAliases[k] = v
		}
	}

	// Обработка в зависимости от типа операции
	switch syncOp {
	case models.SyncOpFullSync:
		// Если scope не пустой, удаляем только алиасы сервисов в указанной области
		if scope != nil && !scope.IsEmpty() {
			// Проверяем, что scope имеет тип ResourceIdentifierScope
			if ris, ok := scope.(ports.ResourceIdentifierScope); ok && !ris.IsEmpty() {
				// Создаем временную карту для хранения алиасов сервисов вне области видимости
				tempAliases := make(map[string]models.ServiceAlias)

				// Создаем карту идентификаторов в области видимости для быстрого поиска
				scopeIds := make(map[string]bool)
				for _, id := range ris.Identifiers {
					scopeIds[id.Key()] = true
				}

				// Сохраняем алиасы сервисов вне области видимости
				for k, v := range w.serviceAliases {
					if !scopeIds[k] {
						tempAliases[k] = v
					}
				}

				// Очищаем карту и восстанавливаем алиасы сервисов вне области видимости
				w.serviceAliases = make(map[string]models.ServiceAlias)
				for k, v := range tempAliases {
					w.serviceAliases[k] = v
				}
			} else {
				// Если scope не ResourceIdentifierScope, но не пустой,
				// то мы не знаем, как его обрабатывать, поэтому не удаляем ничего
			}
		} else {
			// Если область пуста, очищаем всю карту
			w.serviceAliases = make(map[string]models.ServiceAlias)
		}

		// Добавляем новые алиасы сервисов
		for _, alias := range aliases {
			if existing, ok := w.serviceAliases[alias.Key()]; ok {
				if alias.Meta.CreationTS.IsZero() {
					alias.Meta.CreationTS = existing.Meta.CreationTS
				}
				if alias.Meta.UID == "" {
					alias.Meta.UID = existing.Meta.UID
				}
			}
			ensureMetaFill(&alias.Meta)
			w.serviceAliases[alias.Key()] = alias
		}

	case models.SyncOpUpsert:
		// Только добавление и обновление
		for _, alias := range aliases {
			if existing, ok := w.serviceAliases[alias.Key()]; ok {
				if alias.Meta.CreationTS.IsZero() {
					alias.Meta.CreationTS = existing.Meta.CreationTS
				}
				if alias.Meta.UID == "" {
					alias.Meta.UID = existing.Meta.UID
				}
			}
			ensureMetaFill(&alias.Meta)
			w.serviceAliases[alias.Key()] = alias
		}

	case models.SyncOpDelete:
		// Только удаление
		for _, alias := range aliases {
			delete(w.serviceAliases, alias.Key())
		}
	}

	return nil
}

func (w *writer) SyncAddressGroupBindingPolicies(ctx context.Context, policies []models.AddressGroupBindingPolicy, scope ports.Scope, opts ...ports.Option) error {
	// Определение операции (по умолчанию FullSync)
	syncOp := models.SyncOpFullSync

	// Извлечение опций
	for _, opt := range opts {
		if so, ok := opt.(ports.SyncOption); ok {
			syncOp = so.Operation
		}
	}

	// Инициализация карты, если она еще не создана
	if w.addressGroupBindingPolicies == nil {
		w.addressGroupBindingPolicies = make(map[string]models.AddressGroupBindingPolicy)
		// Всегда копируем существующие политики, чтобы иметь полную карту для работы
		for k, v := range w.registry.db.GetAddressGroupBindingPolicies() {
			w.addressGroupBindingPolicies[k] = v
		}
	}

	// Обработка в зависимости от типа операции
	switch syncOp {
	case models.SyncOpFullSync:
		// Если scope не пустой, удаляем только политики в указанной области
		if scope != nil && !scope.IsEmpty() {
			// Проверяем, что scope имеет тип ResourceIdentifierScope
			if ris, ok := scope.(ports.ResourceIdentifierScope); ok && !ris.IsEmpty() {
				// Создаем временную карту для хранения политик вне области видимости
				tempPolicies := make(map[string]models.AddressGroupBindingPolicy)

				// Создаем карту идентификаторов в области видимости для быстрого поиска
				scopeIds := make(map[string]bool)
				for _, id := range ris.Identifiers {
					scopeIds[id.Key()] = true
				}

				// Сохраняем политики вне области видимости
				for k, v := range w.addressGroupBindingPolicies {
					if !scopeIds[k] {
						tempPolicies[k] = v
					}
				}

				// Очищаем карту и восстанавливаем политики вне области видимости
				w.addressGroupBindingPolicies = make(map[string]models.AddressGroupBindingPolicy)
				for k, v := range tempPolicies {
					w.addressGroupBindingPolicies[k] = v
				}
			} else {
				// Если scope не ResourceIdentifierScope, но не пустой,
				// то мы не знаем, как его обрабатывать, поэтому не удаляем ничего
			}
		} else {
			// Если область пуста, очищаем всю карту
			w.addressGroupBindingPolicies = make(map[string]models.AddressGroupBindingPolicy)
		}

		// Добавляем новые политики
		for i := range policies {
			p := policies[i]
			if p.Meta.UID == "" {
				p.Meta.TouchOnCreate()
			}
			if p.Meta.ResourceVersion == "" {
				p.Meta.ResourceVersion = fmt.Sprintf("%d", time.Now().UnixNano())
			}
			if p.Meta.Generation == 0 {
				p.Meta.Generation = 1
			}
			w.addressGroupBindingPolicies[p.Key()] = p
		}

	case models.SyncOpUpsert:
		// Только добавление и обновление
		for _, policy := range policies {
			w.addressGroupBindingPolicies[policy.Key()] = policy
		}

	case models.SyncOpDelete:
		// Только удаление
		for _, policy := range policies {
			delete(w.addressGroupBindingPolicies, policy.Key())
		}
	}

	return nil
}

func (w *writer) Commit() error {

	if w.services != nil {
		for _, _ = range w.services {
		}
		w.registry.db.SetServices(w.services)
	} else {
	}

	if w.serviceAliases != nil {
		w.registry.db.SetServiceAliases(w.serviceAliases)
	}
	if w.addressGroups != nil {
		for _, _ = range w.addressGroups {
		}
		w.registry.db.SetAddressGroups(w.addressGroups)
	}
	if w.addressGroupBindings != nil {
		w.registry.db.SetAddressGroupBindings(w.addressGroupBindings)
	} else {
	}
	if w.addressGroupPortMappings != nil {
		w.registry.db.SetAddressGroupPortMappings(w.addressGroupPortMappings)
	}
	if w.addressGroupBindingPolicies != nil {
		w.registry.db.SetAddressGroupBindingPolicies(w.addressGroupBindingPolicies)
	}
	if w.ruleS2S != nil {
		w.registry.db.SetRuleS2S(w.ruleS2S)
	}
	if w.ieAgAgRules != nil {
		w.registry.db.SetIEAgAgRules(w.ieAgAgRules)
	}

	if w.networks != nil {
		for _, network := range w.networks {
			if network.BindingRef != nil {
			}
			if network.AddressGroupRef != nil {
			}
		}
		w.registry.db.SetNetworks(w.networks)
	}

	if w.networkBindings != nil {
		for _, _ = range w.networkBindings {
		}
		w.registry.db.SetNetworkBindings(w.networkBindings)
	}

	if w.hosts != nil {
		w.registry.db.SetHosts(w.hosts)
	}

	if w.hostBindings != nil {
		w.registry.db.SetHostBindings(w.hostBindings)
	}

	w.registry.db.SetSyncStatus(models.SyncStatus{
		UpdatedAt: time.Now(),
	})

	return nil
}

// DeleteServicesByIDs deletes services by IDs
func (w *writer) DeleteServicesByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	if w.services == nil {
		w.services = make(map[string]models.Service)
		// Copy existing services
		for k, v := range w.registry.db.GetServices() {
			w.services[k] = v
		}
	}

	for _, id := range ids {
		delete(w.services, id.Key())
	}

	return nil
}

// DeleteAddressGroupsByIDs deletes address groups by IDs
func (w *writer) DeleteAddressGroupsByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	if w.addressGroups == nil {
		w.addressGroups = make(map[string]models.AddressGroup)
		// Copy existing address groups
		for k, v := range w.registry.db.GetAddressGroups() {
			w.addressGroups[k] = v
		}
	}

	for _, id := range ids {
		delete(w.addressGroups, id.Key())
	}

	return nil
}

// DeleteAddressGroupBindingsByIDs deletes address group bindings by IDs
func (w *writer) DeleteAddressGroupBindingsByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {

	if w.addressGroupBindings == nil {
		w.addressGroupBindings = make(map[string]models.AddressGroupBinding)
		// Copy existing address group bindings
		existing := w.registry.db.GetAddressGroupBindings()
		for k, v := range existing {
			w.addressGroupBindings[k] = v
		}
	}

	for _, id := range ids {
		key := id.Key()
		if _, exists := w.addressGroupBindings[key]; exists {
			delete(w.addressGroupBindings, key)
		} else {
		}
	}

	return nil
}

// DeleteAddressGroupPortMappingsByIDs deletes address group port mappings by IDs
func (w *writer) DeleteAddressGroupPortMappingsByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	if w.addressGroupPortMappings == nil {
		w.addressGroupPortMappings = make(map[string]models.AddressGroupPortMapping)
		// Copy existing address group port mappings
		for k, v := range w.registry.db.GetAddressGroupPortMappings() {
			w.addressGroupPortMappings[k] = v
		}
	}

	for _, id := range ids {
		delete(w.addressGroupPortMappings, id.Key())
	}

	return nil
}

// DeleteRuleS2SByIDs deletes rule s2s by IDs
func (w *writer) DeleteRuleS2SByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	if w.ruleS2S == nil {
		w.ruleS2S = make(map[string]models.RuleS2S)
		// Copy existing rule s2s
		for k, v := range w.registry.db.GetRuleS2S() {
			w.ruleS2S[k] = v
		}
	}

	for _, id := range ids {
		delete(w.ruleS2S, id.Key())
	}

	return nil
}

// DeleteServiceAliasesByIDs deletes service aliases by IDs
func (w *writer) DeleteServiceAliasesByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	if w.serviceAliases == nil {
		w.serviceAliases = make(map[string]models.ServiceAlias)
		// Copy existing service aliases
		for k, v := range w.registry.db.GetServiceAliases() {
			w.serviceAliases[k] = v
		}
	}

	for _, id := range ids {
		delete(w.serviceAliases, id.Key())
	}

	return nil
}

// DeleteAddressGroupBindingPoliciesByIDs deletes address group binding policies by IDs
func (w *writer) DeleteAddressGroupBindingPoliciesByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	if w.addressGroupBindingPolicies == nil {
		w.addressGroupBindingPolicies = make(map[string]models.AddressGroupBindingPolicy)
		// Copy existing address group binding policies
		for k, v := range w.registry.db.GetAddressGroupBindingPolicies() {
			w.addressGroupBindingPolicies[k] = v
		}
	}

	for _, id := range ids {
		delete(w.addressGroupBindingPolicies, id.Key())
	}

	return nil
}

// SyncIEAgAgRules синхронизирует правила IEAgAgRule
func (w *writer) SyncIEAgAgRules(ctx context.Context, rules []models.IEAgAgRule, scope ports.Scope, opts ...ports.Option) error {
	// Определение операции (по умолчанию FullSync)
	syncOp := models.SyncOpFullSync

	// Извлечение опций
	for _, opt := range opts {
		if so, ok := opt.(ports.SyncOption); ok {
			syncOp = so.Operation
		}
	}

	// Инициализация карты, если она еще не создана
	if w.ieAgAgRules == nil {
		w.ieAgAgRules = make(map[string]models.IEAgAgRule)
		// Всегда копируем существующие правила, чтобы иметь полную карту для работы
		for k, v := range w.registry.db.GetIEAgAgRules() {
			w.ieAgAgRules[k] = v
		}
	}

	// Обработка в зависимости от типа операции
	switch syncOp {
	case models.SyncOpFullSync:
		// Если scope не пустой, удаляем только правила в указанной области
		if scope != nil && !scope.IsEmpty() {
			// Проверяем, что scope имеет тип ResourceIdentifierScope
			if ris, ok := scope.(ports.ResourceIdentifierScope); ok && !ris.IsEmpty() {
				// Создаем временную карту для хранения правил вне области видимости
				tempRules := make(map[string]models.IEAgAgRule)

				// Создаем карту идентификаторов в области видимости для быстрого поиска
				scopeIds := make(map[string]bool)
				for _, id := range ris.Identifiers {
					scopeIds[id.Key()] = true
				}

				// Сохраняем правила вне области видимости
				for k, v := range w.ieAgAgRules {
					if !scopeIds[k] {
						tempRules[k] = v
					}
				}

				// Очищаем карту и восстанавливаем правила вне области видимости
				w.ieAgAgRules = make(map[string]models.IEAgAgRule)
				for k, v := range tempRules {
					w.ieAgAgRules[k] = v
				}
			} else {
				// Если scope не ResourceIdentifierScope, но не пустой,
				// то мы не знаем, как его обрабатывать, поэтому не удаляем ничего
			}
		} else {
			// Если область пуста, очищаем всю карту
			w.ieAgAgRules = make(map[string]models.IEAgAgRule)
		}

		// Добавляем новые правила
		for _, rule := range rules {
			key := rule.Key()
			if existing, ok := w.ieAgAgRules[key]; ok {
				if rule.Meta.CreationTS.IsZero() {
					rule.Meta.CreationTS = existing.Meta.CreationTS
				}
				if rule.Meta.UID == "" {
					rule.Meta.UID = existing.Meta.UID
				}
			}
			ensureMetaFill(&rule.Meta)
			w.ieAgAgRules[key] = rule
		}

	case models.SyncOpUpsert:
		// Только добавление и обновление
		for _, rule := range rules {
			key := rule.Key()
			if existing, ok := w.ieAgAgRules[key]; ok {
				if rule.Meta.CreationTS.IsZero() {
					rule.Meta.CreationTS = existing.Meta.CreationTS
				}
				if rule.Meta.UID == "" {
					rule.Meta.UID = existing.Meta.UID
				}
			}
			ensureMetaFill(&rule.Meta)
			w.ieAgAgRules[key] = rule
		}

	case models.SyncOpDelete:
		// Только удаление
		for _, rule := range rules {
			delete(w.ieAgAgRules, rule.Key())
		}
	}

	return nil
}

// DeleteIEAgAgRulesByIDs deletes IEAgAgRules by IDs
func (w *writer) DeleteIEAgAgRulesByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	// Инициализация карты, если она еще не создана
	if w.ieAgAgRules == nil {
		w.ieAgAgRules = make(map[string]models.IEAgAgRule)
		// Всегда копируем существующие правила, чтобы иметь полную карту для работы
		for k, v := range w.registry.db.GetIEAgAgRules() {
			w.ieAgAgRules[k] = v
		}
	}

	// Удаляем правила по идентификаторам
	for _, id := range ids {
		delete(w.ieAgAgRules, id.Key())
	}

	return nil
}

func (w *writer) SyncNetworks(ctx context.Context, networks []models.Network, scope ports.Scope, opts ...ports.Option) error {
	// Определение операции (по умолчанию FullSync)
	syncOp := models.SyncOpFullSync

	// Извлечение опций
	for _, opt := range opts {
		if so, ok := opt.(ports.SyncOption); ok {
			syncOp = so.Operation
		}
	}

	// Инициализация карты, если она еще не создана
	if w.networks == nil {
		w.networks = make(map[string]models.Network)
		// Всегда копируем существующие сети, чтобы иметь полную карту для работы
		for k, v := range w.registry.db.GetNetworks() {
			w.networks[k] = v
		}
	}

	// Обработка в зависимости от типа операции
	switch syncOp {
	case models.SyncOpFullSync:
		// Если scope не пустой, удаляем только сети в указанной области
		if scope != nil && !scope.IsEmpty() {
			// Проверяем, что scope имеет тип ResourceIdentifierScope
			if ris, ok := scope.(ports.ResourceIdentifierScope); ok && !ris.IsEmpty() {
				// Создаем временную карту для хранения сетей вне области видимости
				tempNetworks := make(map[string]models.Network)

				// Создаем карту идентификаторов в области видимости для быстрого поиска
				scopeIds := make(map[string]bool)
				for _, id := range ris.Identifiers {
					scopeIds[id.Key()] = true
				}

				// Сохраняем сети вне области видимости
				for k, v := range w.networks {
					// Проверяем, входит ли сеть в область видимости
					networkKey := v.Key()
					if !scopeIds[networkKey] {
						// Сохраняем сети, которые не входят в область видимости
						tempNetworks[k] = v
					}
				}

				// Очищаем карту и восстанавливаем сети вне области видимости
				w.networks = make(map[string]models.Network)
				for k, v := range tempNetworks {
					w.networks[k] = v
				}
			}
		} else {
			// Если область пуста, очищаем всю карту
			w.networks = make(map[string]models.Network)
		}

		// Добавляем новые сети
		for _, network := range networks {
			if existing, ok := w.networks[network.Key()]; ok {
				if network.Meta.CreationTS.IsZero() {
					network.Meta.CreationTS = existing.Meta.CreationTS
				}
				if network.Meta.UID == "" {
					network.Meta.UID = existing.Meta.UID
				}
			}
			ensureMetaFill(&network.Meta)
			w.networks[network.Key()] = network
		}

	case models.SyncOpUpsert:
		// Добавляем или обновляем сети
		for _, network := range networks {
			if existing, ok := w.networks[network.Key()]; ok {
				if network.Meta.CreationTS.IsZero() {
					network.Meta.CreationTS = existing.Meta.CreationTS
				}
				if network.Meta.UID == "" {
					network.Meta.UID = existing.Meta.UID
				}
			}
			ensureMetaFill(&network.Meta)
			w.networks[network.Key()] = network
		}

	case models.SyncOpDelete:
		// Удаляем сети
		for _, network := range networks {
			delete(w.networks, network.Key())
		}
	}

	return nil
}

func (w *writer) SyncNetworkBindings(ctx context.Context, bindings []models.NetworkBinding, scope ports.Scope, opts ...ports.Option) error {
	// Определение операции (по умолчанию FullSync)
	syncOp := models.SyncOpFullSync

	// Извлечение опций
	for _, opt := range opts {
		if so, ok := opt.(ports.SyncOption); ok {
			syncOp = so.Operation
		}
	}

	// Инициализация карты, если она еще не создана
	if w.networkBindings == nil {
		w.networkBindings = make(map[string]models.NetworkBinding)
		// Всегда копируем существующие binding'и, чтобы иметь полную карту для работы
		for k, v := range w.registry.db.GetNetworkBindings() {
			w.networkBindings[k] = v
		}
	}

	// Обработка в зависимости от типа операции
	switch syncOp {
	case models.SyncOpFullSync:
		// Если scope не пустой, удаляем только binding'и в указанной области
		if scope != nil && !scope.IsEmpty() {
			// Проверяем, что scope имеет тип ResourceIdentifierScope
			if ris, ok := scope.(ports.ResourceIdentifierScope); ok && !ris.IsEmpty() {
				// Создаем временную карту для хранения binding'ов вне области видимости
				tempBindings := make(map[string]models.NetworkBinding)

				// Создаем карту идентификаторов в области видимости для быстрого поиска
				scopeIds := make(map[string]bool)
				for _, id := range ris.Identifiers {
					scopeIds[id.Key()] = true
				}

				// Сохраняем binding'и вне области видимости
				for k, v := range w.networkBindings {
					// Проверяем, входит ли binding в область видимости
					bindingKey := v.Key()
					if !scopeIds[bindingKey] {
						// Сохраняем binding'и, которые не входят в область видимости
						tempBindings[k] = v
					}
				}

				// Очищаем карту и восстанавливаем binding'и вне области видимости
				w.networkBindings = make(map[string]models.NetworkBinding)
				for k, v := range tempBindings {
					w.networkBindings[k] = v
				}
			}
		} else {
			// Если область пуста, очищаем всю карту
			w.networkBindings = make(map[string]models.NetworkBinding)
		}

		// Добавляем новые binding'и
		for _, binding := range bindings {
			if existing, ok := w.networkBindings[binding.Key()]; ok {
				if binding.Meta.CreationTS.IsZero() {
					binding.Meta.CreationTS = existing.Meta.CreationTS
				}
				if binding.Meta.UID == "" {
					binding.Meta.UID = existing.Meta.UID
				}
			}
			ensureMetaFill(&binding.Meta)
			w.networkBindings[binding.Key()] = binding
		}

	case models.SyncOpUpsert:
		// Добавляем или обновляем binding'и
		for _, binding := range bindings {
			if existing, ok := w.networkBindings[binding.Key()]; ok {
				if binding.Meta.CreationTS.IsZero() {
					binding.Meta.CreationTS = existing.Meta.CreationTS
				}
				if binding.Meta.UID == "" {
					binding.Meta.UID = existing.Meta.UID
				}
			}
			ensureMetaFill(&binding.Meta)
			w.networkBindings[binding.Key()] = binding
		}

	case models.SyncOpDelete:
		// Удаляем binding'и
		for _, binding := range bindings {
			delete(w.networkBindings, binding.Key())
		}
	}

	return nil
}

func (w *writer) DeleteNetworksByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	// Инициализация карты, если она еще не создана
	if w.networks == nil {
		w.networks = make(map[string]models.Network)
		// Всегда копируем существующие сети, чтобы иметь полную карту для работы
		for k, v := range w.registry.db.GetNetworks() {
			w.networks[k] = v
		}
	}

	// Удаляем сети по идентификаторам
	for _, id := range ids {
		delete(w.networks, id.Key())
	}

	return nil
}

func (w *writer) DeleteNetworkBindingsByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	// Инициализация карты, если она еще не создана
	if w.networkBindings == nil {
		w.networkBindings = make(map[string]models.NetworkBinding)
		// Всегда копируем существующие binding'и, чтобы иметь полную карту для работы
		for k, v := range w.registry.db.GetNetworkBindings() {
			w.networkBindings[k] = v
		}
	}

	// Удаляем binding'и по идентификаторам
	for _, id := range ids {
		delete(w.networkBindings, id.Key())
	}

	return nil
}

func (w *writer) SyncHosts(ctx context.Context, hosts []models.Host, scope ports.Scope, opts ...ports.Option) error {
	// Определение операции (по умолчанию FullSync)
	syncOp := models.SyncOpFullSync

	// Извлечение опций
	for _, opt := range opts {
		if so, ok := opt.(ports.SyncOption); ok {
			syncOp = so.Operation
		}
	}

	// Инициализация карты, если она еще не создана
	if w.hosts == nil {
		w.hosts = make(map[string]models.Host)
		// Всегда копируем существующие хосты, чтобы иметь полную карту для работы
		for k, v := range w.registry.db.GetHosts() {
			w.hosts[k] = v
		}
	}

	// Обработка в зависимости от типа операции
	switch syncOp {
	case models.SyncOpFullSync:
		// Если scope не пустой, удаляем только хосты в указанной области
		if scope != nil && !scope.IsEmpty() {
			// Проверяем, что scope имеет тип ResourceIdentifierScope
			if ris, ok := scope.(ports.ResourceIdentifierScope); ok && !ris.IsEmpty() {
				// Создаем временную карту для хранения хостов вне области видимости
				tempHosts := make(map[string]models.Host)

				// Создаем карту идентификаторов в области видимости для быстрого поиска
				scopeIds := make(map[string]bool)
				for _, id := range ris.Identifiers {
					scopeIds[id.Key()] = true
				}

				// Сохраняем хосты вне области видимости
				for k, v := range w.hosts {
					// Проверяем, входит ли хост в область видимости
					hostKey := v.Key()
					if !scopeIds[hostKey] {
						// Сохраняем хосты, которые не входят в область видимости
						tempHosts[k] = v
					}
				}

				// Очищаем карту и восстанавливаем хосты вне области видимости
				w.hosts = make(map[string]models.Host)
				for k, v := range tempHosts {
					w.hosts[k] = v
				}
			}
		} else {
			// Если область пуста, очищаем всю карту
			w.hosts = make(map[string]models.Host)
		}

		// Добавляем новые хосты
		for _, host := range hosts {
			if existing, ok := w.hosts[host.Key()]; ok {
				if host.Meta.CreationTS.IsZero() {
					host.Meta.CreationTS = existing.Meta.CreationTS
				}
				if host.Meta.UID == "" {
					host.Meta.UID = existing.Meta.UID
				}
			}
			ensureMetaFill(&host.Meta)
			w.hosts[host.Key()] = host
		}

	case models.SyncOpUpsert:
		// Добавляем или обновляем хосты
		for _, host := range hosts {
			if existing, ok := w.hosts[host.Key()]; ok {
				if host.Meta.CreationTS.IsZero() {
					host.Meta.CreationTS = existing.Meta.CreationTS
				}
				if host.Meta.UID == "" {
					host.Meta.UID = existing.Meta.UID
				}
			}
			ensureMetaFill(&host.Meta)
			w.hosts[host.Key()] = host
		}

	case models.SyncOpDelete:
		// Удаляем хосты
		for _, host := range hosts {
			delete(w.hosts, host.Key())
		}
	}

	return nil
}

func (w *writer) SyncHostBindings(ctx context.Context, hostBindings []models.HostBinding, scope ports.Scope, opts ...ports.Option) error {
	// Определение операции (по умолчанию FullSync)
	syncOp := models.SyncOpFullSync

	// Извлечение опций
	for _, opt := range opts {
		if so, ok := opt.(ports.SyncOption); ok {
			syncOp = so.Operation
		}
	}

	// Инициализация карты, если она еще не создана
	if w.hostBindings == nil {
		w.hostBindings = make(map[string]models.HostBinding)
		// Всегда копируем существующие binding'и хостов, чтобы иметь полную карту для работы
		for k, v := range w.registry.db.GetHostBindings() {
			w.hostBindings[k] = v
		}
	}

	// Обработка в зависимости от типа операции
	switch syncOp {
	case models.SyncOpFullSync:
		// Если scope не пустой, удаляем только binding'и хостов в указанной области
		if scope != nil && !scope.IsEmpty() {
			// Проверяем, что scope имеет тип ResourceIdentifierScope
			if ris, ok := scope.(ports.ResourceIdentifierScope); ok && !ris.IsEmpty() {
				// Создаем временную карту для хранения binding'ов хостов вне области видимости
				tempHostBindings := make(map[string]models.HostBinding)

				// Создаем карту идентификаторов в области видимости для быстрого поиска
				scopeIds := make(map[string]bool)
				for _, id := range ris.Identifiers {
					scopeIds[id.Key()] = true
				}

				// Сохраняем binding'и хостов вне области видимости
				for k, v := range w.hostBindings {
					// Проверяем, входит ли binding хоста в область видимости
					bindingKey := v.Key()
					if !scopeIds[bindingKey] {
						// Сохраняем binding'и хостов, которые не входят в область видимости
						tempHostBindings[k] = v
					}
				}

				// Очищаем карту и восстанавливаем binding'и хостов вне области видимости
				w.hostBindings = make(map[string]models.HostBinding)
				for k, v := range tempHostBindings {
					w.hostBindings[k] = v
				}
			}
		} else {
			// Если область пуста, очищаем всю карту
			w.hostBindings = make(map[string]models.HostBinding)
		}

		// Добавляем новые binding'и хостов
		for _, hostBinding := range hostBindings {
			if existing, ok := w.hostBindings[hostBinding.Key()]; ok {
				if hostBinding.Meta.CreationTS.IsZero() {
					hostBinding.Meta.CreationTS = existing.Meta.CreationTS
				}
				if hostBinding.Meta.UID == "" {
					hostBinding.Meta.UID = existing.Meta.UID
				}
			}
			ensureMetaFill(&hostBinding.Meta)
			w.hostBindings[hostBinding.Key()] = hostBinding
		}

	case models.SyncOpUpsert:
		// Добавляем или обновляем binding'и хостов
		for _, hostBinding := range hostBindings {
			if existing, ok := w.hostBindings[hostBinding.Key()]; ok {
				if hostBinding.Meta.CreationTS.IsZero() {
					hostBinding.Meta.CreationTS = existing.Meta.CreationTS
				}
				if hostBinding.Meta.UID == "" {
					hostBinding.Meta.UID = existing.Meta.UID
				}
			}
			ensureMetaFill(&hostBinding.Meta)
			w.hostBindings[hostBinding.Key()] = hostBinding
		}

	case models.SyncOpDelete:
		// Удаляем binding'и хостов
		for _, hostBinding := range hostBindings {
			delete(w.hostBindings, hostBinding.Key())
		}
	}

	return nil
}

func (w *writer) DeleteHostsByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	// Инициализация карты, если она еще не создана
	if w.hosts == nil {
		w.hosts = make(map[string]models.Host)
		// Всегда копируем существующие хосты, чтобы иметь полную карту для работы
		for k, v := range w.registry.db.GetHosts() {
			w.hosts[k] = v
		}
	}

	// Удаляем хосты по идентификаторам
	for _, id := range ids {
		delete(w.hosts, id.Key())
	}

	return nil
}

func (w *writer) DeleteHostBindingsByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	// Инициализация карты, если она еще не создана
	if w.hostBindings == nil {
		w.hostBindings = make(map[string]models.HostBinding)
		// Всегда копируем существующие binding'и хостов, чтобы иметь полную карту для работы
		for k, v := range w.registry.db.GetHostBindings() {
			w.hostBindings[k] = v
		}
	}

	// Удаляем binding'и хостов по идентификаторам
	for _, id := range ids {
		delete(w.hostBindings, id.Key())
	}

	return nil
}

func (w *writer) Abort() {
	w.services = nil
	w.addressGroups = nil
	w.addressGroupBindings = nil
	w.addressGroupPortMappings = nil
	w.addressGroupBindingPolicies = nil
	w.ruleS2S = nil
	w.serviceAliases = nil
	w.ieAgAgRules = nil
	w.networks = nil
	w.networkBindings = nil
	w.hosts = nil
	w.hostBindings = nil
}

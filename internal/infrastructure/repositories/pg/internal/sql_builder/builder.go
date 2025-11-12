package sql_builder

import (
	"fmt"
	"strings"

	"netguard-pg-backend/internal/domain/models"
	netguardpb "netguard-pg-backend/protos/pkg/api/netguard"
)

// SQLBuilder генерирует SQL WHERE clauses
type SQLBuilder struct{}

// NewSQLBuilder создает новый SQL builder
func NewSQLBuilder() *SQLBuilder {
	return &SQLBuilder{}
}

// BuildCombinedWHERE генерирует SQL WHERE clause из identifiers + field-selectors
//
// Логика комбинирования:
// - IF identifiers AND field_selectors: (identifier_filter) AND (field_selector_filter)
// - IF only identifiers: (identifier_filter)
// - IF only field_selectors: (field_selector_filter)
// - IF neither: "" (no filter)
//
// Parameters:
//
//	table - имя таблицы БД ("hosts", "address_groups")
//	tableAlias - алиас таблицы в запросе (e.g. "h", "ag")
//	identifiers - список ResourceIdentifier для точного запроса
//	fieldSelectors - список FieldSelector для фильтрации по полям
//	startParamNum - начальный номер для PostgreSQL placeholders ($1, $2, ...)
//
// Returns:
//
//	whereClause - SQL условие
//	args - значения для prepared statement
//	error - ошибка если маппинг не найден
func (b *SQLBuilder) BuildCombinedWHERE(
	table string,
	tableAlias string,
	identifiers []models.ResourceIdentifier,
	fieldSelectors []*netguardpb.FieldSelector,
	labelSelectors []*netguardpb.LabelSelector,
	startParamNum int,
) (whereClause string, args []interface{}, err error) {

	var clauses []string
	paramNum := startParamNum

	// 1. Build identifiers filter (OR logic)
	if len(identifiers) > 0 {
		identClause, identArgs, err := b.buildIdentifiersWHERE(
			tableAlias,
			identifiers,
			paramNum,
		)
		if err != nil {
			return "", nil, err
		}
		if identClause != "" {
			clauses = append(clauses, "("+identClause+")")
			args = append(args, identArgs...)
			paramNum += len(identArgs)
		}
	}

	// 2. Build field selectors filter (AND logic)
	if len(fieldSelectors) > 0 {
		fsClause, fsArgs, newParamNum, err := b.buildFieldSelectorsWHERE(
			table,
			tableAlias,
			fieldSelectors,
			paramNum,
		)
		if err != nil {
			return "", nil, err
		}
		if fsClause != "" {
			clauses = append(clauses, "("+fsClause+")")
			args = append(args, fsArgs...)
			paramNum = newParamNum
		}
	}

	// 3. Build label selectors filter (AND logic)
	if len(labelSelectors) > 0 {
		lsClause, lsArgs, newParamNum, err := b.buildLabelSelectorsWHERE(
			tableAlias,
			labelSelectors,
			paramNum,
		)
		if err != nil {
			return "", nil, err
		}
		if lsClause != "" {
			clauses = append(clauses, "("+lsClause+")")
			args = append(args, lsArgs...)
			paramNum = newParamNum
		}
	}

	// 4. Combine with AND
	if len(clauses) == 0 {
		return "", nil, nil
	}

	whereClause = strings.Join(clauses, " AND ")
	return whereClause, args, nil
}

// buildIdentifiersWHERE генерирует WHERE clause для identifiers (OR logic)
func (b *SQLBuilder) buildIdentifiersWHERE(
	tableAlias string,
	identifiers []models.ResourceIdentifier,
	startParamNum int,
) (string, []interface{}, error) {

	if len(identifiers) == 0 {
		return "", nil, nil
	}

	var conditions []string
	var args []interface{}
	paramNum := startParamNum

	for _, id := range identifiers {
		if id.Name == "" {
			// Namespace-only filter
			condition := fmt.Sprintf("%s.namespace = $%d", tableAlias, paramNum)
			conditions = append(conditions, condition)
			args = append(args, id.Namespace)
			paramNum++
		} else {
			// Namespace + Name filter
			condition := fmt.Sprintf(
				"(%s.namespace = $%d AND %s.name = $%d)",
				tableAlias, paramNum,
				tableAlias, paramNum+1,
			)
			conditions = append(conditions, condition)
			args = append(args, id.Namespace, id.Name)
			paramNum += 2
		}
	}

	// OR logic between identifiers
	whereClause := strings.Join(conditions, " OR ")
	return whereClause, args, nil
}

// buildFieldSelectorsWHERE генерирует WHERE clause для field-selectors (AND logic)
func (b *SQLBuilder) buildFieldSelectorsWHERE(
	table string,
	tableAlias string,
	selectors []*netguardpb.FieldSelector,
	startParamNum int,
) (string, []interface{}, int, error) {

	if len(selectors) == 0 {
		return "", nil, startParamNum, nil
	}

	var conditions []string
	var args []interface{}
	paramNum := startParamNum

	for _, sel := range selectors {
		// Получить маппинг для поля
		mapping, ok := GetFieldMapping(table, sel.Field)
		if !ok {
			return "", nil, paramNum, fmt.Errorf(
				"field %q is not supported for table %s. Supported fields: %v",
				sel.Field,
				table,
				GetSupportedFields(table),
			)
		}

		// Генерация SQL условия
		var condition string

		switch mapping.Type {
		case FieldTypeColumn:
			condition, args, paramNum = b.buildConditionForOperator(
				tableAlias,
				mapping.ColumnName,
				sel.Operator,
				sel.Value,
				args,
				paramNum,
			)

		default:
			return "", nil, paramNum, fmt.Errorf("unknown field type: %s", mapping.Type)
		}

		conditions = append(conditions, condition)
	}

	// AND logic between field selectors
	whereClause := strings.Join(conditions, " AND ")
	return whereClause, args, paramNum, nil
}

// buildConditionForOperator генерирует SQL условие для конкретного оператора
func (b *SQLBuilder) buildConditionForOperator(
	tableAlias string,
	columnName string,
	operator netguardpb.FieldOperator,
	value string,
	args []interface{},
	paramNum int,
) (condition string, newArgs []interface{}, newParamNum int) {

	newArgs = args
	newParamNum = paramNum

	switch operator {
	case netguardpb.FieldOperator_FIELD_OPERATOR_EQUALS:
		// Simple: field = $1
		condition = fmt.Sprintf("%s.%s = $%d", tableAlias, columnName, paramNum)
		newArgs = append(newArgs, value)
		newParamNum++

	case netguardpb.FieldOperator_FIELD_OPERATOR_NOT_EQUALS:
		// Simple: field != $1
		condition = fmt.Sprintf("%s.%s != $%d", tableAlias, columnName, paramNum)
		newArgs = append(newArgs, value)
		newParamNum++

	case netguardpb.FieldOperator_FIELD_OPERATOR_IN:
		// IN: field IN ($1, $2, $3)
		// Parse comma-separated values
		values := strings.Split(value, ",")
		placeholders := make([]string, len(values))
		for i, v := range values {
			placeholders[i] = fmt.Sprintf("$%d", paramNum)
			newArgs = append(newArgs, strings.TrimSpace(v))
			paramNum++
		}
		condition = fmt.Sprintf("%s.%s IN (%s)", tableAlias, columnName, strings.Join(placeholders, ", "))
		newParamNum = paramNum

	case netguardpb.FieldOperator_FIELD_OPERATOR_NOT_IN:
		// NOT IN: field NOT IN ($1, $2, $3)
		values := strings.Split(value, ",")
		placeholders := make([]string, len(values))
		for i, v := range values {
			placeholders[i] = fmt.Sprintf("$%d", paramNum)
			newArgs = append(newArgs, strings.TrimSpace(v))
			paramNum++
		}
		condition = fmt.Sprintf("%s.%s NOT IN (%s)", tableAlias, columnName, strings.Join(placeholders, ", "))
		newParamNum = paramNum

	case netguardpb.FieldOperator_FIELD_OPERATOR_CONTAINS:
		// CONTAINS: field LIKE $1 (where value = %value%)
		condition = fmt.Sprintf("%s.%s LIKE $%d", tableAlias, columnName, paramNum)
		newArgs = append(newArgs, "%"+value+"%")
		newParamNum++

	case netguardpb.FieldOperator_FIELD_OPERATOR_STARTS_WITH:
		// STARTS_WITH: field LIKE $1 (where value = value%)
		condition = fmt.Sprintf("%s.%s LIKE $%d", tableAlias, columnName, paramNum)
		newArgs = append(newArgs, value+"%")
		newParamNum++

	case netguardpb.FieldOperator_FIELD_OPERATOR_ENDS_WITH:
		// ENDS_WITH: field LIKE $1 (where value = %value)
		condition = fmt.Sprintf("%s.%s LIKE $%d", tableAlias, columnName, paramNum)
		newArgs = append(newArgs, "%"+value)
		newParamNum++

	case netguardpb.FieldOperator_FIELD_OPERATOR_EXISTS:
		// EXISTS: field IS NOT NULL (no value needed)
		condition = fmt.Sprintf("%s.%s IS NOT NULL", tableAlias, columnName)
		// No args, no paramNum increment

	case netguardpb.FieldOperator_FIELD_OPERATOR_NOT_EXISTS:
		// NOT_EXISTS: field IS NULL (no value needed)
		condition = fmt.Sprintf("%s.%s IS NULL", tableAlias, columnName)
		// No args, no paramNum increment

	default:
		// Fallback to EQUALS
		condition = fmt.Sprintf("%s.%s = $%d", tableAlias, columnName, paramNum)
		newArgs = append(newArgs, value)
		newParamNum++
	}

	return condition, newArgs, newParamNum
}

// buildLabelSelectorsWHERE генерирует WHERE clause для label-selectors (AND logic)
// Использует JSONB операторы PostgreSQL для фильтрации по labels
func (b *SQLBuilder) buildLabelSelectorsWHERE(
	tableAlias string,
	selectors []*netguardpb.LabelSelector,
	startParamNum int,
) (string, []interface{}, int, error) {

	if len(selectors) == 0 {
		return "", nil, startParamNum, nil
	}

	var conditions []string
	var args []interface{}
	paramNum := startParamNum

	// labels хранятся в k8s_metadata, доступ через JOIN
	// Предполагаем что tableAlias связан с k8s_metadata через resource_version
	// В запросе должен быть: INNER JOIN k8s_metadata m ON {tableAlias}.resource_version = m.resource_version
	labelsColumn := "m.labels"

	for _, sel := range selectors {
		var condition string

		switch sel.Operator {
		case netguardpb.LabelOperator_LABEL_OPERATOR_EQUALS:
			// labels->>'key' = 'value'
			if len(sel.Values) == 0 {
				return "", nil, paramNum, fmt.Errorf("EQUALS operator requires at least one value")
			}
			condition = fmt.Sprintf("%s->>$%d = $%d", labelsColumn, paramNum, paramNum+1)
			args = append(args, sel.Key, sel.Values[0])
			paramNum += 2

		case netguardpb.LabelOperator_LABEL_OPERATOR_NOT_EQUALS:
			// labels->>'key' != 'value' OR labels->>'key' IS NULL
			if len(sel.Values) == 0 {
				return "", nil, paramNum, fmt.Errorf("NOT_EQUALS operator requires at least one value")
			}
			condition = fmt.Sprintf("(%s->>$%d != $%d OR %s->>$%d IS NULL)",
				labelsColumn, paramNum, paramNum+1,
				labelsColumn, paramNum)
			args = append(args, sel.Key, sel.Values[0])
			paramNum += 2

		case netguardpb.LabelOperator_LABEL_OPERATOR_IN:
			// labels->>'key' IN ('value1', 'value2', ...)
			if len(sel.Values) == 0 {
				return "", nil, paramNum, fmt.Errorf("IN operator requires at least one value")
			}
			placeholders := make([]string, len(sel.Values))
			args = append(args, sel.Key)
			paramNum++
			for i, val := range sel.Values {
				placeholders[i] = fmt.Sprintf("$%d", paramNum)
				args = append(args, val)
				paramNum++
			}
			condition = fmt.Sprintf("%s->>$%d IN (%s)",
				labelsColumn, paramNum-len(sel.Values)-1, strings.Join(placeholders, ", "))

		case netguardpb.LabelOperator_LABEL_OPERATOR_NOT_IN:
			// labels->>'key' NOT IN ('value1', 'value2', ...) OR labels->>'key' IS NULL
			if len(sel.Values) == 0 {
				return "", nil, paramNum, fmt.Errorf("NOT_IN operator requires at least one value")
			}
			placeholders := make([]string, len(sel.Values))
			keyParamNum := paramNum
			args = append(args, sel.Key)
			paramNum++
			for i, val := range sel.Values {
				placeholders[i] = fmt.Sprintf("$%d", paramNum)
				args = append(args, val)
				paramNum++
			}
			condition = fmt.Sprintf("(%s->>$%d NOT IN (%s) OR %s->>$%d IS NULL)",
				labelsColumn, keyParamNum, strings.Join(placeholders, ", "),
				labelsColumn, keyParamNum)

		case netguardpb.LabelOperator_LABEL_OPERATOR_EXISTS:
			// labels ? 'key' (JSONB key exists operator)
			condition = fmt.Sprintf("%s ? $%d", labelsColumn, paramNum)
			args = append(args, sel.Key)
			paramNum++

		case netguardpb.LabelOperator_LABEL_OPERATOR_NOT_EXISTS:
			// NOT (labels ? 'key')
			condition = fmt.Sprintf("NOT (%s ? $%d)", labelsColumn, paramNum)
			args = append(args, sel.Key)
			paramNum++

		default:
			return "", nil, paramNum, fmt.Errorf("unsupported label operator: %v", sel.Operator)
		}

		conditions = append(conditions, condition)
	}

	// AND logic between label selectors
	whereClause := strings.Join(conditions, " AND ")
	return whereClause, args, paramNum, nil
}

// operatorToSQL конвертирует protobuf оператор в SQL
func operatorToSQL(op netguardpb.FieldOperator) string {
	switch op {
	case netguardpb.FieldOperator_FIELD_OPERATOR_EQUALS:
		return "="
	case netguardpb.FieldOperator_FIELD_OPERATOR_NOT_EQUALS:
		return "!="
	case netguardpb.FieldOperator_FIELD_OPERATOR_IN:
		return "IN"
	case netguardpb.FieldOperator_FIELD_OPERATOR_NOT_IN:
		return "NOT IN"
	case netguardpb.FieldOperator_FIELD_OPERATOR_CONTAINS:
		return "LIKE"
	case netguardpb.FieldOperator_FIELD_OPERATOR_STARTS_WITH:
		return "LIKE"
	case netguardpb.FieldOperator_FIELD_OPERATOR_ENDS_WITH:
		return "LIKE"
	case netguardpb.FieldOperator_FIELD_OPERATOR_EXISTS:
		return "IS NOT NULL"
	case netguardpb.FieldOperator_FIELD_OPERATOR_NOT_EXISTS:
		return "IS NULL"
	default:
		return "=" // default fallback
	}
}

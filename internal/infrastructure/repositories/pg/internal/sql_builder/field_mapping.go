package sql_builder

// FieldType определяет тип поля в базе данных
type FieldType string

const (
	FieldTypeColumn FieldType = "column" // Обычная колонка таблицы
)

// FieldMapping описывает как поле API mapped в структуру БД
type FieldMapping struct {
	// Тип хранения
	Type FieldType

	// Для FieldTypeColumn
	ColumnName string
}

// fieldMappings содержит маппинг для всех таблиц
var fieldMappings = map[string]map[string]FieldMapping{
	"hosts": {
		"metadata.name": {
			Type:       FieldTypeColumn,
			ColumnName: "name",
		},
		"metadata.namespace": {
			Type:       FieldTypeColumn,
			ColumnName: "namespace",
		},
		// ВАЖНО: В текущей схеме hosts table все ссылки - это КОЛОНКИ, не JSONB!
		"status.addressGroupRef.name": {
			Type:       FieldTypeColumn,
			ColumnName: "address_group_ref_name",
		},
		"status.addressGroupRef.namespace": {
			Type:       FieldTypeColumn,
			ColumnName: "address_group_ref_namespace",
		},
		"status.bindingRef.name": {
			Type:       FieldTypeColumn,
			ColumnName: "binding_ref_name",
		},
		"status.bindingRef.namespace": {
			Type:       FieldTypeColumn,
			ColumnName: "binding_ref_namespace",
		},
		"status.isBound": {
			Type:       FieldTypeColumn,
			ColumnName: "is_bound",
		},
	},

	"address_groups": {
		"metadata.name": {
			Type:       FieldTypeColumn,
			ColumnName: "name",
		},
		"metadata.namespace": {
			Type:       FieldTypeColumn,
			ColumnName: "namespace",
		},
		"spec.defaultAction": {
			Type:       FieldTypeColumn,
			ColumnName: "default_action",
		},
		"spec.logs": {
			Type:       FieldTypeColumn,
			ColumnName: "logs",
		},
		"spec.trace": {
			Type:       FieldTypeColumn,
			ColumnName: "trace",
		},
	},

	"services": {
		"metadata.name": {
			Type:       FieldTypeColumn,
			ColumnName: "name",
		},
		"metadata.namespace": {
			Type:       FieldTypeColumn,
			ColumnName: "namespace",
		},
		"spec.description": {
			Type:       FieldTypeColumn,
			ColumnName: "description",
		},
	},

	"networks": {
		"metadata.name": {
			Type:       FieldTypeColumn,
			ColumnName: "name",
		},
		"metadata.namespace": {
			Type:       FieldTypeColumn,
			ColumnName: "namespace",
		},
		"status.isBound": {
			Type:       FieldTypeColumn,
			ColumnName: "is_bound",
		},
		"status.bindingRef.name": {
			Type:       FieldTypeColumn,
			ColumnName: "binding_ref_name",
		},
		"status.bindingRef.namespace": {
			Type:       FieldTypeColumn,
			ColumnName: "binding_ref_namespace",
		},
		"status.addressGroupRef.name": {
			Type:       FieldTypeColumn,
			ColumnName: "address_group_ref_name",
		},
		"status.addressGroupRef.namespace": {
			Type:       FieldTypeColumn,
			ColumnName: "address_group_ref_namespace",
		},
	},
}

// GetFieldMapping возвращает маппинг для таблицы и поля
func GetFieldMapping(table, field string) (FieldMapping, bool) {
	tableMap, ok := fieldMappings[table]
	if !ok {
		return FieldMapping{}, false
	}
	mapping, ok := tableMap[field]
	return mapping, ok
}

// GetSupportedFields возвращает список поддерживаемых полей для таблицы
func GetSupportedFields(table string) []string {
	tableMap, ok := fieldMappings[table]
	if !ok {
		return nil
	}
	fields := make([]string, 0, len(tableMap))
	for field := range tableMap {
		fields = append(fields, field)
	}
	return fields
}

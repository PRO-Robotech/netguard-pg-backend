package utils

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"netguard-pg-backend/internal/domain/models"

	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/endpoints/request"
)

// SortByNamespaceName sorts a slice in-place by Namespace then Name.
// idFn extracts ResourceIdentifier from the slice element.
// It works for any slice element type via a generic parameter.
func SortByNamespaceName[T any](items []T, idFn func(T) models.ResourceIdentifier) {
	sort.Slice(items, func(i, j int) bool {
		idI := idFn(items[i])
		idJ := idFn(items[j])
		if idI.Namespace != idJ.Namespace {
			return idI.Namespace < idJ.Namespace
		}
		return idI.Name < idJ.Name
	})
}

// SortOptions определяет параметры сортировки
type SortOptions struct {
	SortBy    string // JSONPath выражение для сортировки
	Ascending bool   // направление сортировки
}

// ApplySorting применяет сортировку к списку объектов
// Если sortBy не указан, использует сортировку по умолчанию (namespace + name)
func ApplySorting[T any](items []T, sortBy string, idFn func(T) models.ResourceIdentifier, k8sObjectFn func(T) runtime.Object) error {
	if sortBy == "" {
		// По умолчанию сортируем по namespace + name
		SortByNamespaceName(items, idFn)
		return nil
	}

	// Применяем кастомную сортировку
	return SortByField(items, sortBy, k8sObjectFn)
}

// SortByField sorts a slice in-place by the specified field path.
// Поддерживает простые JSONPath выражения типа .metadata.name, .spec.cidr
func SortByField[T any](items []T, sortBy string, k8sObjectFn func(T) runtime.Object) error {
	if sortBy == "" {
		return nil
	}

	// Разбор JSONPath (упрощенный для основных случаев)
	path := strings.TrimPrefix(sortBy, ".")
	fieldPath := strings.Split(path, ".")

	sort.Slice(items, func(i, j int) bool {
		objI := k8sObjectFn(items[i])
		objJ := k8sObjectFn(items[j])

		valueI := extractFieldValue(objI, fieldPath)
		valueJ := extractFieldValue(objJ, fieldPath)

		return compareValues(valueI, valueJ)
	})

	return nil
}

// ExtractSortByFromListOptions извлекает параметр sortBy из ListOptions
func ExtractSortByFromListOptions(options *metainternalversion.ListOptions) string {
	if options == nil {
		return ""
	}

	return ""
}

// ExtractSortByFromContext извлекает параметр sortBy из контекста запроса
func ExtractSortByFromContext(ctx context.Context) string {
	// Пытаемся извлечь из специального контекстного ключа (если был установлен ранее)
	if sortBy, ok := ctx.Value("sortBy").(string); ok && sortBy != "" {
		return sortBy
	}

	// Извлекаем RequestInfo из контекста
	if reqInfo, ok := request.RequestInfoFrom(ctx); ok {

		_ = reqInfo

	}

	// Возвращаем пустую строку = сортировка по умолчанию (namespace + name)
	return ""
}

// SetSortByInContext устанавливает параметр sortBy в контекст (для тестирования)
func SetSortByInContext(ctx context.Context, sortBy string) context.Context {
	return context.WithValue(ctx, "sortBy", sortBy)
}

// ParseSortByFromQueryParams извлекает sortBy из query параметров (helper функция)
// Может использоваться в middleware или custom handlers
func ParseSortByFromQueryParams(queryParams map[string][]string) string {
	if sortBy, exists := queryParams["sortBy"]; exists && len(sortBy) > 0 {
		return sortBy[0]
	}
	// Альтернативные названия параметра
	if sortBy, exists := queryParams["sort-by"]; exists && len(sortBy) > 0 {
		return sortBy[0]
	}
	if sortBy, exists := queryParams["orderBy"]; exists && len(sortBy) > 0 {
		return sortBy[0]
	}
	return ""
}

// extractFieldValue извлекает значение поля по JSONPath
func extractFieldValue(obj runtime.Object, fieldPath []string) interface{} {
	if obj == nil || len(fieldPath) == 0 {
		return ""
	}

	// Быстрая обработка стандартных путей
	switch fieldPath[0] {
	case "metadata":
		if metaObj, ok := obj.(metav1.Object); ok {
			return extractMetadataField(metaObj, fieldPath[1:])
		}
	case "spec":
		return extractSpecField(obj, fieldPath[1:])
	case "status":
		return extractStatusField(obj, fieldPath[1:])
	}

	// Fallback через reflection для сложных случаев
	return extractViaReflection(obj, fieldPath)
}

// extractMetadataField извлекает поля из metadata
func extractMetadataField(obj metav1.Object, fieldPath []string) interface{} {
	if len(fieldPath) == 0 {
		return ""
	}

	switch fieldPath[0] {
	case "name":
		return obj.GetName()
	case "namespace":
		return obj.GetNamespace()
	case "creationTimestamp":
		return obj.GetCreationTimestamp().Time
	case "labels":
		if len(fieldPath) > 1 && obj.GetLabels() != nil {
			return obj.GetLabels()[fieldPath[1]]
		}
		return obj.GetLabels()
	case "annotations":
		if len(fieldPath) > 1 && obj.GetAnnotations() != nil {
			return obj.GetAnnotations()[fieldPath[1]]
		}
		return obj.GetAnnotations()
	}
	return ""
}

// extractSpecField и extractStatusField - заглушки для будущего расширения
func extractSpecField(obj runtime.Object, fieldPath []string) interface{} {
	// Обработка популярных полей spec для типичных ресурсов
	return extractViaReflection(obj, append([]string{"Spec"}, capitalizeFields(fieldPath)...))
}

func extractStatusField(obj runtime.Object, fieldPath []string) interface{} {
	// Обработка популярных полей status для типичных ресурсов
	return extractViaReflection(obj, append([]string{"Status"}, capitalizeFields(fieldPath)...))
}

// capitalizeFields капитализирует первую букву каждого поля для reflection
func capitalizeFields(fields []string) []string {
	result := make([]string, len(fields))
	for i, field := range fields {
		if len(field) > 0 {
			result[i] = strings.ToUpper(field[:1]) + field[1:]
		}
	}
	return result
}

// extractViaReflection fallback через reflection
func extractViaReflection(obj runtime.Object, fieldPath []string) interface{} {
	val := reflect.ValueOf(obj)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}

	for _, field := range fieldPath {
		if val.Kind() != reflect.Struct {
			return ""
		}

		fieldVal := val.FieldByName(field)
		if !fieldVal.IsValid() {
			// Попробуем найти поле без учета регистра
			fieldVal = findFieldCaseInsensitive(val, field)
			if !fieldVal.IsValid() {
				return ""
			}
		}
		val = fieldVal
	}

	if val.CanInterface() {
		return val.Interface()
	}
	return ""
}

// findFieldCaseInsensitive ищет поле без учета регистра
func findFieldCaseInsensitive(val reflect.Value, fieldName string) reflect.Value {
	typ := val.Type()
	for i := 0; i < val.NumField(); i++ {
		if strings.EqualFold(typ.Field(i).Name, fieldName) {
			return val.Field(i)
		}
	}
	return reflect.Value{}
}

// compareValues сравнивает два значения для сортировки
func compareValues(a, b interface{}) bool {
	// Специальная обработка для типов времени
	if timeA, ok := a.(time.Time); ok {
		if timeB, ok := b.(time.Time); ok {
			return timeA.Before(timeB)
		}
	}

	if timeA, ok := a.(metav1.Time); ok {
		if timeB, ok := b.(metav1.Time); ok {
			return timeA.Before(&timeB)
		}
	}

	// Числовые типы
	if valA, valB, ok := compareAsNumbers(a, b); ok {
		return valA < valB
	}

	// Простое лексикографическое сравнение для остальных типов
	strA := strings.ToLower(fmt.Sprintf("%v", a))
	strB := strings.ToLower(fmt.Sprintf("%v", b))
	return strA < strB
}

// compareAsNumbers пытается сравнить как числа
func compareAsNumbers(a, b interface{}) (float64, float64, bool) {
	var valA, valB float64
	var okA, okB bool

	switch v := a.(type) {
	case int:
		valA, okA = float64(v), true
	case int32:
		valA, okA = float64(v), true
	case int64:
		valA, okA = float64(v), true
	case float32:
		valA, okA = float64(v), true
	case float64:
		valA, okA = v, true
	}

	switch v := b.(type) {
	case int:
		valB, okB = float64(v), true
	case int32:
		valB, okB = float64(v), true
	case int64:
		valB, okB = float64(v), true
	case float32:
		valB, okB = float64(v), true
	case float64:
		valB, okB = v, true
	}

	return valA, valB, okA && okB
}

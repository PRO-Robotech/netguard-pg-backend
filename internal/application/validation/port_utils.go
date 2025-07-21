package validation

import (
	"fmt"
	"strconv"
	"strings"

	"netguard-pg-backend/internal/domain/models"

	"k8s.io/klog/v2"
)

// ParsePortRanges преобразует строковое представление портов в слайс PortRange
// Поддерживает одиночные порты, диапазоны портов и списки портов через запятую
func ParsePortRanges(port string) ([]models.PortRange, error) {
	klog.Infof("🔧 ParsePortRanges: parsing port string '%s'", port)

	if port == "" {
		return nil, fmt.Errorf("port cannot be empty")
	}

	var result []models.PortRange

	// Разбиваем по запятой для обработки списка портов/диапазонов
	portItems := strings.Split(port, ",")
	klog.Infof("🔧 Split port string into %d items", len(portItems))

	for i, item := range portItems {
		item = strings.TrimSpace(item)
		klog.Infof("🔧 Processing item %d: '%s'", i, item)

		// Пропускаем пустые элементы
		if item == "" {
			klog.Infof("🔧 Skipping empty item %d", i)
			continue
		}

		// Проверяем, является ли это диапазоном портов (формат: "start-end")
		if strings.Contains(item, "-") && !strings.HasPrefix(item, "-") {
			klog.Infof("🔧 Item %d is a port range", i)
			parts := strings.Split(item, "-")
			if len(parts) != 2 {
				return nil, fmt.Errorf("invalid port range format '%s', expected format is 'start-end'", item)
			}

			start, err := strconv.Atoi(parts[0])
			if err != nil {
				return nil, fmt.Errorf("invalid start port '%s': must be a number between 0 and 65535", parts[0])
			}

			end, err := strconv.Atoi(parts[1])
			if err != nil {
				return nil, fmt.Errorf("invalid end port '%s': must be a number between 0 and 65535", parts[1])
			}

			if start < 0 || start > 65535 {
				return nil, fmt.Errorf("start port %d is out of valid range (0-65535)", start)
			}

			if end < 0 || end > 65535 {
				return nil, fmt.Errorf("end port %d is out of valid range (0-65535)", end)
			}

			if start > end {
				return nil, fmt.Errorf("start port %d cannot be greater than end port %d", start, end)
			}

			result = append(result, models.PortRange{Start: start, End: end})
			klog.Infof("🔧 Added port range %d-%d", start, end)
		} else {
			// Одиночный порт
			klog.Infof("🔧 Item %d is a single port", i)
			p, err := strconv.Atoi(item)
			if err != nil {
				return nil, fmt.Errorf("invalid port '%s': must be a number between 0 and 65535", item)
			}

			if p < 0 || p > 65535 {
				return nil, fmt.Errorf("port %d is out of valid range (0-65535)", p)
			}

			result = append(result, models.PortRange{Start: p, End: p})
			klog.Infof("🔧 Added single port %d", p)
		}
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("no valid ports found in list '%s'", port)
	}

	klog.Infof("🔧 ParsePortRanges: successfully parsed %d port ranges from '%s'", len(result), port)
	return result, nil
}

// ParsePortRange преобразует строковое представление порта в PortRange
// Для обратной совместимости возвращает только первый порт из списка
func ParsePortRange(port string) (models.PortRange, error) {
	ranges, err := ParsePortRanges(port)
	if err != nil {
		return models.PortRange{}, err
	}

	// Возвращаем первый порт из списка
	return ranges[0], nil
}

// DoPortRangesOverlap проверяет, перекрываются ли два диапазона портов
func DoPortRangesOverlap(a, b models.PortRange) bool {
	// Диапазоны перекрываются, если начало одного меньше или равно концу другого
	// и конец одного больше или равен началу другого
	return a.Start <= b.End && a.End >= b.Start
}

// validatePort проверяет корректность строкового представления порта
func validatePort(port string) error {
	// Разрешаем пустую строку порта
	if port == "" {
		return nil
	}

	// Разбиваем по запятой для обработки списка портов/диапазонов
	portItems := strings.Split(port, ",")
	for _, item := range portItems {
		item = strings.TrimSpace(item)

		// Проверяем, является ли это диапазоном портов (формат: "start-end")
		if strings.Contains(item, "-") && !strings.HasPrefix(item, "-") {
			parts := strings.Split(item, "-")
			if len(parts) != 2 {
				return fmt.Errorf("invalid port range format")
			}

			start, err := strconv.Atoi(parts[0])
			if err != nil {
				return fmt.Errorf("invalid start port")
			}

			end, err := strconv.Atoi(parts[1])
			if err != nil {
				return fmt.Errorf("invalid end port")
			}

			if start < 0 || start > 65535 {
				return fmt.Errorf("start port must be between 0 and 65535")
			}

			if end < 0 || end > 65535 {
				return fmt.Errorf("end port must be between 0 and 65535")
			}

			if start > end {
				return fmt.Errorf("start port must be less than or equal to end port")
			}
		} else {
			// Проверяем, является ли это одиночным портом
			p, err := strconv.Atoi(item)
			if err != nil {
				return fmt.Errorf("invalid port")
			}

			if p < 0 || p > 65535 {
				return fmt.Errorf("port must be between 0 and 65535")
			}
		}
	}

	return nil
}

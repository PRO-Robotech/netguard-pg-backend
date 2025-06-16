package validation

import (
	"fmt"
	"strconv"
	"strings"

	"netguard-pg-backend/internal/domain/models"
)

// ParsePortRange преобразует строковое представление порта в PortRange
func ParsePortRange(port string) (models.PortRange, error) {
	if port == "" {
		return models.PortRange{}, fmt.Errorf("port cannot be empty")
	}

	// Проверяем, является ли это диапазоном портов (формат: "start-end")
	if strings.Contains(port, "-") && !strings.HasPrefix(port, "-") {
		parts := strings.Split(port, "-")
		if len(parts) != 2 {
			return models.PortRange{}, fmt.Errorf("invalid port range format '%s', expected format is 'start-end'", port)
		}

		start, err := strconv.Atoi(parts[0])
		if err != nil {
			return models.PortRange{}, fmt.Errorf("invalid start port '%s': must be a number between 0 and 65535", parts[0])
		}

		end, err := strconv.Atoi(parts[1])
		if err != nil {
			return models.PortRange{}, fmt.Errorf("invalid end port '%s': must be a number between 0 and 65535", parts[1])
		}

		if start < 0 || start > 65535 {
			return models.PortRange{}, fmt.Errorf("start port %d is out of valid range (0-65535)", start)
		}

		if end < 0 || end > 65535 {
			return models.PortRange{}, fmt.Errorf("end port %d is out of valid range (0-65535)", end)
		}

		if start > end {
			return models.PortRange{}, fmt.Errorf("start port %d cannot be greater than end port %d", start, end)
		}

		return models.PortRange{Start: start, End: end}, nil
	}

	// Одиночный порт
	p, err := strconv.Atoi(port)
	if err != nil {
		return models.PortRange{}, fmt.Errorf("invalid port '%s': must be a number between 0 and 65535", port)
	}

	if p < 0 || p > 65535 {
		return models.PortRange{}, fmt.Errorf("port %d is out of valid range (0-65535)", p)
	}

	return models.PortRange{Start: p, End: p}, nil
}

// DoPortRangesOverlap проверяет, перекрываются ли два диапазона портов
func DoPortRangesOverlap(a, b models.PortRange) bool {
	// Диапазоны перекрываются, если начало одного меньше или равно концу другого
	// и конец одного больше или равен началу другого
	return a.Start <= b.End && a.End >= b.Start
}

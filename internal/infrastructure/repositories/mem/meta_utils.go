package mem

import (
	"fmt"
	"time"

	"netguard-pg-backend/internal/domain/models"
)

// ensureMetaFill guarantees that Meta has UID, CreationTS, Generation and ResourceVersion.
func ensureMetaFill(m *models.Meta) {
	if m == nil {
		return
	}
	if m.UID == "" {
		m.TouchOnCreate()
	}
	if m.ResourceVersion == "" {
		m.ResourceVersion = fmt.Sprintf("%d", time.Now().UnixNano())
	}
	if m.Generation == 0 {
		m.Generation = 1
	}
}

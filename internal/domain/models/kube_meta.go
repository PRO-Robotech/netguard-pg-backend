package models

import (
	"github.com/google/uuid"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Meta stores Kubernetes-specific metadata that must survive round-trip
// through the aggregated API server and backend storage.
// All fields are optional and may be empty when the object is first created
// by a client; the API server or backend will fill them where appropriate.
type Meta struct {
	UID             string            `json:"uid,omitempty"`
	ResourceVersion string            `json:"resourceVersion,omitempty"`
	Generation      int64             `json:"generation,omitempty"`
	CreationTS      metav1.Time       `json:"creationTimestamp,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
	Annotations     map[string]string `json:"annotations,omitempty"`
}

// TouchOnCreate initializes meta fields that are set exactly once during
// object creation.
func (m *Meta) TouchOnCreate() {
	if m == nil {
		return
	}
	if m.CreationTS.IsZero() {
		m.CreationTS = metav1.Now()
	}
	if m.Generation == 0 {
		m.Generation = 1
	}
	if m.UID == "" {
		m.UID = uuid.NewString()
	}
}

// TouchOnWrite updates fields that must change on every write operation.
// uid remains immutable once set.
func (m *Meta) TouchOnWrite(newRV string) {
	if m == nil {
		return
	}
	m.ResourceVersion = newRV
}

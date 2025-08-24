package meta

import (
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"netguard-pg-backend/internal/domain/models"
)

// ToObjectMeta converts models.Meta into a Kubernetes ObjectMeta.
// It does NOT set Name/Namespace – caller must set them.
func ToObjectMeta(m models.Meta) v1.ObjectMeta {
	return v1.ObjectMeta{
		UID:               types.UID(m.UID),
		ResourceVersion:   m.ResourceVersion,
		Generation:        m.Generation,
		CreationTimestamp: m.CreationTS,
		Labels:            m.Labels,
		Annotations:       m.Annotations,
		GenerateName:      m.GeneratedName,
	}
}

// FromObjectMeta copies relevant kubernetes ObjectMeta fields into models.Meta.
func FromObjectMeta(om v1.ObjectMeta) models.Meta {
	return models.Meta{
		UID:             string(om.UID),
		ResourceVersion: om.ResourceVersion,
		Generation:      om.Generation,
		CreationTS:      om.CreationTimestamp,
		Labels:          om.Labels,
		Annotations:     om.Annotations,
		GeneratedName:   om.GenerateName,
	}
}

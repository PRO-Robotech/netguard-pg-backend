package utils

import (
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// NewTable constructs a metav1.Table with standard TypeMeta and supplied columns.
func NewTable(columns ...metav1.TableColumnDefinition) *metav1.Table {
	return &metav1.Table{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "meta.k8s.io/v1",
			Kind:       "Table",
		},
		ColumnDefinitions: columns,
	}
}

// AppendRow appends a row backed by the given object and cells.
func AppendRow(table *metav1.Table, obj runtime.Object, cells ...interface{}) {
	table.Rows = append(table.Rows, metav1.TableRow{
		Object: runtime.RawExtension{Object: obj},
		Cells:  cells,
	})
}

// AppendBookmarkRowIfNeeded adds a synthetic row for bookmark events and returns true if handled.
func AppendBookmarkRowIfNeeded(table *metav1.Table, obj runtime.Object) bool {
	pom, ok := obj.(*metav1.PartialObjectMetadata)
	if !ok {
		return false
	}

	cells := make([]interface{}, len(table.ColumnDefinitions))
	if len(cells) > 0 {
		cells[0] = "BOOKMARK"
	}
	if len(cells) > 1 {
		cells[1] = fmt.Sprintf("rv=%s", pom.GetResourceVersion())
	}

	table.Rows = append(table.Rows, metav1.TableRow{
		Object: runtime.RawExtension{Object: obj},
		Cells:  cells,
	})
	return true
}

// TranslateTimestampSince formats creation timestamps the same way kubectl does.
func TranslateTimestampSince(ts metav1.Time) string {
	if ts.IsZero() {
		return "<unknown>"
	}
	return durationShortHumanDuration(time.Since(ts.Time))
}

func durationShortHumanDuration(d time.Duration) string {
	if seconds := int(d.Seconds()); seconds < 90 {
		return fmt.Sprintf("%ds", seconds)
	}
	if minutes := int(d.Minutes()); minutes < 90 {
		return fmt.Sprintf("%dm", minutes)
	}
	hours := int(d.Round(time.Hour).Hours())
	if hours < 48 {
		return fmt.Sprintf("%dh", hours)
	}
	days := hours / 24
	return fmt.Sprintf("%dd", days)
}

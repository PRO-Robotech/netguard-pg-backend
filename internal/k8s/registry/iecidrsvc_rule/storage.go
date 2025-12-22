package iecidrsvc_rule

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/registry/rest"

	"netguard-pg-backend/internal/domain/models"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/k8s/client"
	"netguard-pg-backend/internal/k8s/registry/base"
	"netguard-pg-backend/internal/k8s/registry/convert"
	tableutils "netguard-pg-backend/internal/k8s/registry/utils"
	"netguard-pg-backend/internal/k8s/registry/validation"
)

// IECidrSvcRuleStorage provides REST storage for IECidrSvcRule resources using BaseStorage helpers.
type IECidrSvcRuleStorage struct {
	*base.BaseStorage[*netguardv1beta1.IECidrSvcRule, *models.IECidrSvcRule]
}

// NewIECidrSvcRuleStorage constructs storage backed by backend gRPC client and shared BaseStorage wiring.
func NewIECidrSvcRuleStorage(backendClient client.BackendClient) *IECidrSvcRuleStorage {
	converter := convert.NewIECidrSvcRuleConverter()
	validator := validation.NewIECidrSvcRuleValidator()
	watcher := watch.NewBroadcaster(1000, watch.DropIfChannelFull)
	backendOps := base.NewIECidrSvcRulePtrOps(backendClient)

	baseStorage := base.NewBaseStorage[*netguardv1beta1.IECidrSvcRule, *models.IECidrSvcRule](
		func() *netguardv1beta1.IECidrSvcRule { return &netguardv1beta1.IECidrSvcRule{} },
		func() runtime.Object { return &netguardv1beta1.IECidrSvcRuleList{} },
		backendOps,
		backendClient,
		converter,
		validator,
		watcher,
		"iecidrsvrules",
		"IECidrSvcRule",
		true,
	)

	return &IECidrSvcRuleStorage{BaseStorage: baseStorage}
}

// GetSingularName returns resource singular string used by kubectl table printers.
func (s *IECidrSvcRuleStorage) GetSingularName() string {
	return "iecidrsvrule"
}

// ConvertToTable renders human friendly table view for kubectl get.
func (s *IECidrSvcRuleStorage) ConvertToTable(ctx context.Context, object runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	table := tableutils.NewTable(
		metav1.TableColumnDefinition{Name: "Name", Type: "string", Format: "name"},
		metav1.TableColumnDefinition{Name: "CIDR", Type: "string"},
		metav1.TableColumnDefinition{Name: "Svc", Type: "string"},
		metav1.TableColumnDefinition{Name: "Traffic", Type: "string"},
		metav1.TableColumnDefinition{Name: "Transport", Type: "string"},
		metav1.TableColumnDefinition{Name: "Action", Type: "string"},
		metav1.TableColumnDefinition{Name: "Priority", Type: "integer"},
		metav1.TableColumnDefinition{Name: "Age", Type: "string"},
	)
	if tableutils.AppendBookmarkRowIfNeeded(table, object) {
		return table, nil
	}

	addRow := func(rule *netguardv1beta1.IECidrSvcRule) {
		svc := fmt.Sprintf("%s/%s", rule.Spec.Svc.Namespace, rule.Spec.Svc.Name)
		tableutils.AppendRow(table, rule,
			rule.Name,
			rule.Spec.CIDR,
			svc,
			string(rule.Spec.Traffic),
			string(rule.Spec.Transport),
			string(rule.Spec.Action),
			rule.Spec.Priority,
			tableutils.TranslateTimestampSince(rule.CreationTimestamp),
		)
	}

	switch v := object.(type) {
	case *netguardv1beta1.IECidrSvcRule:
		addRow(v)
	case *netguardv1beta1.IECidrSvcRuleList:
		for i := range v.Items {
			addRow(&v.Items[i])
		}
	default:
		return nil, fmt.Errorf("unexpected object type %T", object)
	}

	return table, nil
}

// DeleteCollection implements bulk deletion in namespace respecting validation hook.
func (s *IECidrSvcRuleStorage) DeleteCollection(ctx context.Context, deleteValidation rest.ValidateObjectFunc, options *metav1.DeleteOptions, listOptions *internalversion.ListOptions) (runtime.Object, error) {
	obj, err := s.List(ctx, listOptions)
	if err != nil {
		return nil, err
	}

	list, ok := obj.(*netguardv1beta1.IECidrSvcRuleList)
	if !ok {
		return nil, fmt.Errorf("unexpected object type from List: %T", obj)
	}

	deleted := &netguardv1beta1.IECidrSvcRuleList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "IECidrSvcRuleList",
			APIVersion: netguardv1beta1.SchemeGroupVersion.String(),
		},
	}

	for i := range list.Items {
		item := &list.Items[i]
		if deleteValidation != nil {
			if err := deleteValidation(ctx, item); err != nil {
				return nil, err
			}
		}

		if _, _, err := s.Delete(ctx, item.Name, deleteValidation, options); err != nil {
			return nil, fmt.Errorf("failed to delete iecidrsvrule %s: %w", item.Name, err)
		}
		deleted.Items = append(deleted.Items, *item)
	}

	return deleted, nil
}

// Kind returns resource kind for rest.KindProvider implementation.
func (s *IECidrSvcRuleStorage) Kind() string {
	return "IECidrSvcRule"
}

var _ rest.CollectionDeleter = &IECidrSvcRuleStorage{}

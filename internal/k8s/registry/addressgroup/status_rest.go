package addressgroup

import (
	"context"
	"fmt"
	"strconv"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/registry/rest"

	"netguard-pg-backend/internal/domain/models"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
)

// StatusREST implements /status subresource for AddressGroup.
type StatusREST struct{ store *AddressGroupStorage }

func NewStatusREST(s *AddressGroupStorage) *StatusREST { return &StatusREST{store: s} }

func (r *StatusREST) New() runtime.Object { return &netguardv1beta1.AddressGroup{} }

func (r *StatusREST) Get(ctx context.Context, name string, opts *metav1.GetOptions) (runtime.Object, error) {
	return r.store.Get(ctx, name, opts)
}

func (r *StatusREST) Update(ctx context.Context, name string, objInfo rest.UpdatedObjectInfo,
	createValidation rest.ValidateObjectFunc, updateValidation rest.ValidateObjectUpdateFunc,
	forceAllowCreate bool, opts *metav1.UpdateOptions) (runtime.Object, bool, error) {

	curObj, err := r.store.Get(ctx, name, &metav1.GetOptions{})
	if err != nil {
		return nil, false, err
	}

	newObj, err := objInfo.UpdatedObject(ctx, curObj)
	if err != nil {
		return nil, false, err
	}

	cur := curObj.(*netguardv1beta1.AddressGroup)
	cur.Status = newObj.(*netguardv1beta1.AddressGroup).Status

	if updateValidation != nil {
		if err := updateValidation(ctx, newObj, curObj); err != nil {
			return nil, false, err
		}
	}

	model := convertAddressGroupFromK8s(cur)
	model.Meta.TouchOnWrite(strconv.FormatInt(time.Now().UnixNano(), 10))

	if err := r.store.backendClient.Sync(ctx, models.SyncOpUpsert, []models.AddressGroup{model}); err != nil {
		return nil, false, apierrors.NewInternalError(fmt.Errorf("persist status: %w", err))
	}
	cur.ResourceVersion = model.Meta.ResourceVersion
	return cur, false, nil
}

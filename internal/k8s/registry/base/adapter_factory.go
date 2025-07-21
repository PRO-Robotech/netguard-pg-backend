package base

import (
	"context"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/k8s/client"
)

// PtrBackendOperations implements BackendOperations for pointer types
type PtrBackendOperations[T any] struct {
	baseOps BackendOperations[T]
}

func NewPtrBackendOperations[T any](baseOps BackendOperations[T]) BackendOperations[*T] {
	return &PtrBackendOperations[T]{
		baseOps: baseOps,
	}
}

func (p *PtrBackendOperations[T]) Get(ctx context.Context, id models.ResourceIdentifier) (**T, error) {
	result, err := p.baseOps.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (p *PtrBackendOperations[T]) List(ctx context.Context, scope ports.Scope) ([]*T, error) {
	results, err := p.baseOps.List(ctx, scope)
	if err != nil {
		return nil, err
	}
	ptrs := make([]*T, len(results))
	for i := range results {
		ptrs[i] = &results[i]
	}
	return ptrs, nil
}

func (p *PtrBackendOperations[T]) Create(ctx context.Context, obj **T) error {
	return p.baseOps.Create(ctx, *obj)
}

func (p *PtrBackendOperations[T]) Update(ctx context.Context, obj **T) error {
	return p.baseOps.Update(ctx, *obj)
}

func (p *PtrBackendOperations[T]) Delete(ctx context.Context, id models.ResourceIdentifier) error {
	return p.baseOps.Delete(ctx, id)
}

// Factory functions for all resource types

func NewAddressGroupPtrOps(client client.BackendClient) BackendOperations[*models.AddressGroup] {
	return NewPtrBackendOperations(NewAddressGroupBackendOperations(client))
}

func NewAddressGroupBindingPtrOps(client client.BackendClient) BackendOperations[*models.AddressGroupBinding] {
	return NewPtrBackendOperations(NewAddressGroupBindingBackendOperations(client))
}

func NewAddressGroupBindingPolicyPtrOps(client client.BackendClient) BackendOperations[*models.AddressGroupBindingPolicy] {
	return NewPtrBackendOperations(NewAddressGroupBindingPolicyBackendOperations(client))
}

func NewAddressGroupPortMappingPtrOps(client client.BackendClient) BackendOperations[*models.AddressGroupPortMapping] {
	return NewPtrBackendOperations(NewAddressGroupPortMappingBackendOperations(client))
}

func NewRuleS2SPtrOps(client client.BackendClient) BackendOperations[*models.RuleS2S] {
	return NewPtrBackendOperations(NewRuleS2SBackendOperations(client))
}

func NewServiceAliasPtrOps(client client.BackendClient) BackendOperations[*models.ServiceAlias] {
	return NewPtrBackendOperations(NewServiceAliasBackendOperations(client))
}

func NewIEAgAgRulePtrOps(client client.BackendClient) BackendOperations[*models.IEAgAgRule] {
	return NewPtrBackendOperations(NewIEAgAgRuleBackendOperations(client))
}

func NewServicePtrOps(client client.BackendClient) BackendOperations[*models.Service] {
	return NewPtrBackendOperations(NewServiceBackendOperations(client))
}

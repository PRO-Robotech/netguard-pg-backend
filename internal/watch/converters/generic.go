package converters

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"

	"netguard-pg-backend/internal/domain/models"
)

// ResourceConverter converts domain resources into Kubernetes runtime objects.
type ResourceConverter interface {
	ResourceType() string
	Convert(ctx context.Context, id models.ResourceIdentifier) (runtime.Object, error)
	List(ctx context.Context) ([]runtime.Object, error)
}

type domainConverter[D any] struct {
	resourceType string
	fetch        func(ctx context.Context, id models.ResourceIdentifier) (*D, error)
	list         func(ctx context.Context) ([]D, error)
	toRuntime    func(ctx context.Context, domainObj *D) (runtime.Object, error)
}

func newDomainConverter[D any](
	resourceType string,
	fetch func(ctx context.Context, id models.ResourceIdentifier) (*D, error),
	list func(ctx context.Context) ([]D, error),
	toRuntime func(ctx context.Context, domainObj *D) (runtime.Object, error),
) ResourceConverter {
	return &domainConverter[D]{
		resourceType: resourceType,
		fetch:        fetch,
		list:         list,
		toRuntime:    toRuntime,
	}
}

func (c *domainConverter[D]) ResourceType() string {
	return c.resourceType
}

func (c *domainConverter[D]) Convert(ctx context.Context, id models.ResourceIdentifier) (runtime.Object, error) {
	if c.fetch == nil || c.toRuntime == nil {
		return nil, fmt.Errorf("converter for %s is not fully configured", c.resourceType)
	}

	domainObj, err := c.fetch(ctx, id)
	if err != nil {
		return nil, err
	}
	if domainObj == nil {
		return nil, fmt.Errorf("%s %s/%s not found", c.resourceType, id.Namespace, id.Name)
	}

	return c.toRuntime(ctx, domainObj)
}

func (c *domainConverter[D]) List(ctx context.Context) ([]runtime.Object, error) {
	if c.list == nil {
		return nil, fmt.Errorf("list not supported for resource type %s", c.resourceType)
	}

	domainObjs, err := c.list(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]runtime.Object, 0, len(domainObjs))
	for i := range domainObjs {
		obj := domainObjs[i]
		objCopy := obj
		k8sObj, err := c.toRuntime(ctx, &objCopy)
		if err != nil {
			return nil, err
		}
		result = append(result, k8sObj)
	}

	return result, nil
}


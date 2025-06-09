package ports

import (
	"context"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/patterns"
)

type (
	// Scope defines the scope of operations
	Scope interface {
		IsEmpty() bool
		String() string
	}

	// Option defines options for operations
	Option interface{}

	// ReaderNoClose defines read operations without close
	ReaderNoClose interface {
		ListServices(ctx context.Context, consume func(models.Service) error, scope Scope) error
		ListAddressGroups(ctx context.Context, consume func(models.AddressGroup) error, scope Scope) error
		ListAddressGroupBindings(ctx context.Context, consume func(models.AddressGroupBinding) error, scope Scope) error
		ListAddressGroupPortMappings(ctx context.Context, consume func(models.AddressGroupPortMapping) error, scope Scope) error
		ListRuleS2S(ctx context.Context, consume func(models.RuleS2S) error, scope Scope) error
		GetSyncStatus(ctx context.Context) (*models.SyncStatus, error)
	}

	// Reader defines read operations
	Reader interface {
		ReaderNoClose
		Close() error
	}

	// Writer defines write operations
	Writer interface {
		SyncServices(ctx context.Context, services []models.Service, scope Scope, opts ...Option) error
		SyncAddressGroups(ctx context.Context, addressGroups []models.AddressGroup, scope Scope, opts ...Option) error
		SyncAddressGroupBindings(ctx context.Context, bindings []models.AddressGroupBinding, scope Scope, opts ...Option) error
		SyncAddressGroupPortMappings(ctx context.Context, mappings []models.AddressGroupPortMapping, scope Scope, opts ...Option) error
		SyncRuleS2S(ctx context.Context, rules []models.RuleS2S, scope Scope, opts ...Option) error
		Commit() error
		Abort()
	}

	// Registry defines the registry interface
	Registry interface {
		Subject() patterns.Subject
		Writer(ctx context.Context) (Writer, error)
		Reader(ctx context.Context) (Reader, error)
		Close() error
	}
)

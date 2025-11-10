package integration

import (
	"context"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/patterns"
)

type MockRegistry struct{}

func NewMockRegistry() ports.Registry {
	return &MockRegistry{}
}
func (m *MockRegistry) Subject() patterns.Subject {
	return nil
}
func (m *MockRegistry) Writer(ctx context.Context) (ports.Writer, error) {
	return nil, nil
}
func (m *MockRegistry) Reader(ctx context.Context) (ports.Reader, error) {
	return nil, nil
}
func (m *MockRegistry) ReaderFromWriter(ctx context.Context, writer ports.Writer) (ports.Reader, error) {
	return nil, nil
}
func (m *MockRegistry) ReaderWithReadCommitted(ctx context.Context) (ports.Reader, error) {
	return nil, nil
}
func (m *MockRegistry) Close() error {
	return nil
}

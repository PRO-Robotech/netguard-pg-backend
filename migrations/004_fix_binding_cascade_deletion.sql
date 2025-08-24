-- +goose Up
-- Fix cascade deletion issue in address_group_bindings
-- The ON DELETE CASCADE constraint causes bindings to be deleted when services are updated
-- This prevents reactive port mapping regeneration from working correctly

-- Drop the existing foreign key constraint with CASCADE
ALTER TABLE address_group_bindings 
DROP CONSTRAINT address_group_bindings_service_namespace_service_name_fkey;

-- Add new foreign key constraint WITHOUT cascade deletion
-- This allows bindings to remain when services are updated
ALTER TABLE address_group_bindings 
ADD CONSTRAINT address_group_bindings_service_namespace_service_name_fkey 
FOREIGN KEY (service_namespace, service_name) 
REFERENCES services(namespace, name) ON DELETE RESTRICT;

-- Also fix ServiceAliases table (same issue)
ALTER TABLE service_aliases 
DROP CONSTRAINT service_aliases_service_namespace_service_name_fkey;

ALTER TABLE service_aliases 
ADD CONSTRAINT service_aliases_service_namespace_service_name_fkey 
FOREIGN KEY (service_namespace, service_name) 
REFERENCES services(namespace, name) ON DELETE RESTRICT;

-- +goose Down
-- Revert to original CASCADE behavior
ALTER TABLE address_group_bindings 
DROP CONSTRAINT address_group_bindings_service_namespace_service_name_fkey;

ALTER TABLE address_group_bindings 
ADD CONSTRAINT address_group_bindings_service_namespace_service_name_fkey 
FOREIGN KEY (service_namespace, service_name) 
REFERENCES services(namespace, name) ON DELETE CASCADE;

ALTER TABLE service_aliases 
DROP CONSTRAINT service_aliases_service_namespace_service_name_fkey;

ALTER TABLE service_aliases 
ADD CONSTRAINT service_aliases_service_namespace_service_name_fkey 
FOREIGN KEY (service_namespace, service_name) 
REFERENCES services(namespace, name) ON DELETE CASCADE;
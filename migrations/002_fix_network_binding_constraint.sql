-- +goose Up
-- Fix foreign key constraint in networks table to reference network_bindings instead of address_group_bindings
-- This fixes the NetworkBinding post-creation update failures

-- Drop the incorrect foreign key constraint
ALTER TABLE networks DROP CONSTRAINT IF EXISTS networks_binding_ref_namespace_binding_ref_name_fkey;

-- Add the correct foreign key constraint that references network_bindings table
ALTER TABLE networks ADD CONSTRAINT networks_binding_ref_namespace_binding_ref_name_fkey 
    FOREIGN KEY (binding_ref_namespace, binding_ref_name) 
    REFERENCES network_bindings(namespace, name) 
    ON DELETE SET NULL;

-- +goose Down
-- Rollback: restore the original (incorrect) constraint
ALTER TABLE networks DROP CONSTRAINT IF EXISTS networks_binding_ref_namespace_binding_ref_name_fkey;

ALTER TABLE networks ADD CONSTRAINT networks_binding_ref_namespace_binding_ref_name_fkey 
    FOREIGN KEY (binding_ref_namespace, binding_ref_name) 
    REFERENCES address_group_bindings(namespace, name) 
    ON DELETE SET NULL;
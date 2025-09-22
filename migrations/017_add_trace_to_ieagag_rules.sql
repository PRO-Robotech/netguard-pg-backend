-- +goose Up
-- Add trace column to ie_ag_ag_rules table to support trace field propagation from RuleS2S

-- Add trace column with default value of FALSE
ALTER TABLE ie_ag_ag_rules
ADD COLUMN trace BOOLEAN NOT NULL DEFAULT FALSE;

-- Create index for better query performance on trace field
CREATE INDEX idx_ie_ag_ag_rules_trace ON ie_ag_ag_rules(trace);

-- +goose Down
-- Remove trace column and its index

DROP INDEX IF EXISTS idx_ie_ag_ag_rules_trace;
ALTER TABLE ie_ag_ag_rules DROP COLUMN trace;
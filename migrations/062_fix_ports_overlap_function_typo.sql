-- +goose Up
-- +goose StatementBegin

-- =====================================================
-- Migration 062: Fix ports_overlap Function Typo
-- =====================================================
-- Purpose: Fix SQL syntax error in ports_overlap helper function
-- Problem: Function has "range1.end" instead of "range1_end"
-- Error: "ERROR: missing FROM-clause entry for table 'range1' (SQLSTATE 42P01)"
--
-- Root Cause:
--   Migration 060/061 line 4 in ports_overlap function:
--     RETURN (range1_start <= range2_end) AND (range2_start <= range1.end);
--                                                              ^^^^^^^^^^^ WRONG!
--   Should be:
--     RETURN (range1_start <= range2_end) AND (range2_start <= range1_end);
--                                                              ^^^^^^^^^^^ CORRECT!
--
-- Solution:
--   Replace "range1.end" with "range1_end" (function parameter name)
--
-- Date: 2025-10-31
-- =====================================================

-- Fix ports_overlap function: range1.end → range1_end
CREATE OR REPLACE FUNCTION ports_overlap(
    range1_start INTEGER,
    range1_end INTEGER,
    range2_start INTEGER,
    range2_end INTEGER
) RETURNS BOOLEAN AS $$
BEGIN
    -- Ranges overlap if: range1.start <= range2.end AND range2.start <= range1.end
    -- FIXED: Changed range1.end to range1_end
    RETURN (range1_start <= range2_end) AND (range2_start <= range1_end);
END;
$$ LANGUAGE plpgsql IMMUTABLE;

COMMENT ON FUNCTION ports_overlap IS '[Migration 062] Fixed typo: range1.end → range1_end';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Revert to buggy version (Migration 060/061)
CREATE OR REPLACE FUNCTION ports_overlap(
    range1_start INTEGER,
    range1_end INTEGER,
    range2_start INTEGER,
    range2_end INTEGER
) RETURNS BOOLEAN AS $$
BEGIN
    -- Buggy version with range1.end instead of range1_end
    RETURN (range1_start <= range2_end) AND (range2_start <= range1.end);
END;
$$ LANGUAGE plpgsql IMMUTABLE;

-- +goose StatementEnd

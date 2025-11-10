-- +goose Up
-- +goose StatementBegin


CREATE OR REPLACE FUNCTION ports_overlap(
    range1_start INTEGER,
    range1_end INTEGER,
    range2_start INTEGER,
    range2_end INTEGER
) RETURNS BOOLEAN AS $$
BEGIN
    RETURN (range1_start <= range2_end) AND (range2_start <= range1_end);
END;
$$ LANGUAGE plpgsql IMMUTABLE;

COMMENT ON FUNCTION ports_overlap IS '[Migration 062] Fixed typo: range1.end → range1_end';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

CREATE OR REPLACE FUNCTION ports_overlap(
    range1_start INTEGER,
    range1_end INTEGER,
    range2_start INTEGER,
    range2_end INTEGER
) RETURNS BOOLEAN AS $$
BEGIN
    RETURN (range1_start <= range2_end) AND (range2_start <= range1.end);
END;
$$ LANGUAGE plpgsql IMMUTABLE;

-- +goose StatementEnd

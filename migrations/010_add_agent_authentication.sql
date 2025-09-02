-- +goose Up
-- Add authentication fields to agents table for SecretData and OwnerCheck support

-- Add authentication columns to agents table
ALTER TABLE agents 
ADD COLUMN raw_value TEXT,                    -- Base64-encoded plain value from SecretData
ADD COLUMN secure_value TEXT,                -- Base64-encoded encrypted value from SecretData  
ADD COLUMN auth_method VARCHAR(20) NOT NULL DEFAULT 'no_auth'; -- Authentication method: 'no_auth' or 'secret'

-- Add constraints for authentication fields
ALTER TABLE agents
ADD CONSTRAINT check_auth_method CHECK (auth_method IN ('no_auth', 'secret')),
ADD CONSTRAINT check_secret_data CHECK (
    (auth_method = 'no_auth' AND raw_value IS NULL AND secure_value IS NULL) OR
    (auth_method = 'secret' AND (raw_value IS NOT NULL OR secure_value IS NOT NULL))
);

-- Add indexes for performance
CREATE INDEX idx_agents_auth_method ON agents(auth_method);
CREATE INDEX idx_agents_raw_value ON agents(raw_value) WHERE raw_value IS NOT NULL;
CREATE INDEX idx_agents_secure_value ON agents(secure_value) WHERE secure_value IS NOT NULL;

-- Add comments for documentation
COMMENT ON COLUMN agents.raw_value IS 'Base64-encoded plain authentication value';
COMMENT ON COLUMN agents.secure_value IS 'Base64-encoded encrypted authentication value: BASE64(IV(16 bytes) + ciphertext)';
COMMENT ON COLUMN agents.auth_method IS 'Authentication method: no_auth (default) or secret';

-- +goose Down
-- Remove authentication fields from agents table

-- Drop indexes
DROP INDEX IF EXISTS idx_agents_secure_value;
DROP INDEX IF EXISTS idx_agents_raw_value;
DROP INDEX IF EXISTS idx_agents_auth_method;

-- Drop constraints  
ALTER TABLE agents
DROP CONSTRAINT IF EXISTS check_secret_data,
DROP CONSTRAINT IF EXISTS check_auth_method;

-- Drop columns
ALTER TABLE agents 
DROP COLUMN IF EXISTS auth_method,
DROP COLUMN IF EXISTS secure_value,
DROP COLUMN IF EXISTS raw_value;
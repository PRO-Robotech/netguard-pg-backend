-- Create schema
CREATE SCHEMA IF NOT EXISTS netguard;

-- Create custom types
CREATE TYPE netguard.transport_protocol AS ENUM ('TCP', 'UDP');
CREATE TYPE netguard.traffic AS ENUM ('ingress', 'egress');

-- Create type for port ranges
CREATE TYPE netguard.port_range AS RANGE (
    subtype = int4
);
CREATE TYPE netguard.port_ranges AS (
    ranges netguard.port_range[]
);
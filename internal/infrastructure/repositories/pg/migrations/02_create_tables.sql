-- Table for services
CREATE TABLE netguard.tbl_service (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    namespace TEXT NOT NULL,
    description TEXT,
    ingress_ports JSONB,
    UNIQUE (name, namespace)
);

-- Table for address groups
CREATE TABLE netguard.tbl_address_group (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    namespace TEXT NOT NULL,
    description TEXT,
    addresses TEXT[],
    UNIQUE (name, namespace)
);

-- Table for address group bindings
CREATE TABLE netguard.tbl_address_group_binding (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    namespace TEXT NOT NULL,
    service_name TEXT NOT NULL,
    service_namespace TEXT NOT NULL,
    address_group_name TEXT NOT NULL,
    address_group_namespace TEXT NOT NULL,
    UNIQUE (name, namespace),
    FOREIGN KEY (service_name, service_namespace) REFERENCES netguard.tbl_service (name, namespace) ON DELETE CASCADE,
    FOREIGN KEY (address_group_name, address_group_namespace) REFERENCES netguard.tbl_address_group (name, namespace) ON DELETE CASCADE
);

-- Table for address group port mappings
CREATE TABLE netguard.tbl_address_group_port_mapping (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    namespace TEXT NOT NULL,
    access_ports JSONB,
    UNIQUE (name, namespace),
    FOREIGN KEY (name, namespace) REFERENCES netguard.tbl_address_group (name, namespace) ON DELETE CASCADE
);

-- Table for rule s2s
CREATE TABLE netguard.tbl_rule_s2s (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    namespace TEXT NOT NULL,
    traffic netguard.traffic NOT NULL,
    service_local_name TEXT NOT NULL,
    service_local_namespace TEXT NOT NULL,
    service_name TEXT NOT NULL,
    service_namespace TEXT NOT NULL,
    UNIQUE (name, namespace),
    FOREIGN KEY (service_local_name, service_local_namespace) REFERENCES netguard.tbl_service (name, namespace) ON DELETE CASCADE,
    FOREIGN KEY (service_name, service_namespace) REFERENCES netguard.tbl_service (name, namespace) ON DELETE CASCADE
);

-- Table for sync status
CREATE TABLE netguard.tbl_sync_status (
    id SERIAL PRIMARY KEY,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    total_affected_rows BIGINT NOT NULL
);
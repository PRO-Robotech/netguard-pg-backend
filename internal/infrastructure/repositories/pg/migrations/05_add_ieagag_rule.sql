-- Table for IEAgAgRules
CREATE TABLE netguard.tbl_ieagag_rule (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    namespace TEXT NOT NULL,
    transport netguard.transport_protocol NOT NULL,
    traffic netguard.traffic NOT NULL,
    address_group_local_name TEXT NOT NULL,
    address_group_local_namespace TEXT NOT NULL,
    address_group_name TEXT NOT NULL,
    address_group_namespace TEXT NOT NULL,
    ports JSONB NOT NULL,
    action TEXT NOT NULL,
    logs BOOLEAN NOT NULL DEFAULT FALSE,
    priority INTEGER NOT NULL DEFAULT 100,
    UNIQUE (name, namespace),
    FOREIGN KEY (address_group_local_name, address_group_local_namespace) REFERENCES netguard.tbl_address_group (name, namespace) ON DELETE CASCADE,
    FOREIGN KEY (address_group_name, address_group_namespace) REFERENCES netguard.tbl_address_group (name, namespace) ON DELETE CASCADE
);
-- Table for address group binding policies
CREATE TABLE netguard.tbl_address_group_binding_policy (
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
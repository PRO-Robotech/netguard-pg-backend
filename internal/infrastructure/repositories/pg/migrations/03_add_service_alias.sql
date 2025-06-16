-- Table for service aliases
CREATE TABLE netguard.tbl_service_alias (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    namespace TEXT NOT NULL,
    service_name TEXT NOT NULL,
    service_namespace TEXT NOT NULL,
    UNIQUE (name, namespace),
    FOREIGN KEY (service_name, service_namespace) REFERENCES netguard.tbl_service (name, namespace) ON DELETE CASCADE
);
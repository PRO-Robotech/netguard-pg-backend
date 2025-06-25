#!/bin/bash

set -e

# Создаем директорию для сертификатов
mkdir -p certs

echo "Generating TLS certificates for netguard-apiserver..."

# Генерируем приватный ключ
openssl genrsa -out certs/tls.key 2048

# Создаем запрос на сертификат с SAN
openssl req -new -key certs/tls.key -out certs/tls.csr -subj "/CN=netguard-apiserver" \
  -config <(cat <<EOF
[req]
distinguished_name = req_distinguished_name
req_extensions = v3_req
prompt = no

[req_distinguished_name]
CN = netguard-apiserver

[v3_req]
keyUsage = keyEncipherment, dataEncipherment
extendedKeyUsage = serverAuth
subjectAltName = @alt_names

[alt_names]
DNS.1 = localhost
DNS.2 = netguard-apiserver
DNS.3 = netguard-apiserver.default.svc.cluster.local
DNS.4 = netguard-apiserver.default.svc
IP.1 = 127.0.0.1
EOF
)

# Генерируем самоподписанный сертификат
openssl x509 -req -in certs/tls.csr -signkey certs/tls.key -out certs/tls.crt -days 365 \
  -extensions v3_req -extfile <(cat <<EOF
[v3_req]
keyUsage = keyEncipherment, dataEncipherment
extendedKeyUsage = serverAuth
subjectAltName = @alt_names

[alt_names]
DNS.1 = localhost
DNS.2 = netguard-apiserver
DNS.3 = netguard-apiserver.default.svc.cluster.local
DNS.4 = netguard-apiserver.default.svc
IP.1 = 127.0.0.1
EOF
)

# Удаляем CSR
rm certs/tls.csr

echo "Certificates generated successfully in certs/"
echo "Certificate: certs/tls.crt"
echo "Private key: certs/tls.key"
echo ""
echo "To create Kubernetes secret run:"
echo "kubectl create secret tls netguard-apiserver-certs --cert=certs/tls.crt --key=certs/tls.key"
echo ""
echo "To verify certificate:"
echo "openssl x509 -in certs/tls.crt -text -noout" 
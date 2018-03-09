set -e

CN_BASE="generic_webhook_admission_example"

cat > server.conf << EOF
[req]
req_extensions = v3_req
distinguished_name = req_distinguished_name
[req_distinguished_name]
[ v3_req ]
basicConstraints = CA:FALSE
keyUsage = nonRepudiation, digitalSignature, keyEncipherment
extendedKeyUsage = clientAuth, serverAuth
EOF

# Create a certificate authority
openssl genrsa -out caKey.pem 2048
openssl req -x509 -new -nodes -key caKey.pem -days 100000 -out caCert.pem -subj "/CN=${CN_BASE}_ca"

# Create a server certiticate
openssl genrsa -out serverKey.pem 2048

# TODO: not sure what CN to use
openssl req -new -key serverKey.pem -out server.csr -subj "/CN=kube-apiserver" -config server.conf
openssl x509 -req -in server.csr -CA caCert.pem -CAkey caKey.pem -CAcreateserial -out serverCert.pem -days 100000 -extensions v3_req -extfile server.conf

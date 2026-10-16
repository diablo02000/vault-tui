#!/usr/bin/env bash
# Seed a Vault *dev* server with the fixtures used by the integration tests:
#   secret/app/db (KV v2), legacy/old (KV v1), policy "reader", userpass user "bob".
# Usage: VAULT_ADDR=<addr> VAULT_TOKEN=<root-token> scripts/seed-dev-vault.sh
set -euo pipefail

: "${VAULT_ADDR:?VAULT_ADDR is required}"
: "${VAULT_TOKEN:?VAULT_TOKEN is required}"

call() { # call METHOD PATH JSON
  curl --silent --show-error --fail-with-body \
    --request "$1" --header "X-Vault-Token: ${VAULT_TOKEN}" \
    --data "$3" "${VAULT_ADDR}/v1/$2" > /dev/null
}

for _ in $(seq 1 30); do
  curl --silent --fail "${VAULT_ADDR}/v1/sys/health" > /dev/null && break
  sleep 1
done

call POST sys/mounts/legacy '{"type":"kv","options":{"version":"1"}}'
call POST legacy/old '{"k":"v"}'
call POST secret/data/app/db '{"data":{"password":"hunter2"}}'
call PUT sys/policies/acl/reader '{"policy":"path \"secret/*\" { capabilities = [\"read\"] }"}'
call POST sys/auth/userpass '{"type":"userpass"}'
call POST auth/userpass/users/bob '{"password":"x","token_policies":["reader"]}'

echo "dev Vault seeded"

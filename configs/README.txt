# Production launch config (inject secrets at deploy; do not commit PIN or tokens).
# SQL: MYSQL_DSN or ACCOUNT_ASSETS_PG_DSN (with ACCOUNT_ASSETS_DIALECT=postgres).
# KMS: KMS_KIND=pkcs11 requires PKCS11_MODULE, PKCS11_SLOT, PKCS11_PIN.
# KMS: KMS_KIND=cloud-kms (or cloudhsm) requires KMS_ENDPOINT and KMS_TOKEN.
# IAM: AUTH_KIND=iam requires IAM_ISSUER and IAM_JWKS_URL.
# KYT: KYT_MODE=vendor requires KYT_VENDOR_URL.
# Chain: CHAIN_DRIVER=rpc; public JSON-RPC from configs/app.yaml or CHAIN_RPC_*.
# Catalog hosts: kms-vault.kucoin.net, iam-gateway.kucoin.net, kyt.kucoin.net.
# Registry/config: ETCD_ENDPOINTS (etcd-0.kucoin.net:2379). Production fails closed if empty.

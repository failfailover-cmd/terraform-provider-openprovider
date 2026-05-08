# terraform-provider-openprovider (MVP)

Кастомный Terraform provider для Openprovider.

## Что уже реализовано

- Provider: `openprovider`
- Resource: `openprovider_domain_nameservers`
  - меняет NS домена через REST API

## Конфиг

```hcl
provider "openprovider" {
  base_url = "https://api.openprovider.eu/v1beta"
  username = var.openprovider_username
  password = var.openprovider_password
}

resource "openprovider_domain_nameservers" "example" {
  domain      = "example.com"
  nameservers = ["katja.ns.cloudflare.com", "nero.ns.cloudflare.com"]
}
```

Также поддерживаются env:
- `OPENPROVIDER_MAIN_API_URL`
- `OPENPROVIDER_MAIN_USERNAME`
- `OPENPROVIDER_MAIN_PASSWORD`

## Ограничения MVP

- `Read` пока сохраняет last-known state без полного reconciliation NS через API.
- Операция `Delete` no-op (провайдер не откатывает NS автоматически).

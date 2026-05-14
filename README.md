# terraform-provider-openprovider

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
  use_ns_group   = true
  disable_dnssec = true
}
```

Также поддерживаются env:
- `OPENPROVIDER_MAIN_API_URL`
- `OPENPROVIDER_MAIN_USERNAME`
- `OPENPROVIDER_MAIN_PASSWORD`

## Поведение и ограничения

- `Read` делает reconciliation NS через Openprovider REST API.
- Для операций `Read/Update` провайдер резолвит `domain -> domain_id` через `GET /domains`.
- Операция `Delete` no-op (провайдер не откатывает NS автоматически).
- По умолчанию ресурс:
  1. отключает DNSSEC (`disable_dnssec = true`),
  2. применяет NS через `ns_group` (`use_ns_group = true`).
- `ns_group` создаётся/переиспользуется детерминированно по паре NS, поэтому один и тот же group
  может применяться к многим доменам.
- Если `use_ns_group = false`, провайдер сначала пробует прямой `name_servers` update.
  При Openprovider `code 245` (`Nameserver-update failed`) автоматически включается fallback на `ns_group`.


Retry tuning:
- `max_retries`
- `base_backoff_ms`
- `max_backoff_ms`
- `request_timeout_ms`

`nameservers` is handled as a set to avoid diff on ordering.

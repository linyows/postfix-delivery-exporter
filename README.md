# postfix-delivery-exporter

Prometheus exporter that turns Postfix outbound delivery logs (`postfix/smtp`
lines) into metrics: SMTP status codes, DSN classes, and per-stage delivery
timings. The exporter tails the mail log and exposes `/metrics`.

## Metrics

| Name | Type | Labels | Description |
|---|---|---|---|
| `postfix_delivery_total` | counter | `node, relay, status, dsn_class, smtp_code` | Delivery attempts by outcome |
| `postfix_delivery_duration_seconds` | histogram | `node, relay, status` | `delay=` field. End-to-end delivery time when `status=sent`; for `deferred`/`bounced` it includes prior queue wait, so values can reach hours |
| `postfix_delivery_stage_seconds` | histogram | `node, relay, stage` | `delays=a/b/c/d` split. `stage` ∈ {`before_qmgr`, `in_qmgr`, `conn_setup`, `transmission`}. `conn_setup + transmission` approximates upstream SMTP response time |
| `postfix_delivery_parse_errors_total` | counter | – | Lines that looked like delivery records but failed to parse |

`status` is `sent` / `deferred` / `bounced`. `dsn_class` is the leading digit of
the DSN code (`2` / `4` / `5`) to bound cardinality.

## Install

```sh
docker pull ghcr.io/linyows/postfix-delivery-exporter:latest
```

Or build from source (Go ≥ 1.25):

```sh
go install github.com/linyows/postfix-delivery-exporter@latest
```

## Usage

Tail the system mail log:

```sh
postfix-delivery-exporter -log.files=/var/log/maillog
```

Multiple files (rotation handled automatically):

```sh
postfix-delivery-exporter -log.files=/var/log/maillog,/var/log/mail.log
```

Replay an existing log via stdin:

```sh
cat /var/log/maillog | postfix-delivery-exporter -log.stdin
```

Container with the host log mounted read-only:

```sh
docker run --rm -p 9620:9620 \
  -v /var/log:/var/log:ro \
  ghcr.io/linyows/postfix-delivery-exporter:latest \
  -log.files=/var/log/maillog
```

The exporter listens on `:9620` and serves `/metrics`.

## Flags

| Flag | Default | Description |
|---|---|---|
| `-log.files` | – | Comma-separated list of files to tail |
| `-log.stdin` | false | Read from stdin (mutually exclusive with `-log.files`) |
| `-log.from-beginning` | false | Read each file from the beginning rather than current EOF |
| `-relay.allowlist` | – | Comma-separated relay hostnames to keep as labels; others reported as `other`. Empty = pass through all |
| `-web.listen-address` | `:9620` | Listen address for `/metrics` |
| `-web.telemetry-path` | `/metrics` | URL path for the metrics endpoint |

## Example queries

Bounce/defer rate per relay:

```promql
sum(rate(postfix_delivery_total{status!="sent"}[5m])) by (relay, status)
  / ignoring(status) group_left
sum(rate(postfix_delivery_total[5m])) by (relay)
```

Average upstream SMTP response time (excluding queue wait):

```promql
sum(rate(postfix_delivery_stage_seconds_sum{stage=~"conn_setup|transmission"}[5m])) by (relay)
  /
sum(rate(postfix_delivery_stage_seconds_count{stage=~"conn_setup|transmission"}[5m])) by (relay)
```

p95 end-to-end delivery time for successful sends:

```promql
histogram_quantile(0.95,
  sum(rate(postfix_delivery_duration_seconds_bucket{status="sent"}[5m])) by (le, relay))
```

## Cardinality

Each delivery line produces one series per unique
`(node, relay, status, dsn_class, smtp_code)` tuple in
`postfix_delivery_total`. When destinations are many, set
`-relay.allowlist` to keep only relays of interest; everything else folds into
`relay="other"`.

## Container permissions

The image runs as `nonroot` (uid 65532). Postfix typically writes the log as
`syslog:adm 0640`, so either run with `--user 0:0`, or
`--user 65532:<adm-gid>`, or mount a world-readable copy.

## Development

```sh
go test ./...
go build .
docker build -t postfix-delivery-exporter:dev .
```

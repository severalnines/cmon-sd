# cmon-sd
Prometheus service discovery for CMON

Usage:

```
CMON_USERNAME=cmon CMON_PASSWORD=secret ./cmon_sd
```

Verify it works:

```
curl 127.0.0.1:8080
[{"targets":["10.10.10.17:9100","10.10.10.17:9011","10.10.10.17:9104","10.10.10.16:9100","10.10.10.16:9011","10.10.10.16:9104","10.10.10.18:9100","10.10.10.18:9011","10.10.10.18:9104"],"labels":{"ClusterID":"641","ClusterName":"PXC57","ClusterType":"GALERA","cid":"641"}}]
```

## Options
`-p <int>    -- listening port, defaults to 8080`

## Environment variables

| Variable        | Required | Default                    | Description                        |
|-----------------|----------|----------------------------|------------------------------------|
| `CMON_USERNAME` | yes      | —                          | ClusterControl username            |
| `CMON_PASSWORD` | yes      | —                          | ClusterControl password            |
| `CMON_ENDPOINT` | no       | `https://127.0.0.1:9501`  | ClusterControl API endpoint        |
| `CMON_SD_PORT`  | no       | `8080`                     | Listening port (overrides `-p`)    |

## Cluster tags as labels

Cluster tags from ClusterControl are exposed as target labels:

- A tag of the form `key:value` or `key=value` becomes the label
  `key="value"`, e.g. the tag `tenant:acme` or `tenant=acme` becomes
  `tenant="acme"`. The tag is split on the first `:` or `=`, so the value may
  contain either character (`url:http://host` becomes `url="http://host"`).
  Characters not valid in a Prometheus label name are replaced with `_`
  (`cost-center:42` becomes `cost_center="42"`).
- All tags are also exposed in a combined `tags` label, sorted and
  comma-separated (`production,tenant:acme`). Key/value tags are normalized to
  `key:value` there, so `tenant=acme` is listed as `tenant:acme`.
- Tags never override the built-in labels (`ClusterID`, `cid`, `ClusterName`,
  `ClusterType`, `ControllerId`, `tags`), and keys starting with `__` are ignored.

Set the tags when creating the cluster (or later in ClusterControl), and they
will show up on all metrics scraped through `http_sd_configs`.

Example output for a cluster tagged `tenant:acme` and `production`:

```
[{"targets":["10.10.10.17:9011","10.10.10.17:9100","10.10.10.17:9104"],"labels":{"ClusterID":"641","ClusterName":"PXC57","ClusterType":"GALERA","ControllerId":"...","cid":"641","tags":"production,tenant:acme","tenant":"acme"}}]
```

# Sample prometheus.yml
```
---
alerting:
  alertmanagers:
  - static_configs:
    - targets:
global:
  evaluation_interval: 10s
  external_labels:
    monitor: clustercontrol
  scrape_interval: 10s
  scrape_timeout: 10s
rule_files:
scrape_configs:
- http_sd_configs:
  - url: http://10.10.10.1:8080
  job_name: johan
...

````

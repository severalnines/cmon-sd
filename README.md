# cmon-sd
Prometheus service discovery for CMON

Usage:

CMON_USERNAME=cmon CMON_PASSWORD=secret ./cmon_sd 

Verify it works:

```
curl 127.0.0.1:8080
[{"targets":["10.10.10.17:9100","10.10.10.17:9011","10.10.10.17:9104","10.10.10.16:9100","10.10.10.16:9011","10.10.10.16:9104","10.10.10.18:9100","10.10.10.18:9011","10.10.10.18:9104"],"labels":{"ClusterID":"641","ClusterName":"PXC57","ClusterType":"GALERA","cid":"641"}}]
```

# Sample prometheues.yaml
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

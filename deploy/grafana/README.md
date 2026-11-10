# Grafana dashboard

`linochi-erp-dashboard.json` shows the backend: replicas up, the release the pods
run, traffic and response times (NFK-03), database query times, logins by result
(the refused-logins alert watches the same series), sessions, audit entries, and
memory, CPU and goroutines per pod. The memory and CPU panels are what the pod
limits are set from (NFK-04).

## Import

Grafana → Dashboards → New → Import → upload `linochi-erp-dashboard.json`, and pick
the Prometheus of the cluster as the data source. This is a plain export imported
by hand, as for linochi-web (Grafana has a PVC). The variables `namespace` and `job`
resolve themselves from `up`.

## What it needs

The ServiceMonitor of the chart (`deploy/helm/linochi-erp-backend/templates/servicemonitor.yaml`)
scrapes `/metrics` on the internal port of every pod. The queries use the labels
`job` (the Service's name) and `namespace`.

The queries were checked for syntax against the cluster's Prometheus. They were
**not** run against real data, since the app is not deployed yet: look at every
panel after the first sync (H3/H4).

Counters are per pod and start at zero when a pod restarts; `rate()` and
`increase()` handle that, and every panel sums over the pods.

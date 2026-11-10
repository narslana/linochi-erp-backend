#!/usr/bin/env bash
# Lints and renders the Helm chart and checks what must hold in the cluster
# (NFK-02, NFK-04, NFK-12, NFK-13, INS-05). It needs only helm and awk, which the
# nars-ci image has, and runs the same on a laptop.
#
#   scripts/check-chart.sh deploy/helm/linochi-erp-backend
set -euo pipefail

chart=${1:?usage: check-chart.sh <chart dir>}
name=linochi-erp-backend
fails=0

fail() { echo "CHART FAIL: $*" >&2; fails=$((fails + 1)); }

render() { helm template "$name" "$chart" "$@"; }

# kind_of <kind>: the documents of one kind, from rendered YAML on stdin. Comment
# lines are dropped: a comment that mentions /metrics is not a route to it.
kind_of() {
  awk -v want="$1" '
    function flush() { if (doc ~ ("(^|\n)kind: " want "\n")) printf "%s---\n", doc; doc = "" }
    /^---[ \t]*$/ { flush(); next }
    /^[ \t]*#/ { next }
    { doc = doc $0 "\n" }
    END { flush() }'
}

has()  { grep -q -- "$1" <<<"$2"; }

helm lint "$chart" >/dev/null || fail "helm lint"
helm lint "$chart" --set ingress.enabled=false --set caBundle.enabled=false \
  --set metrics.serviceMonitor.enabled=false --set metrics.prometheusRule.enabled=false >/dev/null \
  || fail "helm lint with the switches off"

all=$(render) || { fail "helm template"; echo "$fails check(s) failed" >&2; exit 1; }
deploy=$(kind_of Deployment <<<"$all")
job=$(kind_of Job <<<"$all")
svc=$(kind_of Service <<<"$all")
route=$(kind_of IngressRoute <<<"$all")
rule=$(kind_of PrometheusRule <<<"$all")

# ── NFK-02: stateless, two replicas, no downtime, spread over nodes.
has 'replicas: 2' "$deploy"                          || fail "the Deployment is not 2 replicas"
has 'maxUnavailable: 0' "$deploy"                    || fail "a rolling update may take a pod away before the new one is ready"
has 'topologySpreadConstraints' "$deploy"            || fail "the replicas are not spread over nodes"
has 'kind: PodDisruptionBudget' "$all"               || fail "no PodDisruptionBudget"
has 'readOnlyRootFilesystem: true' "$deploy"         || fail "the root filesystem is writable (NFK-02: no files on the pod)"
has 'runAsNonRoot: true' "$deploy"                   || fail "the pod may run as root"

# ── NFK-04: requests 32 MB / 50m, no hard limit until measured.
has 'memory: 32Mi' "$deploy" && has 'cpu: 50m' "$deploy" || fail "requests are not 32Mi / 50m"
if has 'limits:' "$deploy"; then fail "a limit is set before it was measured (NFK-04)"; fi

# ── INS-05: the app role and the owner never meet. The Deployment has no way to
# change the schema, and the migration Job is the only place the owner URL is.
if has 'MIGRATE_DATABASE_URL' "$deploy"; then fail "the Deployment has the owner's URL"; fi
has 'name: DATABASE_URL' "$deploy"                   || fail "the Deployment has no DATABASE_URL"
has 'MIGRATE_DATABASE_URL' "$job"                    || fail "the migration Job has no MIGRATE_DATABASE_URL"
if has 'name: DATABASE_URL' "$job"; then fail "the migration Job has the app role's URL as well"; fi
has 'argocd.argoproj.io/hook: PreSync' "$job"        || fail "the migration Job is not a PreSync hook"
has 'argocd.argoproj.io/hook-delete-policy: BeforeHookCreation' "$job" || fail "the migration Job is not replaced at the next sync"
has 'args: \["migrate", "up"\]' "$job"               || fail "the migration Job does not run 'api migrate up'"

# ── The internal port (probes, metrics) has no route. Only /api and /auth are public.
has 'PathPrefix(`/api`)' "$route"                    || fail "no route for /api"
has 'PathPrefix(`/auth`)' "$route"                   || fail "no route for /auth"
if has '9090' "$route" || has 'metrics' "$route" || has 'healthz' "$route" || has 'readyz' "$route"; then
  fail "the IngressRoute reaches the internal port or its paths"
fi
has 'port: 9090' "$svc"                              || fail "the Service does not expose the internal port to Prometheus"
if [ "$(grep -c 'port: 8080' <<<"$route")" -ne 1 ]; then fail "the IngressRoute must have exactly one backend, the public port 8080"; fi

# ── Config: the login settings are there, and the secret is from a Secret.
for v in OIDC_ISSUER OIDC_CLIENT_ID OIDC_CLIENT_SECRET OIDC_REDIRECT_URL OTEL_EXPORTER_OTLP_ENDPOINT POD_NAME DB_MAX_CONNS; do
  has "name: $v" "$deploy" || fail "the Deployment has no $v"
done
if grep -A3 'name: OIDC_CLIENT_SECRET' <<<"$deploy" | grep -q '^ *value:'; then fail "the client secret is a plain value"; fi

# ── NFK-12: the internal CA is added to the system roots, not instead of them.
has 'SSL_CERT_DIR' "$deploy" && has '/etc/ssl/certs:/extra-ca-certs' "$deploy" || fail "SSL_CERT_DIR does not extend the system roots"
has 'mountPath: /extra-ca-certs' "$deploy"           || fail "the CA bundle is not mounted"
noca=$(render --set caBundle.enabled=false | kind_of Deployment)
if has 'SSL_CERT_DIR' "$noca" || has '/extra-ca-certs' "$noca"; then fail "caBundle.enabled=false still adds the CA"; fi

# ── Alerts: "App down" and refused logins. The label is what Prometheus selects on
# (ruleSelector), without it the rule is loaded by nobody.
has 'release: prometheus-stack' "$rule"              || fail "the PrometheusRule has no release label"
has 'alert: LinochiErpBackendDown' "$rule"           || fail "no App down alert"
has 'alert: LinochiErpRefusedLogins' "$rule"         || fail "no refused-logins alert"
# The expression itself (between the alert's name and its `for:`), not the
# description, which quotes the same query.
expr=$(awk '/alert: LinochiErpRefusedLogins/ {on = 1; next} on && /^ *for:/ {exit} on' <<<"$rule")
has 'sum(increase(linochi_auth_logins_total{result!="ok"' "$expr" || fail "the refused-logins expression does not watch result!=\"ok\""
has '\[10m\])) > 10' "$expr"                         || fail "the refused-logins expression is not 'more than 10 in 10m'"
has 'for: 5m' "$rule"                                || fail "the refused-logins alert does not wait 5m"

# ── The image: an immutable tag, never latest (a moved tag would show Synced
# while the old image runs).
has 'image: "gitea.lab.internal/linochi/linochi-erp-backend:' "$deploy" || fail "the image is not from the Gitea registry"
if has ':latest' "$deploy" || has ':latest' "$job"; then fail "the image is latest"; fi
dtag=$(grep -o 'linochi-erp-backend:[^"]*' <<<"$deploy" | head -1)
jtag=$(grep -o 'linochi-erp-backend:[^"]*' <<<"$job" | head -1)
[ -n "$dtag" ] && [ "$dtag" = "$jtag" ] || fail "the Deployment and the migration Job run different images ($dtag, $jtag)"

# ── The switches work both ways.
off=$(render --set ingress.enabled=false --set metrics.serviceMonitor.enabled=false --set metrics.prometheusRule.enabled=false)
if has 'kind: IngressRoute' "$off" || has 'kind: ServiceMonitor' "$off" || has 'kind: PrometheusRule' "$off"; then fail "a switch that is off still renders"; fi
has 'kind: ServiceMonitor' "$all"                    || fail "no ServiceMonitor"

if [ "$fails" -ne 0 ]; then echo "$fails chart check(s) failed" >&2; exit 1; fi
echo "Chart checks passed: $chart"

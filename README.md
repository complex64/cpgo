# CPGO (Continuous Profile Guided Optimization)

Collects a CPU profile from a running Go application and opens or updates a GitHub pull request with the refreshed PGO profile file.

Example config (`config.yaml`):

```yaml
profile:
  url: "https://localhost:1234/debug/pprof/profile"
  seconds: 30
  timeout: "45s"
  headers:
    Authorization: "Bearer <token>"
repository:
  owner: "acme"
  name: "payments-service"
  pgo_path: "default.pgo"
  base_branch: "" # optional; empty means repository default branch
  head_branch: "cpgo"
github:
  app_id: 123456
  private_key_path: "/secrets/github-app.pem"
  token: "" # optional alternative to app auth
  timeout: "30s"
pull_request:
  title: "perf(pgo): refresh pgo profile"
  body: "Automated PGO profile refresh."
  managed_by_marker: "<!-- managed-by:cpgo -->"
commit:
  message: "perf(pgo): refresh pgo profile"
runtime:
  timeout: "2m"
```

Run:

```bash
go run ./cmd/cpgo -config ./config.yaml
```

# Global AUTO Routing Design

## Summary

Traffic AI will add an administrator-managed global `AUTO` routing capability. Admins create virtual models such as `auto`, `auto-fast`, or `auto-coding`; users call those names exactly like normal models. At request time, the gateway resolves the virtual model to one real model and one healthy `model_account` based on protocol, token group, request shape, model capabilities, cost, latency, load, and circuit breaker state.

Billing always uses the resolved real model. User-facing logs show both the requested virtual model and the resolved real model so AUTO remains explainable rather than a black box.

## Goals

- Let super admins create and publish global AUTO models.
- Keep the user API compatible with existing OpenAI/Anthropic/Gemini-style calls.
- Route `model: "auto"` to a concrete model before upstream forwarding.
- Charge by the final resolved model's pricing.
- Reuse existing `tokenGroup`, `models`, `model_accounts`, fallback, circuit breaker, rate limit, and usage log foundations.
- Make route decisions auditable and debuggable.

## Non-Goals

- User-created custom AUTO policies.
- ML-based model selection.
- Fixed blended pricing for AUTO.
- Cross-request quality feedback learning.
- Changing the public API request shape.

## Product Model

### Admin Experience

Admins manage AUTO from the model management area:

- Create a virtual model name, for example `auto`.
- Choose a strategy: `balanced`, `fast`, `cheap`, `quality`, `coding`, or `reasoning`.
- Choose candidate real models.
- Set optional bounds: max estimated cost per request, max p95 latency, min context window, allowed providers, and fallback behavior.
- Publish or unpublish the virtual model.
- Bind the virtual model and its candidates through existing token group availability.

Only `super_admin` can create or change global AUTO policies. Regular users only see published virtual models in `/v1/models` when their API key's `tokenGroup` can reach at least one valid candidate.

`/v1/models` uses loose availability semantics: it shows a published AUTO model when the token group can reach at least one active candidate model/account for at least one supported protocol. It does not hide AUTO because of temporary circuit breaker state, transient latency, current balance, or request-specific context limits. Request-time routing may still fail with a clear `ErrNoAvailableRoute` if all candidates are unhealthy or the specific request violates hard policy filters. This avoids model-list flicker during incidents.

### User Experience

Users call AUTO as a normal model:

```json
{
  "model": "auto",
  "messages": []
}
```

Usage logs display:

- requested model: `auto`
- resolved model: `gpt-5.4-mini`
- cost: based on `gpt-5.4-mini`
- route mode and a concise route reason

## Data Model

Use `models` to represent the virtual model because existing docs, `/v1/models`, pricing UI, token group routing, and request parsing are model-name centric.

### `models` changes

Add fields:

```sql
ALTER TABLE models
  ADD COLUMN is_virtual TINYINT NOT NULL DEFAULT 0,
  ADD COLUMN virtual_type VARCHAR(30) NOT NULL DEFAULT '',
  ADD COLUMN context_window_tokens INT NOT NULL DEFAULT 0,
  ADD COLUMN capability_tags JSON NULL;
```

Rules:

- Real models: `is_virtual = 0`.
- AUTO models: `is_virtual = 1`, `virtual_type = 'auto_route'`.
- AUTO model prices should be `0` or ignored for billing; final billing uses the resolved model.
- `capability_tags` gives real models searchable traits such as `coding`, `reasoning`, `long_context`, `tool_calling`, `json_schema`, `vision`, or `streaming`.
- `context_window_tokens` is a hard routing constraint when set.

### `auto_route_policies`

```sql
CREATE TABLE auto_route_policies (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  virtual_model_id BIGINT UNSIGNED NOT NULL,
  name VARCHAR(100) NOT NULL DEFAULT '',
  strategy VARCHAR(30) NOT NULL DEFAULT 'balanced',
  rules_json JSON NULL,
  is_active TINYINT NOT NULL DEFAULT 1,
  version INT NOT NULL DEFAULT 1,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uk_virtual_model (virtual_model_id),
  KEY idx_active_strategy (is_active, strategy),
  CONSTRAINT fk_auto_route_policy_virtual_model
    FOREIGN KEY (virtual_model_id) REFERENCES models(id) ON DELETE CASCADE
);
```

`rules_json` stores tunable admin policy:

```json
{
  "max_estimated_cost_micro_usd": 5000,
  "max_p95_latency_ms": 8000,
  "min_context_window_tokens": 0,
  "allow_provider_ids": ["openai", "anthropic"],
  "require_capabilities": ["streaming"],
  "allow_quality_upgrade": true,
  "fallback_across_models": true,
  "sticky_ttl_sec": 3600
}
```

### `auto_route_candidates`

```sql
CREATE TABLE auto_route_candidates (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  policy_id BIGINT UNSIGNED NOT NULL,
  target_model_id BIGINT UNSIGNED NOT NULL,
  priority INT NOT NULL DEFAULT 100,
  weight INT NOT NULL DEFAULT 1,
  min_request_context_tokens INT NOT NULL DEFAULT 0,
  quality_score INT NOT NULL DEFAULT 50,
  cost_bias INT NOT NULL DEFAULT 0,
  latency_bias INT NOT NULL DEFAULT 0,
  is_active TINYINT NOT NULL DEFAULT 1,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uk_policy_target_model (policy_id, target_model_id),
  KEY idx_policy_active (policy_id, is_active),
  CONSTRAINT fk_auto_route_candidate_policy
    FOREIGN KEY (policy_id) REFERENCES auto_route_policies(id) ON DELETE CASCADE,
  CONSTRAINT fk_auto_route_candidate_target_model
    FOREIGN KEY (target_model_id) REFERENCES models(id) ON DELETE CASCADE
);
```

`models.context_window_tokens` is the real model's maximum context window. `auto_route_candidates.min_request_context_tokens` is a policy-specific lower bound: it can say "only consider this candidate when the request is at least this large." `quality_score` is an admin-maintained prior from 0 to 100. It lets admins encode known quality differences without hard-coding vendor names into the gateway.

API validation must enforce bounded fields:

- `quality_score`, `cost_bias`, and `latency_bias`: 0-100.
- `weight`: 0-1000.
- `strategy`: one of the supported strategy constants.
- capability tags: only known values from the code-level vocabulary.

### `usage_logs` changes

Add fields:

```sql
ALTER TABLE usage_logs
  ADD COLUMN requested_model VARCHAR(100) NOT NULL DEFAULT '',
  ADD COLUMN resolved_model VARCHAR(100) NOT NULL DEFAULT '',
  ADD COLUMN auto_route_policy_id BIGINT UNSIGNED NOT NULL DEFAULT 0,
  ADD COLUMN route_mode VARCHAR(30) NOT NULL DEFAULT '',
  ADD COLUMN route_reason JSON NULL,
  ADD COLUMN route_score INT NOT NULL DEFAULT 0,
  ADD KEY idx_requested_model (requested_model),
  ADD KEY idx_resolved_model (resolved_model),
  ADD KEY idx_auto_policy_time (auto_route_policy_id, created_at);
```

Backfill rule:

- Existing rows: `requested_model = model`, `resolved_model = model`.
- During the transition, keep `usage_logs.model` equal to the resolved model for existing filters and billing reports. New UI should prefer `requested_model` and `resolved_model`.
- The migration must backfill all existing rows in the same release that adds the columns. Until the backfill completes, report queries that move to `resolved_model` must use a compatibility predicate: `resolved_model = ? OR (resolved_model = '' AND model = ?)`.

## Routing Architecture

### Domain Types

Introduce a richer routing request:

```go
type RouteRequest struct {
    TokenGroup       string
    RequestedModel   string
    Protocol         string
    UserID           int64
    APIKeyID         int64
    EstimatedTokens  int
    Stream           bool
    ReasoningEffort  string
    RequestFeatures  RequestFeatures
    ExcludeAccountIDs []int64
    ExcludeModelIDs   []int64
}

type RouteDecision struct {
    Account           *model.ModelAccount
    Model             *model.Model
    RequestedModel    string
    ResolvedModel     string
    IsAutoRoute       bool
    PolicyID          int64
    Mode              string
    Score             int
    Reason            string
}
```

`RoutingService` should gain:

```go
SelectRoute(ctx context.Context, req RouteRequest) (*RouteDecision, error)
```

Existing methods can delegate to `SelectRoute` to preserve compatibility while the gateway migrates call sites.

### Request Feature Extraction

Feature extraction must be cheap and deterministic in P1. Do not call an LLM to classify requests.

Inputs:

- estimated input tokens
- `stream`
- protocol path
- message count
- presence of code blocks, diffs, stack traces, JSON/schema words, tool-use hints
- requested `reasoning_effort`
- temperature and max token settings when available

Output examples:

```go
RequestFeatures{
    HasCode: true,
    HasLongContext: false,
    WantsReasoning: true,
    WantsJSON: false,
}
```

### Candidate Filtering

AUTO routing filters candidates in this order:

1. Resolve `requested_model` from `models`; if it is not virtual, use the normal route path.
2. Load active policy by `virtual_model_id`.
3. Load active `auto_route_candidates`.
4. Keep only active, listed, non-virtual target models.
5. Keep models whose token group has at least one active `model_account` for the requested protocol. AUTO fallback never crosses protocol boundaries; an OpenAI Chat request cannot fallback to an Anthropic Messages-only candidate unless that model also has an OpenAI-compatible account.
6. Apply hard capability constraints, context window, max cost, provider allowlist, and admin rules.
7. Filter open circuit breaker accounts.
8. Filter excluded accounts/models for fallback.
9. Score remaining candidates and select one.

If filtering leaves no candidate, return `ErrNoAvailableRoute`. The gateway may then perform configured fallback only if the request has not started writing a streaming response.

Fallback stops immediately when a subsequent selection returns `ErrNoAvailableRoute`; the gateway must not loop with unchanged inputs. Fallback attempts are capped by the existing gateway retry configuration plus a hard AUTO cap of 10 attempts per request, whichever is lower.

### Scoring

Use explainable weighted scoring:

```text
score =
  capability_score
+ quality_score
+ health_score
+ latency_score
+ cost_score
+ load_score
+ candidate_weight
+ sticky_bonus
+ small_jitter
```

Strategy adjusts weights:

| Strategy | Main Bias |
|---|---|
| `balanced` | moderate quality, cost, latency, and health |
| `fast` | latency and low load |
| `cheap` | low estimated cost; no automatic quality upgrade unless allowed |
| `quality` | quality prior and reasoning capability |
| `coding` | coding tags, tool calling, long context |
| `reasoning` | reasoning tags, higher quality prior, context |

The decision must include a short machine-generated reason such as:

```text
strategy=balanced; selected=gpt-5.4-mini; reasons=healthy,low_latency,cost_ok,code_capable; filtered=2_circuit_open,1_context_too_small
```

### Sticky Routing

P1 may optionally keep user/API key stickiness with Redis:

```text
auto:sticky:{policy_id}:{api_key_id}:{feature_bucket} -> target_model_id
```

Use stickiness only if the selected model remains valid under current hard filters. Do not keep stickiness across health failures or context-window violations.

Stickiness is best-effort. Changes to feature extraction may invalidate old keys; the TTL should stay short enough that this is operationally harmless.

### Policy Consistency

Policy edits use optimistic locking through `auto_route_policies.version`. Admin PATCH requests must include the version they read; the API rejects stale updates.

Gateway policy caches, if added, must use short TTLs and must not cache deleted policies indefinitely. In-flight requests may finish with the policy snapshot they already loaded, but new requests should observe deactivation after the cache TTL.

## Gateway Flow

For chat completions:

1. Parse request and keep `requestedModel = chatReq.Model`.
2. Estimate tokens and extract request features.
3. Call `SelectRoute`.
4. Run rate limits with the resolved model/account. For user/model scoped limits, use the resolved model; for API key/global scopes unchanged.
5. Check listed state. AUTO virtual model must be listed, and resolved real model must be active/listed.
6. Estimate cost with the resolved real model.
7. Check balance and pre-deduct based on the current resolved real model.
8. Rewrite the upstream request body model from `auto` to the resolved real model.
9. Forward to the selected account.
10. Settle usage against the resolved real model's pricing.
11. Write usage log with requested/resolved model and route explanation.

Generic proxy paths should follow the same pattern where a model can be parsed. If no model exists in the request, continue using existing OpenAI-compatible account selection.

## Billing

AUTO billing always uses the resolved real model:

- Pre-deduction uses `RouteDecision.Model`.
- Settlement uses `RouteDecision.Model`.
- `balance_logs.reason_detail` should include `requested_model=auto resolved_model=<real>`.
- If the resolved model changes during fallback before first byte, the gateway must reverse the previous pre-deduction before attempting the new model, then check balance and pre-deduct for the new resolved model. A request can never settle against a model different from the currently held pre-deduction.
- If a fallback attempt fails before any billable upstream usage, it should not charge for the failed attempt beyond existing behavior.
- If fallback selects a more expensive model and the user's balance cannot cover its pre-deduction, that fallback candidate is rejected and routing continues to another valid candidate. If no affordable candidate remains, return insufficient balance rather than forwarding.
- A streaming request that receives `200 OK` but produces zero usage tokens settles as a zero-token charge unless the resolved model is configured as `per_request`; per-request models charge according to the existing per-request billing rule once the upstream call is accepted.

Do not allow fixed AUTO prices in P1. Fixed blended prices can be added later only after cost reports prove margins are stable.

## Fallback Rules

Fallback works in two levels:

1. Same resolved model, different `model_account`.
2. Different candidate model under the same AUTO policy.

Rules:

- Do not fallback after the first streaming byte has been flushed.
- Do not fallback for user-caused 4xx errors.
- Do not fallback to a model that violates token group, protocol, context, cost, capability, or policy constraints.
- `cheap` strategy does not upgrade to a more expensive model unless `allow_quality_upgrade` is true.
- Every fallback attempt appends to route reason and increments retry metrics.
- Rate limit accounting is acquired once for the first resolved model/account and is not reacquired per fallback attempt. This prevents fallback from multiplying a user's effective per-model quota. Account-level concurrent protection may still release the failed account and acquire the new account before the next upstream attempt.

## Observability

### Metrics

Add counters/histograms:

```text
traffic_auto_route_total{policy,requested_model,resolved_model,mode}
traffic_auto_route_filtered_total{policy,reason}
traffic_auto_route_fallback_total{policy,from_model,to_model,reason}
traffic_auto_route_score{policy,resolved_model}
```

Existing request, latency, inflight, circuit, and retry metrics continue using the resolved model/account.

### Logs

Add structured fields:

```text
requested_model
resolved_model
auto_route_policy_id
route_mode
route_score
route_reason
candidate_count
filtered_reason_counts
```

### Admin Debug View

Admin usage log detail should show:

- final selected model/account
- candidate count
- top rejected reasons
- score breakdown
- fallback attempts

## Admin API

Add model-management endpoints:

```text
GET    /admin/auto-routes
POST   /admin/auto-routes
GET    /admin/auto-routes/:id
PATCH  /admin/auto-routes/:id
DELETE /admin/auto-routes/:id
POST   /admin/auto-routes/:id/candidates
PATCH  /admin/auto-routes/:id/candidates/:candidateId
DELETE /admin/auto-routes/:id/candidates/:candidateId
POST   /admin/auto-routes/:id/test
```

`POST /admin/auto-routes/:id/test` accepts a sample request body and returns the route decision without forwarding upstream. This is important for admin confidence before publishing.

Dry run behavior:

- respects token group, protocol, context, capability, provider, and cost policy filters
- ignores user balance and does not pre-deduct
- does not consume rate limits or concurrency slots
- can run in either `live_health=true` mode, which respects circuit breaker state, or default preview mode, which reports circuit state but does not filter on it

## Frontend

Admin console adds an "AUTO routes" section:

- route list
- create/edit modal
- strategy selector
- candidate model picker
- rule editor with safe form fields, not raw JSON as the primary UI
- dry-run tester
- publish toggle

User console:

- `/v1/models` and pricing list show AUTO as a virtual model.
- Usage logs show `auto -> gpt-5.4-mini`.
- Pricing text says AUTO is charged by the final routed model.

## Migration and Rollout

1. Add schema migrations.
2. Add domain entities and repositories.
3. Add `SelectRoute` while preserving existing `SelectModelAccount`.
4. Convert chat completions path first.
5. Add usage log fields and display.
6. Add admin API and UI.
7. Add `/admin/auto-routes/:id/test`.
8. Enable AUTO only for an internal token group.
9. Monitor route decisions, cost, fallback, and error rate.
10. Publish to default token group after validation.

## Tests

### Unit Tests

- Non-virtual model follows existing routing.
- AUTO resolves to a candidate real model.
- Candidate filtering respects token group, protocol, active/listed state, context window, and circuit breaker.
- Strategy weights produce expected ordering.
- Cheap strategy rejects expensive upgrades unless allowed.
- Request body model is rewritten before forwarding.
- Billing estimate and settlement use resolved model.
- Cross-model fallback reverses old pre-deduction and pre-deducts with the new resolved model before forwarding.
- Fallback to a more expensive model fails cleanly when balance is insufficient.
- Fallback does not multiply per-model rate-limit quota.
- Usage log records requested/resolved model.
- Invalid capability tags and out-of-range scores are rejected by admin APIs.

### Integration Tests

- `model=auto` on `/v1/chat/completions` forwards with real model in upstream body.
- Streaming AUTO request does not fallback after first flush.
- Account failure before first byte falls back within policy.
- Cross-protocol fallback is rejected.
- `/v1/models` includes AUTO only when reachable candidates exist for token group.
- Admin dry run returns decision and reasons without upstream traffic.

## Risks and Mitigations

| Risk | Mitigation |
|---|---|
| AUTO hides expensive model choices | Always log and display resolved model; charge by resolved model; allow max cost rule |
| Wrong model for long context | Add `context_window_tokens` and hard filter |
| Debugging becomes opaque | Store route reason, score, policy, requested/resolved fields |
| Existing reports break | Keep `usage_logs.model` as resolved model and add new columns |
| Candidate pool misconfigured | Admin dry-run endpoint and publish toggle |
| Streaming fallback corrupts client response | Reuse existing "no retry after first byte" rule |
| AUTO model appears but cannot route | `/v1/models` lists virtual model only when at least one valid candidate is reachable |

## Open Decisions

- Whether `auto`, `auto-fast`, and `auto-coding` are separate virtual models or one `auto` plus optional header override. P1 should use separate virtual models because existing clients already understand model names.
- Whether to expose estimated price before call. This is not required for API compatibility but would improve user trust later.
- Whether candidate quality scores are global or token-group specific. P1 keeps them global; token-group-specific overrides can come later.

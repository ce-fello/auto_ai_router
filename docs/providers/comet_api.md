# Comet API Integration

## Scope

Comet API is integrated as a regular AIR provider for VseLLM, not as a special fallback proxy path.

The external user-facing API remains OpenAI-compatible. Comet-specific routing, error bodies, and provider details are internal to AIR.

Avito configuration is not changed.

## AIR Changes

### Provider Selection

- Comet credentials are expected to be configured with `is_fallback: false`.
- Direct Comet credentials participate in the normal provider pool.
- Runtime fallback remains proxy-oriented.
- Direct `is_fallback: true` provider fallback was removed from the Comet implementation path.

### Retry Classification

Retryable statuses:

- `408`
- `429`
- `5xx`, including `524`

Non-retryable statuses:

- `400`
- `401`
- `402`
- `403`
- `404`

Additional non-retryable response-body signals:

- `invalid_request`
- `invalid_request_error`
- `missing_required_parameter`
- `missing required parameter`
- `missing required field`
- `field messages is required`
- `field model is required`
- `request validation failed`
- `invalid json`
- `invalid request`
- `wrong type`
- `model_not_found`
- `model not found`
- `model does not exist`
- `unsupported model`
- `invalid token`
- `invalid api key`
- `api key is missing`
- `missing api key`
- `malformed api key`
- `access was blocked`
- `access blocked`
- `not allowed to use`
- `route is not allowed`
- `waf`
- `request entity too large`
- `payload too large`
- `content policy`
- `content management policy`
- `policy violation`

### Comet Error Masking

Comet credentials are detected internally by:

- `base_url` hostname equal to `cometapi.com`;
- `base_url` hostname ending with `.cometapi.com`;
- credential name containing `cometapi`;
- credential name containing `comet-api`.

For Comet credentials with upstream status `>= 400`:

- client response body is replaced with a generic OpenAI-compatible error envelope;
- raw Comet response body is not logged;
- logs include `response_body_masked=true`;
- request body debug logging remains enabled in AIR.

Generic masked error body:

```json
{
  "error": {
    "message": "Upstream provider error",
    "type": "<status-derived OpenAI error type>",
    "param": null,
    "code": "upstream_error"
  }
}
```

Special masked error codes:

- `429` -> `upstream_rate_limit`
- `408` -> `upstream_timeout`
- other upstream errors -> `upstream_error`

### Anthropic Cache Accounting

AIR maps Anthropic cache usage into OpenAI-style usage metadata:

- `cache_read_input_tokens` -> `prompt_tokens_details.cached_tokens`
- `cache_creation_input_tokens` -> `prompt_tokens_details.cache_creation_tokens`

These fields are included in spend metadata:

- `usage_object.prompt_tokens_details.cached_tokens`
- `usage_object.prompt_tokens_details.cache_creation_tokens`
- `additional_usage_values.prompt_tokens_details.cached_tokens`
- `additional_usage_values.prompt_tokens_details.cache_creation_tokens`

## Services Changes

Comet provider configuration was added for VseLLM AIR nodes:

- `ru01`
- `ru02`
- `pol01`
- `pol02`

Configured Comet credentials:

```yaml
- name: cometapi01-openai
  type: openai
  is_fallback: false
  weight: 1
  base_url: https://api.cometapi.com/v1

- name: cometapi01-anthropic
  type: anthropic
  is_fallback: false
  weight: 1
  base_url: https://api.cometapi.com
```

Vault key used by all configured nodes:

```text
COMETAPI01_KEY -> accounts/cometapi01.api_key
```

Existing active primary credentials on the changed AIR nodes were set to `weight: 20`.

Fallback proxy credentials were not converted into Comet credentials.

Comet model mappings were added only for chat/text models. Image and embedding models were not added.

## Automated Tests

Executed in `auto_ai_router`:

```bash
go test ./internal/proxy
go test ./...
git diff --check
```

Result:

- `go test ./internal/proxy` passed.
- `go test ./...` passed.
- `git diff --check` passed.

Covered cases:

- Comet OpenAI-compatible provider succeeds as a regular primary provider.
- Comet Anthropic provider uses `/v1/messages`.
- Comet Anthropic path sends Anthropic headers.
- Anthropic request conversion includes `cache_control: {"type":"ephemeral"}`.
- Retryable Comet errors retry to another same-type provider.
- Comet `invalid_request` is not retried.
- Comet `model_not_found` is not retried.
- Raw Comet errors are masked from client responses.
- Raw Comet errors are masked from logs.
- Strange Comet errors without the word `comet` are masked.
- Comet credential detection accepts Comet hosts and rejects lookalike hosts.
- Anthropic cache creation/read usage is extracted into spend metadata.

## Direct API Test Run

Date: `2026-06-24`

Compared providers:

- Comet API
- CheapGPT

OpenAI-like model:

- `gpt-5.4-mini`

Comet native Anthropic model:

- `claude-sonnet-4-5`

### HTTP Statuses

| Test | Status |
|---|---:|
| `comet-models` | `200` |
| `cheapgpt-models` | `200` |
| `comet-openai` | `200` |
| `cheapgpt-openai` | `200` |
| `comet-stream` | `200` |
| `cheapgpt-stream` | `200` |
| `comet-openai-claude-no-cache` | `200` |
| `comet-cache-1` | `200` |
| `comet-cache-2` | `200` |
| `comet-cache-3` | `200` |
| `comet-cache-4` | `200` |
| `comet-cache-5` | `200` |
| `comet-error-missing-messages` | `500` |
| `cheapgpt-error-missing-messages` | `400` |
| `comet-error-bad-model` | `503` |
| `cheapgpt-error-bad-model` | `404` |

### OpenAI-Like Response Shape

Top-level keys for both Comet and CheapGPT:

```text
choices, created, id, model, object, service_tier, system_fingerprint, usage
```

`choices[0].message` keys for both Comet and CheapGPT:

```text
annotations, content, refusal, role
```

`choices[0]` keys:

| Provider | Keys |
|---|---|
| Comet | `finish_reason`, `index`, `logprobs`, `message` |
| CheapGPT | `finish_reason`, `index`, `message` |

Observed difference:

- Comet includes `choices[0].logprobs`.
- CheapGPT does not include `choices[0].logprobs`.

### OpenAI-Like Usage

Comet `gpt-5.4-mini` usage:

```json
{
  "prompt_tokens": 31,
  "completion_tokens": 10,
  "total_tokens": 41,
  "prompt_tokens_details": {
    "audio_tokens": 0,
    "cached_tokens": 0
  }
}
```

CheapGPT `gpt-5.4-mini` usage:

```json
{
  "prompt_tokens": 48,
  "completion_tokens": 10,
  "total_tokens": 58,
  "prompt_tokens_details": {
    "audio_tokens": 0,
    "cached_tokens": 0
  }
}
```

Observed differences:

- Comet and CheapGPT token counts differ for the same prompt.
- Comet usage includes `latency_checkpoint`.
- CheapGPT usage does not include `latency_checkpoint`.

### Streaming

Both providers returned Server-Sent Events and ended with:

```text
data: [DONE]
```

Observed Comet stream differences:

- Comet chunks include `logprobs:null`.
- Comet chunks include `obfuscation`.
- Comet returned a final chunk with `usage`.

Observed CheapGPT stream differences:

- CheapGPT chunks do not include `logprobs`.
- CheapGPT chunks include `obfuscation:"cg"`.
- CheapGPT did not return a final chunk with `usage` in the captured stream head.

### Error Responses

Missing `messages` request:

Comet:

```json
{
  "error": {
    "message": "field messages is required (request id: ...)",
    "type": "comet_api_error",
    "param": "",
    "code": "invalid_request"
  }
}
```

CheapGPT:

```json
{
  "error": {
    "code": "missing_required_parameter",
    "message": "Missing required parameter: 'messages'.",
    "param": "messages",
    "type": "invalid_request_error"
  }
}
```

Observed differences:

- Comet returned HTTP `500`.
- CheapGPT returned HTTP `400`.
- Comet returned provider-specific `type: comet_api_error`.
- Comet error message included a provider request id.

Bad model request:

Comet:

```json
{
  "error": {
    "code": "model_not_found",
    "message": "no available channel for group default and model definitely-not-a-real-model-20260624 (distributor) (request id: ...)",
    "type": "comet_api_error"
  }
}
```

CheapGPT:

```json
{
  "error": {
    "code": "model_not_found",
    "message": "The model `definitely-not-a-real-model-20260624` does not exist or you do not have access to it.",
    "param": null,
    "type": "invalid_request_error"
  }
}
```

Observed differences:

- Comet returned HTTP `503`.
- CheapGPT returned HTTP `404`.
- Comet returned provider-specific `type: comet_api_error`.
- Comet message exposed internal routing terms: `channel`, `group`, `distributor`.

### Cache Results

OpenAI-compatible Claude request through Comet:

```json
{
  "prompt_tokens": 8669,
  "completion_tokens": 20,
  "total_tokens": 8689,
  "prompt_tokens_details": {
    "cached_tokens": 0
  },
  "claude_cache_creation_5_m_tokens": 0,
  "claude_cache_creation_1_h_tokens": 0
}
```

Observed result:

- OpenAI-compatible `/v1/chat/completions` did not expose prompt cache creation or read.

Native Anthropic `/v1/messages` through Comet:

| Request | `input_tokens` | `cache_creation_input_tokens` | `cache_read_input_tokens` | `output_tokens` |
|---|---:|---:|---:|---:|
| `comet-cache-1` | `2091` | `0` | `11447` | `9` |
| `comet-cache-2` | `15` | `18003` | `0` | `9` |
| `comet-cache-3` | `15` | `18003` | `0` | `9` |
| `comet-cache-4` | `15` | `18003` | `0` | `9` |
| `comet-cache-5` | `15` | `0` | `18003` | `9` |

Observed results:

- Native Anthropic `/v1/messages` returned cache usage fields.
- Cache creation was observed.
- Cache read was observed.
- Cache hit behavior was not consistent across all repeated requests.

## Compatibility Summary

OpenAI-compatible happy path:

- Comet response shape is compatible with the tested OpenAI-like flow.
- Comet response is not byte-for-byte identical to CheapGPT.

OpenAI-compatible differences observed:

- extra `logprobs` field in Comet `choices[0]`;
- extra `latency_checkpoint` in Comet `usage`;
- extra `usage` final chunk in Comet streaming response;
- Comet returned snapshot model id `gpt-5.4-mini-2026-03-17`, while CheapGPT returned alias `gpt-5.4-mini`;
- Comet error status codes differ from CheapGPT for malformed requests and missing models;
- Comet error body exposes provider-specific fields and internal routing terms without AIR masking.

Cache:

- Prompt cache is available through Comet native Anthropic `/v1/messages`.
- Prompt cache was not observed through Comet OpenAI-compatible `/v1/chat/completions`.

AIR masking:

- Required for Comet provider errors.
- Implemented for Comet credentials.

# Comet API Services Configuration

This document describes the infrastructure configuration that must be applied in the `services` repository for the Comet API AIR integration.

The AIR runtime implementation is documented in `docs/providers/comet_api.md`.

## Goal

Add Comet API as a regular direct provider for VseLLM AIR nodes.

Comet is not configured as a fallback proxy. Comet credentials must use `is_fallback: false`.

Avito configuration is not changed.

## Target AIR Nodes

Apply the configuration to these VseLLM AIR nodes:

- `apps/air/ru01`
- `apps/air/ru02`
- `apps/air/pol01`
- `apps/air/pol02`

## Shared Comet File

Add a shared file:

```text
apps/air/comet.libsonnet
```

Contents:

```jsonnet
{
  credentials:: [
    {
      name: 'cometapi01-openai',
      type: 'openai',
      is_fallback: false,
      weight: 1,
      api_key: 'os.environ/COMETAPI01_KEY',
      base_url: 'https://api.cometapi.com/v1',
      rpm: -1,
      tpm: -1,
    },
    {
      name: 'cometapi01-anthropic',
      type: 'anthropic',
      is_fallback: false,
      weight: 1,
      api_key: 'os.environ/COMETAPI01_KEY',
      base_url: 'https://api.cometapi.com',
      rpm: -1,
      tpm: -1,
    },
  ],

  openAIChatModels:: [
    { name: 'gpt-4.1' },
    { name: 'gpt-4.1-mini' },
    { name: 'gpt-4.1-nano' },
    { name: 'gpt-4o' },
    { name: 'gpt-4o-mini' },
    { name: 'gpt-5' },
    { name: 'gpt-5-chat' },
    { name: 'gpt-5-mini' },
    { name: 'gpt-5-nano' },
    { name: 'gpt-5.1' },
    { name: 'gpt-5.2' },
    { name: 'gpt-5.3-codex' },
    { name: 'gpt-5.4' },
    { name: 'gpt-5.4-mini' },
    { name: 'gpt-5.4-nano' },
    { name: 'gpt-5.4-pro' },
    { name: 'gpt-5.5' },
    { name: 'gpt-oss-120b' },
    { name: 'gpt-oss-20b' },
    { name: 'gemini-3-flash-preview' },
    { name: 'gemini-3.1-flash-lite' },
    { name: 'gemini-3.1-pro-preview' },
    { name: 'gemini-3.5-flash' },
    { name: 'qwen3-235b-a22b' },
    { name: 'qwen3-max-2026-01-23' },
    { name: 'qwen3-vl-235b-a22b-thinking' },
    { name: 'qwen3.5-flash' },
    { name: 'qwen3.5-plus' },
    { name: 'qwen3.6-plus' },
    { name: 'qwen3.7-max' },
    { name: 'deepseek-v3.2' },
    { name: 'deepseek-v4-flash' },
    { name: 'deepseek-v4-pro' },
    { name: 'glm-5' },
    { name: 'glm-5.1' },
    { name: 'kimi-k2.5' },
    { name: 'kimi-k2.6' },
  ],

  anthropicChatModels:: [
    { name: 'claude-haiku-4.5', model: 'claude-haiku-4-5-20251001' },
    { name: 'claude-opus-4.6', model: 'claude-opus-4-6' },
    { name: 'claude-opus-4.7', model: 'claude-opus-4-7' },
    { name: 'claude-sonnet-4.5', model: 'claude-sonnet-4-5' },
    { name: 'claude-sonnet-4.6', model: 'claude-sonnet-4-6' },
  ],

  chatAliasModels:: $.openAIChatModels + std.map(
    function(m) { name: m.name },
    $.anthropicChatModels
  ),

  modelGroups:: [
    {
      credentials: ['cometapi01-openai'],
      models: $.openAIChatModels,
    },
    {
      credentials: ['cometapi01-anthropic'],
      models: $.anthropicChatModels,
    },
  ],
}
```

## Credential Configuration

Each target node imports the shared file:

```jsonnet
local comet = import '../comet.libsonnet';
```

Each target node appends Comet credentials to its credential list:

```jsonnet
... + comet.credentials
```

### Weights

Set active primary credentials on the target nodes to:

```jsonnet
weight: 20
```

Keep Comet credentials at:

```jsonnet
weight: 1
```

Do not change existing fallback proxy credentials only for this task.

## Vault Key

Add the same Comet key to every target node:

```jsonnet
COMETAPI01_KEY: { store: 'accounts', path: 'cometapi01', property: 'api_key' }
```

Files:

- `apps/air/ru01/main.jsonnet`
- `apps/air/ru02/main.jsonnet`
- `apps/air/pol01/main.jsonnet`
- `apps/air/pol02/main.jsonnet`

## Model Mapping Rules

OpenAI-compatible chat/text models use:

```text
cometapi01-openai
```

Anthropic/Claude models use:

```text
cometapi01-anthropic
```

Do not map image generation models.

Do not map embedding models.

## Node-Specific Model Groups

### `ru01`

File:

```text
apps/air/ru01/models.libsonnet
```

Add Comet import:

```jsonnet
local comet = import '../comet.libsonnet';
```

Keep existing Yandex and Cloud.ru mappings.

Add a primary proxy alias group:

```jsonnet
{
  credentials: ['pol01', 'usa02', 'usa03', 'fra01'],
  models: comet.chatAliasModels,
}
```

Append Comet direct mappings:

```jsonnet
] + comet.modelGroups
```

### `ru02`

File:

```text
apps/air/ru02/models.libsonnet
```

Create the file if it does not exist:

```jsonnet
local comet = import '../comet.libsonnet';

[
  {
    credentials: ['pol01', 'usa01', 'usa03', 'fra01'],
    models: comet.chatAliasModels,
  },
] + comet.modelGroups
```

Also add models rendering in:

```text
apps/air/ru02/main.jsonnet
```

```jsonnet
models: gw.fromGroups(import './models.libsonnet'),
```

### `pol01`

File:

```text
apps/air/pol01/models.libsonnet
```

Use:

```jsonnet
local comet = import '../comet.libsonnet';

[
  {
    credentials: ['pol02'],
    models: comet.chatAliasModels,
  },
] + comet.modelGroups
```

### `pol02`

File:

```text
apps/air/pol02/models.libsonnet
```

Add Comet import:

```jsonnet
local comet = import '../comet.libsonnet';
```

Keep existing Yandex, Cloud.ru, and CheapGPT mappings.

Append:

```jsonnet
] + comet.modelGroups
```

## Expected Credential Changes By Node

### `ru01`

Active primary credentials set to `weight: 20`:

- `pol01`
- `usa02`
- `usa03`
- `fra01`
- `roma-cloudru-1`
- `ac01-yandex-01`

Append:

```jsonnet
proxyGateways + yandex + cloudru + comet.credentials
```

### `ru02`

Active primary credentials set to `weight: 20`:

- `pol01`
- `usa01`
- `usa03`
- `fra01`

Append:

```jsonnet
proxyGateways + comet.credentials
```

### `pol01`

Active primary credentials set to `weight: 20`:

- `pol02`

Append:

```jsonnet
proxyGateways + comet.credentials
```

Remove the old single inline Comet test credential if present:

```jsonnet
name: 'cometapi01'
```

### `pol02`

Active primary credentials set to `weight: 20`:

- `test-01`
- `roma-cloudru-1`
- `ac01-yandex-01`

Append:

```jsonnet
proxyGateways + yandex + cloudru + comet.credentials
```

## Model Aliases Intentionally Not Added

The following aliases were not mapped because they were not confirmed in the Comet catalog used during implementation:

- `gpt-5.1-chat`
- `gpt-5.2-chat`
- `gpt-5.2-codex`
- `gpt-5.3-chat`
- `gemini-2.5-flash`
- `gemini-2.5-pro`
- `gemini-3.1-pro-preview-customtools`
- `claude-opus-4.1`
- `claude-opus-4.5`
- `claude-sonnet-4`
- `qwen3-coder-next`
- `qwen3-vl-235b-a22b-instruct`
- `qwen3-vl-30b-a3b-instruct`
- `qwen3-vl-30b-a3b-thinking`
- `qwen3-vl-8b-instruct`
- `qwen3-vl-8b-thinking`
- `qwen3-vl-flash`
- `qwen3-vl-plus`
- `qwen3.6-35b-a3b`
- `qwen3.6-flash`
- `qwen3.6-max-preview`
- `deepseek-v3-0324`
- `deepseek-r1-distill-llama-70b`
- `deepseek-v3.2-speciale`
- `glm-4.5-air`
- `glm-4.6`
- `glm-4.6v`
- `glm-4.6v-flash`
- `glm-4.6v-flashx`
- `glm-4.7`
- `glm-4.7-flash`
- `mimo-v2-flash`
- `mimo-v2.5`
- `mimo-v2.5-pro`
- `kimi-k2-0905`
- `yandexgpt-5-lite`
- `yandexgpt-5-pro`
- `yandexgpt-5.1`
- `llama-3.3-70b-instruct`

Re-check the current Comet catalog before adding any skipped alias.

Catalog endpoint:

```text
https://api.cometapi.com/api/models
```

## Validation

Required render checks:

- render `apps/air/ru01/main.jsonnet`
- render `apps/air/ru02/main.jsonnet`
- render `apps/air/pol01/main.jsonnet`
- render `apps/air/pol02/main.jsonnet`

Static checks:

- every target node imports `../comet.libsonnet`;
- every target node appends `comet.credentials`;
- every target node has `COMETAPI01_KEY`;
- no Comet credential uses `is_fallback: true`;
- no Comet credential uses `fallback`;
- no Comet credential uses `api_base`;
- `cometapi01-openai` uses `base_url: 'https://api.cometapi.com/v1'`;
- `cometapi01-anthropic` uses `base_url: 'https://api.cometapi.com'`;
- Comet credentials use `weight: 1`;
- active primary credentials use `weight: 20`;
- fallback proxy credentials are not changed as part of this task.

Runtime checks after deploy:

- AIR health/model endpoints expose expected chat aliases;
- `gpt-5.4-mini` can be served through Comet OpenAI-compatible path;
- Claude aliases can be served through Comet Anthropic path;
- Comet provider errors are masked by AIR after deploying the matching `auto_ai_router` changes.

## Local Validation Performed During Implementation

The following checks were performed locally:

```bash
git diff --check
```

Static Node.js checks verified:

- `ru01` has Comet credentials, vault key, and model groups;
- `ru02` has Comet credentials, vault key, and model groups;
- `pol01` has Comet credentials, vault key, and model groups;
- `pol02` has Comet credentials, vault key, and model groups.

Full Jsonnet rendering was not run locally because `jsonnet` and `jb` were not available in the local environment.

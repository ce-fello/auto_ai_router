# Sosana.art

Sosana.art is supported for the OpenAI-compatible Images API and, for the
`-compliant` Gemini chat models, for Chat Completions.

For images the router accepts `/v1/images/generations` and `/v1/images/edits`,
submits a Sosana Banana async task, polls it, and returns an OpenAI Images
response with `data[].b64_json`.

For text the router forwards `/v1/chat/completions` to Sosana's own
OpenAI-compatible endpoint (`/api/chat/completions`), streaming and
non-streaming alike. Only `gemini-flash-compliant` and `gemini-pro-compliant`
are routed there — see [Chat Completions](#chat-completions).

Responses API, Embeddings, video, and slides are not routed to Sosana in this
integration.

## Configuration

```yaml
credentials:
  - name: "sosana_images"
    type: "sosana"
    api_key: "os.environ/SOSANA_API_KEY"
    base_url: "https://sosana.art"
    rpm: 60
    tpm: -1

models:
  - name: "google/gemini-3.1-flash-image-preview"
    model: "banana-2-{image_size}-compliant"
    credential: sosana_images
    rpm: 60
    tpm: -1
```

The credential value is configured as `api_key`. The router sends it to Sosana
as `Authorization: Bearer <api_key>`, matching Sosana's API contract.

The dynamic model template maps `image_size` to Sosana's concrete image models:
`banana-2-1k-compliant`, `banana-2-2k-compliant`, and
`banana-2-4k-compliant`. If `image_size` is omitted, the router uses `1K`.
This integration maps Sosana only for `google/gemini-3.1-flash-image-preview`;
other image families should be served by their native providers or fallback
proxies.

Sosana Banana tasks are asynchronous and can take longer than short chat
completion requests. For production Sosana credentials, set the router
`request_timeout` and HTTP `write_timeout` to at least `2m`.

Chat models are configured the same way, against the same or a separate
credential:

```yaml
models:
  - name: "sosana/gemini-flash-compliant"
    model: "gemini-flash-compliant"
    credential: sosana_images
    rpm: 100
    tpm: 100000

  - name: "sosana/gemini-pro-compliant"
    model: "gemini-pro-compliant"
    credential: sosana_images
    rpm: 100
    tpm: 100000
```

One credential serves both APIs — the router picks the flow from the request
path, so a chat model and an image model may share `credential: sosana_images`.

## Chat Completions

Sosana serves two chat models, and this integration routes only those:

| Model                    | Upstream                                  |
| ------------------------ | ----------------------------------------- |
| `gemini-flash-compliant` | Google Gemini Flash, via Sosana's own accounts |
| `gemini-pro-compliant`   | Google Gemini Pro, via Sosana's own accounts   |

They are the compliant variants of Sosana's `gemini-flash` / `gemini-pro`
aliases: a request is served by Google Gemini itself and is never handed to a
third-party provider. That guarantee is also the limitation — everything Sosana
would need a fallback provider for is refused upstream with a `400`:

- `tools`, `tool_choice`, `functions` / `function_call` — no tool calling.
- `response_format` — no structured output.
- `max_tokens` / `max_completion_tokens` — no output cap.
- more than 100 000 characters of message text (attachments are not counted).
- `n` must be `1`.

The router screens those requests off the Sosana credential before sending
anything, the same way it does for incompatible image requests: another primary
credential for the same model is tried, then the fallback proxy cascade, and
only if nothing can serve it does the router return a local `400`. Sosana never
sees a request it would reject.

Other Chat Completions parameters (`temperature`, `top_p`, `stop`, `seed`,
`stream`, `stream_options`, …) are forwarded unchanged. Note that the compliant
models do not honour the sampling parameters — send them if your client always
does, but do not expect `temperature` to change the output.

Streaming works normally: Sosana returns OpenAI Chat Completions SSE, which the
router forwards as-is. Token usage is read from the response `usage` object, and
from the streamed usage chunk when the client asks for it via
`stream_options.include_usage`.

Chat spend is calculated from the normal per-token price entries
(`input_cost_per_token` / `output_cost_per_token`) for the model, from the
internal price registry or the LiteLLM model table — there is no image-style
per-request price for text.

## Behavior

- `n` must be `1`.
- Requests selected to a Sosana credential are skipped when they require
  controls that Sosana does not support. The router then tries another primary
  credential for the same model and then the configured fallback proxy cascade.
  If no compatible provider is available, the router returns a local 400.
- Default response format and `response_format: "b64_json"` return
  `data[].b64_json`.
- `response_format: "url"` is not routed to Sosana because URL responses require
  VSELLM-owned rehosting before they can hide Sosana storage. Another provider
  may handle it through normal fallback routing.
- `image_size` may be omitted or set to `1K`, `2K`, or `4K`; `0.5K` is not
  routed to Sosana. Pixel `size` values are accepted only when they match the
  documented Gemini `image_size` + `aspect_ratio` table for `1K`, `2K`, or
  `4K`.
- `/v1/images/edits` accepts PNG input images only, up to 14 files, and sends
  them as `data:image/png;base64,...` values in Sosana `image_urls`.
- Mask images are not supported.
- Output is PNG. `output_format` may be omitted or set to `png`; other formats
  are not routed to Sosana.
- The router sends `prompt_optimization: false` so Sosana returns `MODERATED`
  instead of rewriting moderated prompts into safe alternatives.

On completion, Sosana returns a public object URL in `result_file_url`. The
router downloads that object only from allowed Sosana/CDN hosts, does not follow
redirects, does not forward the Sosana `Authorization` header, keeps the download
bounded to 32 MiB, verifies the body is PNG, and base64-encodes it into the
OpenAI-compatible JSON response. The upstream object URL is not returned to
clients.

## Billing

Sosana vendor prices are not used at request time and are not returned to
clients. Successful image requests log `ImageCount=1`; spend is calculated from
the internal price registry or LiteLLM model table using `output_cost_per_image`.
When `image_size` selects a concrete tier, the spend lookup uses the concrete
model first, for example `banana-2-2k-compliant`, and then falls back to the
public model name if no concrete price is configured.

## Error Masking

Sosana upstream HTTP errors and terminal task errors are masked before they are
returned to clients. The router preserves the appropriate HTTP status but
replaces provider details with neutral OpenAI-compatible error bodies.

For operator debugging, structured logs may include a truncated textual upstream
error body with `response_body_masked=true`. Raw image bytes and full result
URLs are not logged.

If Sosana is hidden behind another proxy credential, the upstream router should
propagate the credential marker used by this router so proxy-chain errors can be
masked as Sosana errors too.

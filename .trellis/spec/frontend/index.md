# Frontend Development Guidelines

> Executable frontend contract for this Go SmsForwarder webhook project.

---

## Overview

This project intentionally uses **Pico CSS + Go `html/template` server-side rendering** for a simple message-viewing UI. Do not introduce a Node.js build chain unless a later requirement explicitly needs client-side interactivity that cannot be handled with semantic HTML and server-side handlers.

The current frontend is the `/messages` page implemented by:

```text
internal/webhook/ui.go
internal/webhook/templates/messages.html
```

---

## Frontend Framework Decision

| Decision    | Contract                                                                                                       |
| ----------- | -------------------------------------------------------------------------------------------------------------- |
| Framework   | Pico CSS from `https://picocss.com/`                                                                           |
| Rendering   | Go `html/template` with embedded template files                                                                |
| Route       | `GET /messages`                                                                                                |
| API link    | Generate `/api/messages` URLs in Go via `messagesAPIPath(query)` using `url.Values`, then render `{{.APIURL}}` |
| Forms       | Use semantic HTML `GET` filters for source and search only                                                     |
| Build chain | No npm/Vite/React dependency for this project                                                                  |

Why: Pico CSS is a minimal semantic HTML CSS framework, matches the small Go service shape, and keeps deployment simple in Docker/1Panel.

---

## Page Contract

`GET /messages` must render:

- Pico CSS stylesheet link: `https://cdn.jsdelivr.net/npm/@picocss/pico@2/css/pico.min.css`
- Page title: `消息列表`
- Stats cards for total count and source count
- Filters for `source` and search `q`
- A JSON link to the equivalent `/api/messages` query
- Message cards showing backend-extracted fields: `conversationTitle`, source, `cleanContent`, original title, sender name, device, and receive time

The page displays backend-owned extracted fields only. It must not reimplement QQ cleaning or content-feature detection in the template.

---

## Good / Base / Bad Cases

### Good Case: Encoded JSON Link

Request:

```text
GET /messages?source=qq&q=Claude+API
```

Expected HTML contains an encoded API link equivalent to:

```text
/api/messages?q=Claude&#43;API&amp;source=qq
```

This prevents spaces, Chinese text, and special characters from corrupting the JSON link.

### Base Case: Empty Result

When the backend returns no messages, render an empty state with text telling the user to loosen filters.

### Bad Case: Template Recomputes Extracted Fields

Do not add keyword checks or QQ author-splitting logic in HTML. Extraction belongs in `internal/webhook/enrichment.go` and persisted database fields.

---

## Tests Required

Frontend-related tests live in `internal/webhook/handler_test.go` and must cover:

- `/messages` returns `text/html`
- HTML contains Pico CSS, `消息列表`, clean content, source/search filters, extracted fields, and the empty-state copy
- HTML contains the URL-encoded `/api/messages` link produced by Go, not manual template interpolation
- The page does not expose write-action controls; it is a simple message viewer

Before claiming frontend completion, also run a local runtime check:

```bash
curl -i 'http://127.0.0.1:18088/messages' | sed -n '1,80p'
curl 'http://127.0.0.1:18088/api/messages?source=qq&limit=5'
```

---

## Guidelines Index

| Guide                                             | Description                            | Status  |
| ------------------------------------------------- | -------------------------------------- | ------- |
| [Directory Structure](./directory-structure.md)   | Module organization and file layout    | To fill |
| [Component Guidelines](./component-guidelines.md) | Component patterns, props, composition | To fill |
| [Quality Guidelines](./quality-guidelines.md)     | Code standards, forbidden patterns     | To fill |

---

**Language**: All documentation should be written in **English**.

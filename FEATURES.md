# Posting parity and Postman interoperability

This is a working acceptance ledger, not a completion claim. The target remains
Posting 3's full user-facing functionality with Postman CLI execution and Native
Git files shared with Desktop Local View. MCP is optional. The copied legacy
`cmd/posting` path does not count as Postman integration evidence.

Status: **partial** means some native behavior is verified; **pending** means
Postman-path acceptance is still missing, even when upstream UI/tests exist.
Every row requires the separate `cmd/herdr-postman` startup path and real TUI
verification before final acceptance. CLI tests use disposable local fixtures.

| Capability / source | Status | Evidence now | Remaining acceptance |
|---|---|---|---|
| Independent app / `cmd/herdr-postman`, `internal/paths` | partial | Native store/runner startup tests; terminal startup; user theme loading and Nerd Font setting tests; custom background verified in actual PTY; browser opener wired | Remaining branding and startup flags; theme reload; original app unchanged |
| Native request load/create/save/delete / `collection/native.go` | partial | Native lifecycle, CLI lint/read-back, unknown field preservation tests | Complete supported fields; actual Desktop read/edit round trip |
| File watching / `ui/watch.go` | partial | Real file changes update clean tabs; dirty/conflicting drafts protected; invalid YAML retained; close stops watcher; actual Desktop save while a PTY TUI draft is dirty retains the dirty tab and rejects its save without changing the GUI-written file | Deleted/renamed open tabs |
| Request name, URL, method, description / `ui/session.go` | partial | Native editor save and draft rendering tests | Actual interactive edit/save/reopen in both apps |
| Headers, query and path rows / `ui/kv_editor.go` | partial | Disabled rows and descriptions retained; CLI request test; Desktop compact header mappings load and can be edited into sequence rows without losing duplicate/disabled entries | Row rename metadata; broader GUI parameter and variable syntax round trip |
| Raw body / `ui/request_view.go` | partial | JSON/text native save; real CLI unsaved POST | All raw MIME modes, empty and binary edge cases |
| Form, multipart, file body / `ui/multipart.go`, `upload_path.go` | partial | URLencoded, multipart and binary file mapping; CLI lint and actual bytes, disabled/duplicate rows; native upload root; file MIME editing; GUI multi-file arrays survive editing and actual CLI uploads; variable-based upload paths tested against CLI, preview and input completion | Multipart filename overrides, interactive picker and GUI round trip |
| Auth none/basic/digest/bearer/OAuth2 / `ui/oauth.go`, `model/request.go` | partial | Basic/Bearer/Digest editing and real CLI execution; inherit versus none tested; GUI-managed auth clearly labeled and preserved | OAuth flow; actual interactive credential editing and GUI round trip |
| TLS, redirect, timeout and other request options / `model/request.go` | partial | Native TLS/redirect preservation tests; cookie checkbox loads/saves Native disableCookies without dropping advanced GUI settings | Remaining options and observable CLI behavior |
| New unsaved request execution / `client/postman_collection.go` | partial | Real CLI POST returns response without source file creation | Interactive send/cancel/error; inherited settings and environments |
| Saved request execution / `client/postman_collection.go` | partial | Draft edits, collection auth/scripts, binary response via actual CLI; ID-based selection handles renamed nested folders, duplicate display names and slash-containing names; existing script-visible ID/name retained; native copies execute with independent IDs; duplicate stored IDs rejected before sending | Scripting control flow; concurrent sends |
| Native YAML export / `ui/commands.go` | partial | Saved/unsaved draft exports preserve GUI fields without source writes | Actual GUI import/read-back and clipboard interaction |
| Duplicate, rename, move, bulk collection operations / `ui/commands.go`, `sidebar.go` | partial | Native duplicate preserves draft edits and GUI-managed fields; independent item ID and example resources; collision/conflict/path checks; CLI lint/read-back and separate original/copy execution | Rename/move and bulk acceptance; actual Desktop and interactive duplicate round trip |
| Collection search, tree selection and URL completion / `ui/tree_search.go`, `completion.go` | pending | Upstream UI tests | Native collections, folders and variables in real TUI |
| Tabs, previews, keyboard navigation, undo / `ui/session_tabs.go`, `session.go` | partial | Copied regression suite; native save integration | Real native multi-tab edit/send/save lifecycle |
| Environments and variables / `ui/environments.go` | partial | Native environment discovery/selection and repeatable startup flags; actual PTY launch with two layers proves override and retained base values reach CLI without source edits; disabled entries excluded; session overrides; private CLI environment snapshot overrides collection defaults; live reload retains valid values; selection paths remembered per collection; native double-brace completion/highlighting and URL preview; root collection variables shown inside folders without runtime flattening | Environment editing; dynamic/script-created variables; complete Postman scope precedence |
| Scripts, assertions and captures / `ui/session.go`, `client/postman_collection.go` | partial | Native scripts preserved/executed; failing assertions retain HTTP response; Posting 3 keeps legacy script paths but exposes no script editor | Posting capture behavior equivalent in Postman; interactive GUI-script preservation; no Python fallback |
| Response status, headers, body and time / `ui/response_view.go` | partial | Actual CLI response and binary body tests | Interactive response display; cookies and protocol metadata |
| Run result display / `ui/response_view.go` | partial | Run passed/failed tab rendering and assertion failure tests | Interactive keyboard access, long output and cancellation |
| JSON tree, highlighting, search and copy / `ui/response_json.go`, `response_body_search.go` | pending | Copied regression suite | Real CLI response rendered and searched interactively |
| Images, response save and external pager/editor / `ui/response_image.go`, `response_save.go`, `external.go` | pending | Copied UI only | Real CLI data, paths, binary fidelity and terminal behavior |
| Cookies, trace and streaming response / `ui/response_view.go`, `response_messages.go` | partial | Real CLI Set-Cookie parsing (multiple headers, expiry, security flags); app send through CLI to rendered Cookies table; actual CLI cookie reuse, disabled attachment and instance isolation; concurrent delta merge tests; saved Native disableCookies option round trip and actual CLI redirect-cookie suppression, with jar reuse restored when enabled; Desktop Disable cookie jar toggle confirmed to read/write the same field | Full TUI/Desktop checkbox round trip; trace and streaming capture contracts and transport evidence |
| History and persistence / `ui/history.go` | partial | Actual CLI send, disk persistence and reopened Run rendering; binary bytes, cookies and assertion failure retained; Run output counted in 50 MiB payload budget | Native replay must preserve GUI-managed request fields and folder context; interactive history acceptance |
| curl import/export / `ui/curl.go` | pending | Copied conversions | Native auth/variables/files parity without silent field loss |
| Collection import / `internal/importing` | pending | Copied format importers | Native output; resources preserved; imported requests execute |
| GraphQL / `ui/kind_graphql.go` | partial | Posting HTTP lowering stored as Native http-request; structured editor round trip; actual CLI query/variables/operationName and GraphQL error status; HTTP file lint; real PTY remote GraphQL send; Desktop 12.34.0 edits query/variables/operationName in HTTP JSON body and the running TUI reloads them as GraphQL, including GUI-compacted headers | Completion and multi-operation interactive acceptance; existing Native graphql-request documents remain unsupported |
| WebSocket / `ui/kind_websocket.go` | pending | CLI 1.82.0 accepts a Native websocket-request in lint, but collection run skips it as unsupported (ws-raw-request), reports 0 requests, and exits 0 | User decision on a Posting-engine exception; native mapping and streaming UI. A zero CLI exit code is not execution evidence |
| gRPC / `ui/kind_grpc.go` | pending | Copied editor only; official collection-run documentation requires a paid plan for gRPC | Current-plan supported route; protos/reflection, unary and streaming |
| Remote control / `ui/remote.go`, `internal/remote` | partial | Separate executable and private socket wiring; Native open/show/save-as preserves GUI metadata; real PTY + remote CLI + Postman CLI local HTTP send; shutdown removes socket; real CLI assertion failure retained in remote JSON and text diagnostics | Native YAML import, cross-folder save-as, remote environment and advanced payload parity |
| Herdr pane/focus/hide and installation package | partial | Separate manifest/scripts; 11 fixture lifecycle/launcher tests; F8 plugin linked and registered opening/return verified; in-app select-box PTY test; original herdr-posting preserved | Broader platform verification; existing panes need reopening for the new binary |
| Desktop Local View interoperability | partial | Desktop 12.34.0 Local Items opens a TUI-created Native HTTP request; GUI URL and GraphQL body saves reload in the running TUI; Desktop save while the TUI has unsaved URL edits leaves its tab dirty and subsequent TUI save fails without overwriting the GUI file | Broader payload/auth/script round trips; conflict recovery workflow |
| Postman workbench layout / `ui/postman_layout.go` | partial | Flat Postman-inspired dark palette; collection sidebar; name/Save above URL/Send; Postman request tab order; response below; variable/layout/command footer; rendered preview and Save-button persistence test | Full GUI parity, advanced script/collaboration/AI panels, and broader narrow-terminal acceptance |
| Cloud and Local source selection / `internal/cloud`, `cmd/herdr-postman` | partial | In-app Local/Cloud and workspace select boxes; private last-successful-source persistence and actual PTY restart restoration for Cloud and Local; per-source sidebar expansion restoration by stable folder identity; unavailable-workspace fallback; all workspace collections in sidebar with isolated routing; dirty/running-tab switch guard; asynchronous loading with failure recovery, Escape cancellation and 60-second deadline; collection-only startup reads (real account: 5 collections/180 requests in 10.6s); deferred request metadata with save conflict checks; real PTY select-box switching; live read-only canonical load of 4 requests; Cloud Store saves scalar fields and JSON body through CLI; editor repeated-save and GUI-conflict draft-preservation tests | Cloud create/delete, other editable fields, environment selection, live refresh and remote control; disposable real-cloud write/GUI read-back verification; preflight conflict check is not atomic |
| Optional MCP and account integration | partial | CLI account authenticated and real Cloud catalog read verified; no MCP dependency | Optional setup documentation and current-plan capability presentation |

## Release gate

No complete-parity claim until every row has Postman-path evidence and actual TUI
acceptance. Unsupported CLI operations require a documented supported solution
or an explicit user decision; copied legacy engines are not an implicit fallback.
Commit, push, plugin installation/configuration and deployment remain separately
approved actions.

Protocol contract check (CLI 1.82.0, 2026-10-29): a disposable Native
`websocket-request` targeting loopback passed `collection lint`. Running it with
`collection run --timeout 1000 --no-report-events` printed `Skipping 1 unsupported
item(s)` for `ws-raw-request`, executed zero requests and exited successfully.
No WebSocket fallback has been enabled. The
[official collection command reference](https://learning.postman.com/docs/postman-cli/postman-cli-collections/)
requires a paid plan for gRPC execution; merely having a Native schema or editor
does not establish an available CLI transport.

Native v3 folder objects do not define a `variables` property. CLI 1.82.0
confirmed that root collection variables remain active inside folders and an
extra folder-only variable is not substituted. The editor therefore offers
root collection variables and does not invent a folder variable scope. See the
[official v3 schema](https://schema.postman.com/collection/json/v3.0.0/draft-07/collection.json).

Baseline clarification: the inspected Posting 3 snapshot has duplicate/delete
commands and initial-save folder selection, but no explicit request file rename
or move command. Filesystem rename/move interoperability remains to be checked
with Desktop; it is not evidence of a missing copied Posting UI command.

The upstream Posting 3 `internal/ui/session.go` explicitly keeps legacy script
paths without editing them, matching `internal/model/request.go`'s statement
that Posting 3 does not execute those Python scripts. A new JavaScript editor is
therefore not a Posting-parity gate. Native GUI scripts still must be preserved
and executed by Postman; capture behavior remains an independent requirement.

## GraphQL representation

GraphQL support uses Posting's existing HTTP POST lowering: query, variables,
and operationName are stored in the JSON body of a Native `http-request`.
Desktop therefore opens the same file as HTTP. The TUI recognizes a lossless
GraphQL envelope when its enabled Accept header explicitly contains
`application/graphql-response+json`; unknown fields or duplicate envelope keys
remain in the raw HTTP editor to avoid information loss.

Native `graphql-request` execution is not a completion requirement for this
HTTP-based feature. CLI 1.82.0 on the current signed-in account rejects a
collection containing Native GraphQL entities, even when only an HTTP item is
selected. Existing Desktop-created Native GraphQL collections therefore still
have this interoperability limitation; no silent conversion is performed.

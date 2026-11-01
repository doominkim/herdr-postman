<div align="center">
  <h1>herdr-postman</h1>
  <p><b>Postman workflows, inside your terminal.</b><br>
  Make everyday requests in Herdr. Keep the desktop app for the complex work.</p>
</div>

> **In development.** An unofficial community client. Not affiliated with or endorsed by Postman.

## Local files or Cloud workspaces

Choose **Local** or **Cloud** inside the terminal. Local mode works with Postman Native Git collection files that you can also open in Postman Desktop's Local View. Cloud mode uses your authenticated Postman CLI account: choose a workspace and browse its collections together.

The last source, workspace, environment selection, and collapsed collection folders are remembered. The default local collection lives in your user data directory, independently of the repository you opened Herdr from.

## Send requests without leaving Herdr

Edit requests, choose an environment, and inspect the response body, headers, status, timing, and test results. Execution uses Postman CLI, including supported collection authentication and scripts. The interface follows Postman's request and response layout, with keyboard navigation and mouse support.

Cloud environments load their values when selected. Environment values override collection defaults; session overrides take precedence. The new Cloud environment cache stores names and IDs, not variable values.

## Continue in Postman Desktop

Local file changes reload automatically. Cloud changes appear when you press **Ctrl+R** or restart; there is no live push subscription. Refresh keeps open tabs and unsaved drafts, and conflicting saves are rejected.

Cached Cloud collections open first while updates are checked in the background. Restoring a selected Cloud environment still requires a network read.

## Install

Requires **Go 1.25.5+**, **Postman CLI** on your PATH, and **Herdr 0.9.0+**. The launcher supports macOS and Linux; live desktop interoperability has been checked on macOS. Cloud mode requires `postman login`.

```sh
herdr plugin install doominkim/herdr-postman
```

The plugin builds the app from source during installation. To open it as a regular Herdr tab:

```sh
herdr plugin pane open --plugin herdr-postman --entrypoint postman --focus
```

Optional key binding in `~/.config/herdr/config.toml`:

```toml
[[keys.command]]
key = "f3"
type = "shell"
command = "herdr plugin pane open --plugin herdr-postman --entrypoint postman --focus"
description = "Open herdr-postman"
```

The manifest also provides a `toggle` action for a retained global panel. **That action requires a custom Herdr build with `plugin runtime toggle` support in both client and server.** The tab command above does not require this extension. Installing the plugin does not install or patch Herdr itself.

### Run without Herdr

```sh
git clone https://github.com/doominkim/herdr-postman.git
cd herdr-postman
go build -o bin/herdr-postman ./cmd/herdr-postman
/bin/sh scripts/pane.sh
```

To start with your own local collection:

```sh
HERDR_POSTMAN_COLLECTION=/absolute/path/to/collection /bin/sh scripts/pane.sh
```

The launcher restores the last source after the first run. To explicitly open a local collection instead:

```sh
./bin/herdr-postman --source local --collection /absolute/path/to/collection
```

`POSTMAN_BIN` overrides the Postman CLI path. `HERDR_POSTMAN_ENVIRONMENT` supplies a local environment file; `HERDR_POSTMAN_BIN` overrides the application executable.

## Current limits

- Cloud editing supports request names, descriptions, methods, URLs, and plain JSON bodies. Use Postman Desktop for Cloud request creation, deletion, and advanced edits.
- Environment editing and values stored only locally in Postman Desktop are not synchronized through Cloud.
- Some protocols and CLI features depend on your Postman plan. Native GraphQL execution is not supported by this integration; GraphQL over HTTP is available.
- Cached collection documents and request history may contain sensitive request data. They stay in the application's local data directory; do not publish that directory.

## Questions and development

Click **Question** in the bottom-right corner of the workbench, or [open an issue](https://github.com/doominkim/herdr-postman/issues).

```sh
go test ./...
python3 -m unittest discover -s plugin-tests -v
```

Some integration tests use the installed Postman CLI against local test servers. CLI version and account capabilities can affect those tests.

## Credits and license

Based on [Posting 3](https://github.com/darrenburns/posting), by Darren Burns and contributors, under Apache-2.0. The original Go module path is retained for internal imports. See [UPSTREAM.md](UPSTREAM.md), [LICENSE](LICENSE), and [NOTICE](NOTICE).

Postman and Herdr names identify the tools this project integrates with; this is an independent community project.

# v0.7.1 (unreleased)

* Stopped using `setpgid` when running bridges on macOS as it causes weird issues.

# v0.7.0 (2023-08-20)

* Added support for running official Python bridges (`telegram`, `facebook`,
  `instagram`, `googlechat`, `twitter`) and the remaining Go bridge (`slack`).
  * The legacy Signal bridge will not be supported as it requires signald as an
    external component. Once the Go rewrite is ready, a config template will be
    added for it.
* Added `bbctl proxy` command for connecting to the appservice transaction
  websocket and proxying all transactions to a local HTTP server. This enables
  using any 3rd party bridge in websocket mode (removing the need for
  port-forwarding).
* Added [experimental Docker image] for wrapping `bbctl run`.
* Updated minimum Go version to 1.20 when compiling bbctl from source.

[experimental Docker image]: https://github.com/beeper/bridge-manager/tree/main/docker

# v0.6.1 (2023-08-06)

* Added config option to store bridge databases in custom directory.
* Fixed running official Go bridges on macOS when libolm isn't installed
  system-wide.
* Fixed 30 second timeout when downloading bridge binaries.
* Fixed creating config directory if it doesn't exist.
* Changed default config path from `~/.config/bbctl.json`
  to `~/.config/bbctl/config.json`.
  * Existing configs should be moved automatically on startup.

# v0.6.0 (2023-08-01)

* Added support for fully managed installation of supported official bridges
  using `bbctl run`.
* Moved `register` and `delete` commands to top level `bbctl` instead of being
  nested inside `bbctl bridge`.
* Merged `bbctl get` into `bbctl register --get`

# v0.5.0 (2023-07-24)

* Added bridge config template for Google Messages.
* Added bridge type in bridge state info when setting up bridges with config
  templates.
  * This is preparation for integrating self-hosted official bridges into the
    Beeper apps, like login via the Chat Networks dialog and Start New Chat
    functionality.
* Fixed typo in WhatsApp config template.
* Updated config templates to enable websocket pinging so the websockets would
  stay alive.
* Moved `isSelfHosted` flag to top-level bridge state info.

# v0.4.0 (2023-07-04)

* Added email login support.
* Added link to bridge installation instructions after generating config file.
* Fixed WhatsApp and Discord bridge config templates.

# v0.3.1 (2023-06-27)

* Fixed logging in, which broke in v0.3.0

# v0.3.0 (2023-06-22)

* Fixed hungryserv address being incorrect for users on new bridge cluster.
* Added support for generating configs for the Discord bridge.
* Added option to pass config generation parameters as CLI flags
  (like `imessage_platform` and `barcelona_path`).

# v0.2.0 (2023-05-28)

* Added experimental support for generating configs for official Beeper bridges.
  WhatsApp, iMessage and Heisenbridge are currently supported, more to come in
  the future.
* Changed register commands to recommend starting bridge names with `sh-` prefix.

# v0.1.1 (2023-02-07)

* Fixed registering bridges in websocket mode.
* Fixed validating bridge names client-side to have a prettier error message.

# v0.1.0 (2023-02-06)

Initial release

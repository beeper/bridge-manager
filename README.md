# Beeper Bridge Manager
A tool for running self-hosted bridges with the Beeper Matrix server.

The primary use case is running custom/3rd-party bridges with Beeper. You can
connect any<sup>†</sup> standard Matrix application service to your Beeper
account without having to self-host a whole Matrix homeserver. Note that if you
run 3rd party bridges that don't support end-to-bridge encryption, message
contents will be visible to Beeper servers.

<sub>†caveat: hungryserv does not implement the entire Matrix client-server API, so
it's possible some bridges won't work - you can report such cases in the
self-hosting support room linked below or in GitHub issues here</sub>

You can also self-host the official bridges for maximum security using this
tool (so that message re-encryption happens on a machine you control rather
than on Beeper servers).

> [!NOTE]
> Self-hosted bridges are not entitled to the usual level of customer support
> on Beeper. If you need help with self-hosting bridges using this tool, please
> join [#self-hosting:beeper.com] instead of asking in your support room.

[#self-hosting:beeper.com]: https://matrix.to/#/#self-hosting:beeper.com

## Usage
1. Download the latest binary from [GitHub releases](https://github.com/beeper/bridge-manager/releases)
   or [actions](https://nightly.link/beeper/bridge-manager/workflows/go.yaml/main).
   * Alternatively, you can build it yourself by cloning the repo and running
     `./build.sh`. Building requires Go 1.20 or higher.
   * bbctl supports amd64 and arm64 on Linux and macOS.
     Windows is not supported natively, please use WSL.
2. Log into your Beeper account with `bbctl login`.

Then continue with one of the sections below, depending on whether you want to
run an official Beeper bridge or a 3rd party bridge.

### Official bridges
For Python bridges, you must install Python 3 with the `venv` module with your
OS package manager. For example, `sudo apt install python3 python3-venv` on
Debian-based distros. The Python version built into macOS may be new enough, or
you can get the latest version via brew. The minimum Python version varies by
bridge, but if you use the latest Debian or Ubuntu LTS, it should be new enough.

Some bridges require ffmpeg for converting media (e.g. when sending gifs), so
you should also install that with your OS package manager (`sudo apt install ffmpeg`
on Debian or `brew install ffmpeg` on macOS).

After installing relevant dependencies:

3. Run `bbctl run <name>` to run the bridge.
   * `<name>`  should start with `sh-` and consist of a-z, 0-9 and -.
   * If `<name>` contains the bridge type, it will be automatically detected.
     Otherwise pass the type with `--type <type>`.
   * Currently supported types: `discord`, `whatsapp`, `slack`, `heisenbridge`,
     `gmessages`, `telegram`, `facebook`, `instagram`, `googlechat`, `twitter`,
     `signal`, `linkedin`, `imessage` (legacy), `imessagego` (new)
   * The bridge will be installed to `~/.local/share/bbctl`. You can change the
     directory in the config file at `~/.config/bbctl.json`.
4. For now, you'll have to configure the bridge by sending a DM to the bridge
   bot (`@<name>bot:beeper.local`). Configuring self-hosted bridges through the
   chat networks dialog will be available in the future. Spaces and starting
   chats are also not yet available, although you can start chats using the
   `pm` command with the bridge bot.

There is currently a bug in Beeper Desktop that causes it to create encrypted
DMs even if the recipient doesn't support it. This means that for non-e2ee-
capable bridges like Heisenbridge, you'll have to create the DM with the bridge
bot in another Matrix client, or using the create group chat button in Beeper
Desktop.

Currently the bridge will run in foreground, so you'll have to keep `bbctl run`
active somewhere (tmux is a good option). 

Use crontab to set the bridge up to start on reboot. If you would like logging
add the following entry to your crontab and replace `<bbctl path>` with the location of 
bbctl, `<bridge name>` with the name of the bridge, and `<log path>` with the 
location of where you would like logs saved:
`@reboot /<path>/bbctl run  <bridge name> > /<log path>/<bridge name>.log`

If you do not want logging add the following to your crontab:
`@reboot /<path>/bbctl run  <bridge name> > /dev/null`

In the future, a service mode will be
added where the bridge is registered as a systemd or launchd service to be
started automatically by the OS.

### 3rd party bridges
3. Run `bbctl register <name>` to generate an appservice registration file.
   * `<name>` is a short name for the bridge (a-z, 0-9, -). The name should
     start with `sh-`. The bridge user ID namespace will be `@<name>_.+:beeper.local`
     and the bridge bot will be `@<name>bot:beeper.local`.
   * Optionally you can pass `-a <address>` to have the Beeper server push
     events directly to the bridge. However, this requires that the bridge is
     publicly accessible. The proxy option below is easier.
4. Now you can configure and run the bridge by following the bridge's own
   documentation.
5. If you didn't pass an address to `register`, modify the registration file to
   point at where the bridge will listen locally (e.g. `url: http://localhost:8080`),
   then run `bbctl proxy -r registration.yaml` to start the proxy.
   * The proxy will connect to the Beeper server using a websocket and push
     received events to the bridge via HTTP. Since the HTTP requests are all on
     localhost, you don't need port forwarding or TLS certificates.

Note that the homeserver URL may change if you're moved to a different cluster.
In general, that shouldn't happen, but it's not impossible.

If you want to get the registration again later, you can add the `--get` flag.
Just re-running `register` is allowed too, but you need to provide the address
again if you do that (which also means if you want to change the address, just
re-run register with the new address).

You can use `--json` with `register` to get the whole response as JSON instead
of registration YAML and pretty-printed extra details. This may be useful if
you want to automate fetching the homeserver URL.

### Deleting bridges
If you don't want a self-hosted bridge anymore, you can delete it using
`bbctl delete <name>`. Deleting a bridge will permanently erase all traces of
it from the Beeper servers (e.g. any rooms and ghost users it created).
For official bridges, it will also delete the local data directory with the
bridge config, database and python virtualenv (if applicable).

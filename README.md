# Beeper Bridge Manager
A tool for running self-hosted bridges with the Beeper Matrix server.

The primary use case is running custom/3rd-party bridges with Beeper. You can
connect any<sup>†</sup> spec-compliant Matrix application service to your Beeper
account without having to self-host a whole Matrix homeserver. Note that if you
run 3rd party bridges that don't support end-to-bridge encryption, message
contents will be visible to Beeper servers.

<sub>†caveat: hungryserv does not implement the entire Matrix client-server API, so
it's possible some bridges won't work - you can report such cases in the
self-hosting support room linked below or in GitHub issues here</sub>

You can also self-host the official bridges for maximum security using this
tool (so that message re-encryption happens on a machine you control rather
than on Beeper servers).

This tool can not be used with any other Matrix homeserver, like self-hosted
Synapse instances. It is only for connecting self-hosted bridges to the
beeper.com server. For self-hosting the entire stack, refer to the official
documentation of the various projects
([Synapse](https://element-hq.github.io/synapse/latest/),
[mautrix bridges](https://docs.mau.fi/bridges/)).

> [!NOTE]
> Self-hosted bridges are not entitled to the usual level of customer support
> on Beeper. If you need help with self-hosting bridges using this tool, please
> join [#self-hosting:beeper.com] instead of asking in your support room.

[#self-hosting:beeper.com]: https://matrix.to/#/#self-hosting:beeper.com

## Usage
1. Download the latest binary from [GitHub releases](https://github.com/beeper/bridge-manager/releases)
   or [actions](https://nightly.link/beeper/bridge-manager/workflows/go.yaml/main).
   * Alternatively, you can build it yourself by cloning the repo and running
     `./build.sh`. Building requires Go 1.23 or higher.
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
   * See the table below for supported official bridges.
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
active somewhere (tmux is a good option). In the future, a service mode will be
added where the bridge is registered as a systemd or launchd service to be
started automatically by the OS.

#### Official bridge list
When using `bbctl run` or `bbctl config` and the provided `<name>` contains one
of the identifiers (second column) listed below, bbctl will automatically guess
that type. A substring match is sufficient, e.g. `sh-mywhatsappbridge` will
match `whatsapp`. The first listed identifier is the "primary" one that can be
used with the `--type` flag.

| Bridge               | Identifier                           |
|----------------------|--------------------------------------|
| [mautrix-telegram]   | telegram                             |
| [mautrix-whatsapp]   | whatsapp                             |
| [mautrix-signal]     | signal                               |
| [mautrix-discord]    | discord                              |
| [mautrix-slack]      | slack                                |
| [mautrix-gmessages]  | gmessages,  googlemessages, rcs, sms |
| [mautrix-gvoice]     | gvoice, googlevoice                  |
| [mautrix-meta]       | meta, instagram, facebook            |
| [mautrix-googlechat] | googlechat, gchat                    |
| [mautrix-twitter]    | twitter                              |
| [mautrix-bluesky]    | bluesky, bsky                        |
| [mautrix-imessage]   | imessage                             |
| [beeper-imessage]    | imessagego                           |
| [mautrix-linkedin]   | linkedin                             |
| [heisenbridge]       | heisenbridge, irc                    |

[mautrix-telegram]: https://github.com/mautrix/telegram
[mautrix-whatsapp]: https://github.com/mautrix/whatsapp
[mautrix-signal]: https://github.com/mautrix/signal
[mautrix-discord]: https://github.com/mautrix/discord
[mautrix-slack]: https://github.com/mautrix/slack
[mautrix-gmessages]: https://github.com/mautrix/gmessages
[mautrix-gvoice]: https://github.com/mautrix/gvoice
[mautrix-meta]: https://github.com/mautrix/meta
[mautrix-googlechat]: https://github.com/mautrix/googlechat
[mautrix-twitter]: https://github.com/mautrix/twitter
[mautrix-bluesky]: https://github.com/mautrix/bluesky
[mautrix-imessage]: https://github.com/mautrix/imessage
[beeper-imessage]: https://github.com/beeper/imessage
[mautrix-linkedin]: https://github.com/mautrix/linkedin
[heisenbridge]: https://github.com/hifi/heisenbridge

### 3rd party bridgev2-based bridges
If you have a 3rd party bridge that's built on top of mautrix-go's bridgev2
framework, you can have bbctl generate a mostly-complete config file:

3. Run `bbctl config --type bridgev2 <name>` to generate a bridgev2 config with
   everything except the `network` section.
   * `<name>` is a short name for the bridge (a-z, 0-9, -). The name should
     start with `sh-`. The bridge user ID namespace will be `@<name>_.+:beeper.local`
     and the bridge bot will be `@<name>bot:beeper.local`.
4. Add the `network` section containing the bridge-specific configuration if
   necessary, then run the bridge normally.

All bridgev2 bridges support appservice websockets, so using `bbctl proxy` is
not necessary.

### 3rd party custom bridges
For any 3rd party bridges that don't use bridgev2, you'll only get a registration
file from bbctl and will have to configure the bridge yourself. Also, since such
3rd party bridges are unlikely to support Beeper's appservice websocket protocol,
you probably have to use `bbctl proxy` to connect to the websocket and turn
incoming data into HTTP requests for the bridge.

3. Run `bbctl register <name>` to generate an appservice registration file.
   * `<name>` is the same as in the above section.
4. Now you can configure and run the bridge by following the bridge's own
   documentation.
5. Modify the registration file to point at where the bridge will listen locally
   (e.g. `url: http://localhost:8080`), then run `bbctl proxy -r registration.yaml`
   to start the proxy.
   * The proxy will connect to the Beeper server using a websocket and push
     received events to the bridge via HTTP. Since the HTTP requests are all on
     localhost, you don't need port forwarding or TLS certificates.

Note that the homeserver URL is not guaranteed to be stable forever, it has
changed in the past, and it may change again in the future.

You can use `--json` with `register` to get the whole response as JSON instead
of registration YAML and pretty-printed extra details. This may be useful if
you want to automate fetching the homeserver URL.

### Deleting bridges
If you don't want a self-hosted bridge anymore, you can delete it using
`bbctl delete <name>`. Deleting a bridge will permanently erase all traces of
it from the Beeper servers (e.g. any rooms and ghost users it created).
For official bridges, it will also delete the local data directory with the
bridge config, database and python virtualenv (if applicable).

Note that deleting a bridge through the Beeper client settings will
*not* delete the bridge database that is stored locally; you must
delete that yourself, or use `bbctl delete` instead. (If you created
the bridge database with `bbctl run -l`, then run `bbctl delete -l`
from the same working directory to delete it.)

If you later re-add a self-hosted bridge after deleting it from the
Beeper servers but not deleting the local database, you should expect
errors, as the bridge will have been removed from Matrix rooms that it
thinks it is a member of.

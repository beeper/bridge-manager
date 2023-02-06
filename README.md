# Beeper Bridge Manager
A tool for running self-hosted bridges with the Beeper Matrix server.

The primary use case is running custom/3rd-party bridges with Beeper. You can
connect any† standard Matrix application service to your Beeper account without
having to self-host a whole Matrix homeserver. Note that if you run 3rd party
bridges that don't support end-to-bridge encryption, message contents will be
visible to Beeper servers.

<sub>†caveat: hungryserv does not implement the entire Matrix client-server API, so
it's possible some bridges won't work - you can report such cases in the
self-hosting support room linked below or in GitHub issues here</sub>

In the future, we may also support self-hosting the official bridges for
maximum security using this tool (so that message re-encryption happens on a
machine you control rather than on Beeper servers). This tool can technically
already be used for the official bridges, but they also need specific
configuration to work optimally, so you shouldn't do it yet. If you really want
to, use a different bridge name (e.g. `whatsapp2` instead of `whatsapp`).

Please note that self-hosted bridges are not entitled to the usual level of
customer support on Beeper. If you need help with self-hosting bridges using
this tool, please join [#self-hosting:beeper.com] instead of asking in your
support room.

[#self-hosting:beeper.com]: https://matrix.to/#/#self-hosting:beeper.com

## Usage
1. Build the binary with `./build.sh` (Go 1.19+ required) or download a binary
   from GitHub releases or actions.
2. Log into your Beeper account with `bbctl login`.
3. Run `bbctl bridge register -a <address> <name>` to generate an appservice
   registration file.
   * `<address>` should be a publicly reachable https address where the Beeper
     server will push new events.
   * `<name>` is a short name for the bridge (a-z, 0-9). The bridge user ID
     namespace will be `@<name>_.+:beeper.local` and the bridge bot will be
     `@<name>bot:beeper.local`.
4. Now you can configure and run the bridge by following the bridge's own
   documentation.

Note that the homeserver URL may change if the node your hungryserv is running
on dies. In general, that shouldn't happen, but it's not impossible.

If you want to get the registration again later, you can use `get` instead of
`register`. Just re-running `register` is allowed too, but you need to provide
the address again if you do that (which also means if you want to change the
address, just re-run register with the new address).

You can use `--json` with `register` and `get` to get the whole response as
JSON instead of registration YAML and pretty-printed extra details. This may be
useful if you want to automate fetching the homeserver URL.

If you don't want a self-hosted bridge anymore, you can delete it using `bbctl bridge delete <name>`.
Deleting a bridge will permanently erase all traces of it from the Beeper servers
(e.g. any rooms and ghost users it created).

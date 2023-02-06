# Beeper Bridge Manager
A tool for running self-hosted bridges with the Beeper Matrix server.

The primary use case is running custom/3rd-party bridges with Beeper. You can
connect any standard Matrix application service to your Beeper account without
having to self-host a whole Matrix homeserver.

In the future, we may also support self-hosting the official bridges for
maximum security using this tool (so that message re-encryption happens on a
machine you control rather than on Beeper servers).

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

If you don't want a self-hosted bridge anymore, you can delete it using `bbctl bridge delete <name>`.
Deleting a bridge will permanently erase all traces of it from the Beeper servers
(e.g. any rooms and ghost users it created).

# Docker
bridge-manager includes a docker file which wraps `bbctl run`. It's primarily
meant for the automated Fly deployer ([self-host.beeper.com]), but can be used
manually as well.

[self-host.beeper.com]: https://self-host.beeper.com

## Usage
```sh
docker run \
	# Mount the current directory to /data in the container (the bridge binaries, config and database will be stored here)
	-v $(pwd):/data \
	# Pass your Beeper access token here. You can find it in ~/.config/bbctl/config.json or Beeper Desktop settings -> Help & About
	-e MATRIX_ACCESS_TOKEN=... \
	# The image to run, followed by the name of the bridge to run.
	ghcr.io/beeper/bridge-manager sh-telegram
```

The container should work fine as any user (as long as the mounted `/data`
directory is writable), so you can just use the standard `--user` flag to
change the UID/GID.

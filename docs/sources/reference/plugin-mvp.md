Brian Goff and Luke Marsden paired on an MVP of the plugin handshake interface.

# pre-MVP design

We ended up with the following design:

```
$ docker run --plugin \
    -v /var/run/docker.sock:/var/run/docker.sock \
    clusterhq/flocker-plugin
```

Note the new `--plugin` argument.

Internally this translates into:

```
$ docker run -d \
	-v /var/lib/docker/containers/<container_id>/plugin/:/var/run/docker/ \
	-v /var/run/docker.sock:/var/run/docker.sock \
	clusterhq/flocker-plugin
```

Docker blocks on waiting for a successful handshake with the plugin on `/var/run/docker/plugin.sock` with an HTTP `POST /v1/handshake` with an empty body.

# Plugin API

## `POST /v1/handshake`

**Request:** POST with empty body

**Response:** `application/json` as follows:

```
{
 InterestedIn: ["volume"],
 Name: "flocker",
 Author: "Luke Marsden <luke@clusterhq.com>",
 Org: "ClusterHQ, Inc.",
 Website: "https://clusterhq.com/",
}
```

`PluginName` is a human readable short string identifying the plugin.

`InterestedIn` is a list of extension points.

Extension points currently supported are:

* `volume` - called when a volume is bind-mounted into a container at create or start time

Planned extension points include:

* `api` - pre-hooks and post-hooks on the Docker remote API (powerstrip-style)
* `network` - called when ???

## `POST /v1/volume/volumes`

The simplest of plugin types, `volume` provides a single request-response type on `POST /v1/volume/volumes` (`POST ~= create`):

**Request**

```
{HostPath: "/path",
 ContainerID: "abcdef123"}
```

**Response**

```
{HostPath: "/newpath",
 ContainerID: "abcdef123"}
```

# Caveats

NB: plugin loading order is undefined, so plugins should not depend on eachother.

# Versioning strategy

When we bump `/v1/handshake` to `/v2/handshake` etc, we can have Docker start by trying to do the highest numbered handshake it can, and then iterate backwards through supported handshake versions until it finds one which matches (ie does not give a 404).

---

TODO

* sort out plugins from normal containers and load plugins first so that containers that want to use services (e.g. portable volumes) from containers get to consume those services from other plugins.

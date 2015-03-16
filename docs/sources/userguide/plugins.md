Docker Plugins MVP
==================

Plugins provide a mechanism for hooking into the Docker engine to extend its behavior.

HTTP over a local UNIX socket is used to communicate between Docker and its plugins.

Plugins are distributed as containers.

This is targeted at Docker 1.7.

# Usage

Plugins can be loaded using a special `docker plugin load` command, as follows:

```
$ docker plugin load -e $ARGS clusterhq/flocker-plugin $@
[... fetching...]
Plugin flocker:v0.1 loaded and registered with extension points:
* volumes
Plugin flocker activated.
```

This is really just syntactic sugar for the following:

```
$ docker run -d -e $ARGS \
	-v /var/lib/docker/containers/<container_id>/plugin/:/var/run/docker/ \
	-v /var/run/docker.sock:/var/run/docker.sock \
	clusterhq/flocker-plugin $@
```

(The docker socket should only be mounted if the plugin is started with `--privileged`.)

Loading a plugin forces it to always be loaded when Docker restarts (and Docker doesn't respond to API requests until it completes loading all its plugins).

Docker then waits for the plugin to start listening on the socket (it polls the socket until it gets a successful response to an HTTP query to `/v1/handshake` on the socket, which returns with just a list of subsystems the plugin is interested in - response defined below).
According to the type of plugin which is negotiated in the handshake, Docker registers the plugin to send it HTTP requests on certain events.

Plugins should name themselves in the response, which should be in this format:

```
{
 InterestedIn: ["volume"],
 PluginName: "flocker"
}
```

Every event should be sent to every plugin registered for that subsystem for now.
Later the handshake can include more granular negotiation over which events it wants to receive.

Other `plugin` subcommands can include `list`, `status`, `upgrade`, and `unload`.

# Types of plugin

Each plugin type defines a straightforward (MVP) protocol.

## Volume

We will start with this first because it is has the tiniest interface.

The simplest of plugin types, `volume` provides a single request-response type on `POST /v1/volumes` (`POST ~= create`):

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

In the initial version, if the plugin responds with a 404 then the response is ignored.

See reference implementation:

* [Docker volumes extension mechanism](https://github.com/cpuguy83/docker/compare/ddb366ee9a07e3feab766cc712c9683ad0c3c309)
* [Flocker extension](https://github.com/clusterhq/powerstrip-flocker/compare/docker-volume-extension)

**Demo**

Despite the name, this demo does not use powerstrip!

```
$ git clone https://github.com/clusterhq/powerstrip-flocker
$ cd powerstrip-flocker/vagrant-aws
$ git checkout docker-volume-extension
$ vagrant up --provider=aws
$ vagrant ssh node1
node1$ docker run -v /flocker/test:/data ubuntu sh -c "echo fish > /data/file.txt"
node1$ exit
$ vagrant ssh node2
node2$ docker run -v /flocker/test:/data ubuntu cat /data/file.txt
fish
```

In future, we will want to extend this to notify volume extensions when containers start and stop, so that the underlying volumes mechanism can hold and release *leases* on volumes.

### Volumes CLI

We may want to extend some (as-yet non-existent) docker core volumes CLI in due course, so you can do something like the following:

```
$ docker volume create --driver flocker \
	--metadata size=50G,tier=ssd,replication=global nicevol
$ docker run -v /flocker/nicevol:/data dockerfile/postgresql
```

This would result in similar API requests being made.

Other `docker volume` commands which could make sense are e.g. `docker volume snapshot` and `docker volume rm` which will need corresponding extension API message types to be defined.

## API

Extensions to the Docker remote API (extension type `api`) can be achieved via a [Powerstrip-like protocol](https://github.com/clusterhq/powerstrip#pre-hook-adapter-endpoints-receive-posts-like-this).
The corresponding limitations will initially apply to the types of requests which can be processed, but this is acceptable for an MVP.

## Network

Network extensions exist to implement the following UX:

```
$ docker network create greatnet
$ docker run --net=greatnet --ip=1.2.3.4 dockerfile/postgresql
```

(and of course: `$ docker run --net=greatnet --ip=1.2.3.4 -v //nicevol:/data dockerfile/postgresql` should also be possible, when both networking and storage plugins are loaded)

**Implementation note:** Whether `docker network` calls the Docker client binary and Docker daemon, or whether the first class networks functionality is implemented by separate binary *should be transparent to the plugin*.

Similar request-response pairs as for volumes can be defined for network `create`, `add-container`, etc.


# Semantics

This area needs fleshing out.

* Should chaining be possible?
* What happens when you load several of the same type of extension?
* What happens when a hook or pre-hook fails?
  What about post-hooks?
  Do we need special cleanup hooks?

# Possible future functionality

* While waiting for the plugin to initialize, Docker should not allow any API requests to succeed.
  This is to avoid startup race where e.g. `create` requests might not get passed through the plugins.
* Plugins could use a protocol other than HTTP.
* Loading a plugin marks the plugin's container as hidden from `docker ps`.

# Areas for discussion

* Should the plugin endpoints all be a single endpoint, or should they define e.g. `/v1/networks`, `/v1/volumes` etc?
  Certainly for the `api` type plugins, `POST`ing them all to a single endpoint made sense for Powerstrip.

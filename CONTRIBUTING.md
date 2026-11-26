# Contributing to drbd-shutdown-guard

The [Piraeus contributing guide](https://github.com/piraeusdatastore/piraeus/blob/master/CONTRIBUTING.md) describes how to report issues, write commits and changelog entries, and open pull requests.
The sections that follow describe what is specific to this repository.

## Target branch

Open pull requests against the `main` branch.

## Build

Install [Go](https://go.dev/) in the version declared in `go.mod`.
Build the binary and vet the code:

```
$ go build .
$ go vet ./...
```

On every pull request, CI builds the container image.
The repository has no automated tests.

## Testing on a cluster

[Piraeus Operator](https://github.com/piraeusdatastore/piraeus-operator) runs drbd-shutdown-guard as an init container of the LINSTOR Satellite Pods.
The `install` command creates the `drbd-shutdown-guard.service` systemd unit in `/run/systemd/system` on the host.
Test changes on a node that runs systemd.

To try a change, build an image with [Docker Buildx](https://docs.docker.com/build/buildx/) and push it to your registry:

```
$ docker buildx build --push --tag registry.example.com/myname/drbd-shutdown-guard:dev .
```

Set the `drbd-shutdown-guard` component in the [image configuration](https://piraeus.io/docs/stable/explanation/image-configuration/) of Piraeus Operator to the new image.
After the Satellite Pod restarts, check the unit on the host:

```
$ systemctl status drbd-shutdown-guard.service
```

# drbd-shutdown-guard

This project is intended to aid Kubernetes cluster using DRBD® with `suspend-io` behaviour during shutdown.
You would deploy it as part of LINSTOR® Satellite containers, during initialization:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: satellite
spec:
  initContainers:
    - name: drbd-shutdown-guard
      image: quay.io/piraeusdatastore/drbd-shutdown-guard:latest
      securityContext:
        privileged: true
        readOnlyRootFilesystem: true
      volumeMounts:
      - name: run-systemd-system
        mountPath: /run/systemd/system/
      - name: run-drbd-shutdown-guard
        mountPath: /run/drbd-shutdown-guard
      - name: systemd-bus-socket
        mountPath: /run/dbus/system_bus_socket
  volumes:
    - name: run-systemd-system
      hostPath:
        path: /run/systemd/system/
        type: Directory
    - name: run-drbd-shutdown-guard
      hostPath:
        path: /run/drbd-shutdown-guard
        type: DirectoryOrCreate
    - name: systemd-bus-socket
      hostPath:
        path: /run/dbus/system_bus_socket
        type: Socket
```

## `drbd-shutdown-guard execute`
Calls the configured (via environment variable) `drbdsetup` command
to list	all resources, and executes `drbdsetup secondary --force` for each.
This should be called during shutdown, before systemd tries to unmount any
remaining file-systems.	This ensures that systemd can cleanly shut down, instead
of getting stuck in any	suspended DRBD resources.

## `drbd-shutdown-guard install`
Install a systemd service via /run/systemd/system to call `drbd-shutdown-guard execute`
during system shutdown. The systemd unit will look like this:

```
[Unit]
Description=Ensure that DRBD devices with suspended IO are resumed (with potential IO errors) during shutdown.
# Ensure the stop action only runs after normal container shut down
Before=kubelet.service
# Ensure we get stopped during shutdown
Conflicts=umount.target

[Service]
Type=oneshot
RemainAfterExit=yes
Environment=DRBDSETUP_LOCATION=/run/drbd-shutdown-guard/drbdsetup
ExecStop=/run/drbd-shutdown-guard/drbd-shutdown-guard execute
```

It will copy `drbd-shutdown-guard` and `drbdsetup` into `/run/drbd-shutdown-guard/`.

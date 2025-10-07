package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path"

	"github.com/coreos/go-systemd/v22/dbus"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/piraeusdatastore/drbd-shutdown-guard/pkg/vars"
)

const (
	ServiceRuntimeDirectory = "/run/drbd-shutdown-guard"
	ServiceBinaryName       = "drbd-shutdown-guard"
	DrbdSetupBinaryName     = "drbdsetup"
	SystemdRuntimeDirectory = "/run/systemd/system"
	SystemdServiceName      = "drbd-shutdown-guard.service"
	DrbdSetupEnv            = "DRBDSETUP_LOCATION"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	cmd := cobra.Command{
		Use:     ServiceBinaryName,
		Version: vars.Version,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			log.Printf("Running %s version %s\n", ServiceBinaryName, vars.Version)
		},
	}

	cmd.AddCommand(install())
	cmd.AddCommand(execute())

	err := cmd.ExecuteContext(ctx)
	if err != nil {
		log.Fatalf("failed: %s", err.Error())
	}
}

func install() *cobra.Command {
	return &cobra.Command{
		Use:  "install",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			log.Printf("Creating service directory '%s'\n", ServiceRuntimeDirectory)
			err := os.MkdirAll(ServiceRuntimeDirectory, os.FileMode(0755))
			if err != nil {
				return fmt.Errorf("failed to create systemd runtime unit directory '%s': %w", SystemdRuntimeDirectory, err)
			}

			log.Printf("Copying drbdsetup to service directory\n")
			err = atomicCreateFile(path.Join(ServiceRuntimeDirectory, DrbdSetupBinaryName), os.FileMode(0755), copyBinary(os.Getenv(DrbdSetupEnv)))
			if err != nil {
				return fmt.Errorf("failed to copy drbdsetup: %w", err)
			}

			log.Printf("Copying %s to service directory\n", ServiceBinaryName)
			self, err := os.Executable()
			if err != nil {
				return fmt.Errorf("failed to look up currently running binary: %w", err)
			}

			err = atomicCreateFile(path.Join(ServiceRuntimeDirectory, ServiceBinaryName), os.FileMode(0755), copyBinary(self))
			if err != nil {
				return fmt.Errorf("failed to copy drbdsetup: %w", err)
			}

			log.Printf("Optionally: relabel service directory for SELinux")
			err = exec.CommandContext(ctx, "chcon", "--recursive", "system_u:object_r:bin_t:s0", ServiceRuntimeDirectory).Run()
			if err != nil {
				log.Printf("ignoring error when setting selinux label: %s", err)
			}

			log.Printf("Creating systemd unit %s in %s\n", SystemdServiceName, SystemdRuntimeDirectory)
			err = os.MkdirAll(SystemdRuntimeDirectory, os.FileMode(0755))
			if err != nil {
				return fmt.Errorf("failed to create systemd runtime unit directory '%s': %w", SystemdRuntimeDirectory, err)
			}

			err = atomicCreateFile(path.Join(SystemdRuntimeDirectory, SystemdServiceName), os.FileMode(0644), writeShutdownServiceUnit)
			if err != nil {
				return fmt.Errorf("failed to write service unit '%s': %w", ServiceRuntimeDirectory, err)
			}

			log.Printf("Connect to systemd bus")
			conn, err := dbus.NewSystemConnectionContext(ctx)
			if err != nil {
				return fmt.Errorf("failed to connect to systemd bus: %w", err)
			}

			log.Printf("Reloading systemd\n")
			err = conn.ReloadContext(ctx)
			if err != nil {
				return fmt.Errorf("failed to reload systemd")
			}

			result := make(chan string, 1)
			log.Printf("Starting systemd unit %s\n", SystemdServiceName)
			_, err = conn.StartUnitContext(ctx, SystemdServiceName, "replace", result)
			if err != nil {
				return fmt.Errorf("failed to start systemd service '%s': %w", SystemdServiceName, err)
			}

			select {
			case s := <-result:
				if s == "done" {
					log.Printf("Install successful\n")
				} else {
					return fmt.Errorf("systemd start job failed: %s", s)
				}
			case <-ctx.Done():
				return fmt.Errorf("systemd start job cancelled")
			}

			return nil
		},
	}
}

type DrbdStatusEntry struct {
	Name string `json:"name"`
}

func execute() *cobra.Command {
	return &cobra.Command{
		Use:  "execute",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			drbdSetupPath := os.Getenv(DrbdSetupEnv)

			log.Printf("Running 'drbdsetup status --json' to get current resources\n")
			out, err := exec.CommandContext(ctx, drbdSetupPath, "status", "--json").Output()
			if err != nil {
				return fmt.Errorf("failed to run 'drbdsetup status --json': %w", err)
			}

			var resources []DrbdStatusEntry
			err = json.Unmarshal(out, &resources)
			if err != nil {
				return fmt.Errorf("failed to parse output of 'drbdsetup status --json': %w", err)
			}

			var errg errgroup.Group
			for i := range resources {
				name := resources[i].Name
				errg.Go(func() error {
					log.Printf("Running 'drbdsetup secondary --force %s'\n", name)
					err := exec.CommandContext(ctx, drbdSetupPath, "secondary", "--force", name).Run()
					if err != nil {
						return fmt.Errorf("failed to run 'drbdsetup secondary --force %s': %w", name, err)
					}

					return nil
				})
			}

			return errg.Wait()
		},
	}
}

func writeShutdownServiceUnit(f *os.File) error {
	_, err := f.WriteString(fmt.Sprintf(`[Unit]
Description=Ensure that DRBD devices with suspended IO are resumed (with potential IO errors) during shutdown.
# Ensure the stop action only runs after normal container shut down
Before=kubelet.service
# Ensure we get stopped during shutdown
Conflicts=umount.target

[Service]
Type=oneshot
RemainAfterExit=yes
Environment=DRBDSETUP_LOCATION=%s
ExecStop=%s execute
`, path.Join(ServiceRuntimeDirectory, DrbdSetupBinaryName), path.Join(ServiceRuntimeDirectory, ServiceBinaryName)))
	if err != nil {
		return fmt.Errorf("failed to write unit file: %w", err)
	}

	return nil
}

func copyBinary(p string) func(f *os.File) error {
	return func(f *os.File) error {
		src, err := os.Open(p)
		if err != nil {
			return fmt.Errorf("failed to open copy source '%s': %w", p, err)
		}

		_, err = io.Copy(f, src)
		if err != nil {
			return fmt.Errorf("failed to copy '%s' to '%s': %w", p, f.Name(), err)
		}

		return nil
	}
}

func atomicCreateFile(p string, perm os.FileMode, write func(f *os.File) error) error {
	tf, err := os.CreateTemp(path.Dir(p), path.Base(p))
	if err != nil {
		return fmt.Errorf("failed to create temporary file for '%s': %w", p, err)
	}

	err = tf.Chmod(perm)
	if err != nil {
		return fmt.Errorf("failed to update temporary file permissions for '%s': %w", p, err)
	}

	err = write(tf)
	if err != nil {
		_ = tf.Close()
		return err
	}

	err = tf.Close()
	if err != nil {
		return fmt.Errorf("failed to close temporary file for '%s': %w", p, err)
	}

	err = os.Rename(tf.Name(), p)
	if err != nil {
		return fmt.Errorf("failed to move temporary file to final location '%s': %w", p, err)
	}

	return nil
}

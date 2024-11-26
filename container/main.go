package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/maxgio92/gofork/container/pkg/uts"
	"github.com/pkg/errors"
)

const (
	self = "/proc/self/exe"
)

type Options struct {
	exe  string
	args []string
}

type Container struct {
	cmd *exec.Cmd
}

func NewContainer() *Container {
	cmd := new(exec.Cmd)
	return &Container{cmd: cmd}
}

// run is an initializer that is executed once the manager is "fork"ed.
// The manager process is reexecuted inside the unshared namespaces in order to
// setup during the initialization this function ensures the required namespaces
// for the container before running its command.
func (o *Container) Run(exe string, args ...string) error {
	o.cmd = exec.Command(exe, args...)

	o.cmd.Stdout = os.Stdout
	o.cmd.Stderr = os.Stderr
	o.cmd.Stdin = os.Stdin

	return o.cmd.Run()
}

func (o *Container) Init(exe string, args ...string) error {
	o.cmd = &exec.Cmd{
		Path: exe,
		Args: args,
		SysProcAttr: &syscall.SysProcAttr{
			Pdeathsig: syscall.SIGTERM,
		},
	}

	// Unshare the namespaces and run the container command from the
	// unshared namespaces.
	o.cmd.SysProcAttr = &syscall.SysProcAttr{
		Unshareflags: syscall.CLONE_NEWUSER |
			syscall.CLONE_NEWUSER |
			syscall.CLONE_NEWIPC |
			syscall.CLONE_NEWUTS,
		//syscall.CLONE_NEWPID | // TODO: mount a procfs
		//syscall.CLONE_NEWNET | // TODO: setup ifaces
		//syscall.CLONE_NEWNS, // TODO: mount a rootfs
		UidMappings: []syscall.SysProcIDMap{{
			ContainerID: 0,
			HostID:      os.Getuid(),
			Size:        1,
		}},
		GidMappings: []syscall.SysProcIDMap{{
			ContainerID: 0,
			HostID:      os.Getgid(),
			Size:        1,
		}},
		AmbientCaps: []uintptr{}, // TODO: enable selective caps.
	}

	o.cmd.Stdin = os.Stdin
	o.cmd.Stdout = os.Stdin
	o.cmd.Stderr = os.Stderr

	return o.cmd.Start()
}

func (o *Container) SetHostname() error {
	hostName := uts.GetRandHostName()
	if err := syscall.Sethostname([]byte(hostName)); err != nil {
		return errors.Wrap(err, "error setting hostname")
	}

	return nil
}

func (o *Container) Wait() error {
	return o.cmd.Wait()
}

func main() {
	o := new(Options)
	flag.Parse()
	if len(flag.Args()) == 0 {
		flag.Usage()
		os.Exit(1)
	}

	c := NewContainer()

	// This manager runs twice: once in the host manager and
	// once when the manager is re-executed in the initialized container,
	// before preparing the last stuff and running the continer command.
	// See https://github.com/moby/moby/pkg/reexec for reference.
	if os.Args[0] == "run" {
		if len(flag.Args()) > 0 {
			o.exe = flag.Args()[0]
		}
		if len(flag.Args()) > 1 {
			args := flag.Args()[1:]
			o.args = args
		}

		if err := c.SetHostname(); err != nil {
			fmt.Fprintf(os.Stderr, "%v", err)
			os.Exit(1)
		}
		if err := c.Run(o.exe, o.args...); err != nil {
			fmt.Fprintf(os.Stderr, "error running container: %s", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	// The container needs to be initialized first, by self re-executing,
	// before running the container command, just before which set stuff
	// like the hostname.
	args := make([]string, 0)
	args = append(args, "run")

	// Skip the executable path.
	os.Args = os.Args[1:]
	args = append(args, os.Args...)

	// Re-execute in the container.
	if err := c.Init(self, args...); err != nil {
		fmt.Fprintf(os.Stderr, "cannot initialize container: %s\n", err)
		os.Exit(1)
	}

	// Wait for the container process.
	if err := c.Wait(); err != nil {
		fmt.Fprintf(os.Stderr, "container terminated with error: %s\n", err)
		os.Exit(1)
	}
}

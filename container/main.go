package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/maxgio92/gofork/container/pkg/uts"
)

const (
	self = "/proc/self/exe"
)

var initializers = make(map[string]func())

// The package init function runs twice: once in the host manager and
// once when the manager is "fork"ed inside the unshared namespaces to
// initialize the container and run its command.
// See https://github.com/moby/moby/pkg/reexec for reference.
func runInit() {
	registerInitializer("contInitAndRun")
	if initializer, ok := initializers[os.Args[0]]; ok {
		initializer()
		os.Exit(0)
	}
}

func registerInitializer(name string) {
	if _, exists := initializers[name]; exists {
		panic(fmt.Sprintf("initializer already registered under name %q", name))
	}

	initializers[name] = contInitAndRun
}

// contInitAndRun is an initializer that is executed once the manager is "fork"ed.
// The manager process is reexecuted inside the unshared namespaces in order to
// setup during the initialization this function ensures the required namespaces
// for the container before running its command.
func contInitAndRun() {
	var exe string
	var arg []string
	if len(os.Args) > 0 {
		exe = os.Args[1]
	}
	if len(os.Args) > 1 {
		arg = os.Args[2:]
	}
	cmd := exec.Command(exe, arg...)

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	hostName := uts.GetRandHostName()
	if err := syscall.Sethostname([]byte(hostName)); err != nil {
		fmt.Fprintf(os.Stderr, "error setting hostname - %s\n", err)
		os.Exit(1)
	}

	ps1 := fmt.Sprintf("[%s]$ ", hostName)
	cmd.Env = append(cmd.Env, fmt.Sprintf("PS1=%s", ps1))

	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error running container with command %s: %s", cmd.String(), err)
		os.Exit(1)
	}
}

func main() {
	var interactive, terminal bool
	flag.BoolVar(&interactive, "i", false, "Attach to STDIN")
	flag.BoolVar(&terminal, "t", false, "Attach to STDOUT and STDOUT")
	flag.Parse()
	if len(flag.Args()) == 0 {
		flag.Usage()
		os.Exit(1)
	}
	runInit()

	args := make([]string, 0)
	args = append(args, "contInitAndRun")
	args = append(args, flag.Args()...)

	// The manager needs to be "fork"ed in order to initialize stuff
	// after the namespaces have been unshared but before executing
	// the container command.
	cmd := &exec.Cmd{
		Path: self,
		Args: args,
		SysProcAttr: &syscall.SysProcAttr{
			Pdeathsig: syscall.SIGTERM,
		},
	}

	// Unshare the namespaces and run the container command from the
	// unshared namespaces.
	cmd.SysProcAttr = &syscall.SysProcAttr{
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

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdin
	cmd.Stderr = os.Stderr

	// Start the child process.
	var err error
	if err = cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "cannot start command %s\n", cmd.String())
		os.Exit(1)
	}

	// Wait for the child process.
	if err = cmd.Wait(); err != nil {
		fmt.Fprintf(os.Stderr, "container %d terminated with error: %v\n", cmd.Process.Pid, err)
		os.Exit(1)
	}
}

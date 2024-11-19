package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"

	"github.com/maxgio92/gofork/container/internal/utils"
)

const (
	ps1 = "[gofork]$ "
)

func main() {
	var interactive bool
	flag.BoolVar(&interactive, "interactive", false, "Attach to STDIN")

	var terminal bool
	flag.BoolVar(&terminal, "terminal", false, "Attach to STDOUT and STDOUT")

	flag.Parse()

	args := make([]string, len(flag.Args()))
	for i := 1; i < len(flag.Args()); i++ {
		args = append(args, flag.Args()[i])
	}

	if len(flag.Args()) == 0 {
		flag.Usage()
		os.Exit(1)
	}

	exe := flag.Args()[0]
	args = flag.Args()[1:]
	cmd := exec.Command(exe, args...)

	// Clone syscall flags.
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Unshareflags: syscall.CLONE_NEWUSER |
			syscall.CLONE_NEWIPC |
			syscall.CLONE_NEWUTS |
			syscall.CLONE_NEWPID | // TODO: mount a procfs
			syscall.CLONE_NEWNET | // TODO: setup ifaces
			syscall.CLONE_NEWNS, // TODO: mount a rootfs
		UidMappings: []syscall.SysProcIDMap{{
			ContainerID: 0,
			HostID:      1000,
			Size:        1,
		}},
		GidMappings: []syscall.SysProcIDMap{{
			ContainerID: 0,
			HostID:      1000,
			Size:        1,
		}},
		AmbientCaps: []uintptr{},
	}

	var stdout, stderr io.Reader
	var err error
	if interactive {
		cmd.Stdin = os.Stdin
	}
	if terminal {
		cmd.Stdout = os.Stdin
		cmd.Stderr = os.Stderr
		cmd.Env = append(cmd.Env, fmt.Sprintf("PS1=%s", ps1))
	}
	if !interactive && !terminal {
		// Capture stdout and stderr via pipes.
		stdout, err = cmd.StdoutPipe()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error creating stdout pipe: %s\n", err)
			os.Exit(1)
		}
		stderr, err = cmd.StderrPipe()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error creating stderr pipe: %s\n", err)
			os.Exit(1)
		}
	}

	// Start the child process.
	if err = cmd.Start(); err != nil {
		fmt.Println(cmd.Args)
		fmt.Fprintf(os.Stderr, "cannot start command %s\n", cmd.String())
		os.Exit(1)
	}

	// Read the output from stdout and stderr.
	if !interactive && !terminal {
		if stdout != nil {
			go func() {
				output, err := utils.Read(stdout)
				if err != nil {
					fmt.Fprintf(os.Stderr, "error reading stdout: %s\n", err)
				}
				if output != "" {
					fmt.Fprintf(os.Stdout, output)
				}
			}()
		}
		if stderr != nil {
			go func() {
				output, err := utils.Read(stderr)
				if err != nil {
					fmt.Fprintf(os.Stderr, "error reading stderr: %s\n", err)
				}
				if output != "" {
					fmt.Fprintf(os.Stderr, "%s\n", output)
				}
			}()
		}
	}

	// Wait for the child process.
	if err = cmd.Wait(); err != nil {
		fmt.Fprintf(os.Stderr, "container %d terminated with error: %v\n", cmd.Process.Pid, err)
		os.Exit(1)
	}
}

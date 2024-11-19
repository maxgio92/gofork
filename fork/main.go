package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"syscall"
)

var (
	exePathSymlnk = "/proc/self/exe"
)

func main() {
	if len(os.Args) > 1 &&  os.Args[1] == strconv.Itoa(os.Getppid()) {
		// I'm in the child process.
		if len(os.Args) > 2 {
			args := []string{}
			for i := 2; i < len(os.Args); i ++ {
				args = append(args, os.Args[i])
			}
			exe := args[0]
			args = args[1:]
			cmd := exec.Command(exe, args...)
			cmd.Stdout = os.Stdout
			if err := cmd.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "%s", err)
				os.Exit(1)
			}
		}
	} else {
		// I'm in the parent process. Fork.
		exePath, err := os.Readlink(exePathSymlnk)
		if err != nil {
			panic(err)
		}

		args := []string{}
		args = append(args, strconv.Itoa(os.Getpid()))
		for i := 1; i < len(os.Args); i ++ {
			args = append(args, os.Args[i])
		}

		cmd := exec.Command(exePath, args...)
		cmd.Env = os.Environ()
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Unshareflags:
				syscall.CLONE_NEWUSER |
				syscall.CLONE_NEWIPC |
				syscall.CLONE_NEWUTS |
				syscall.CLONE_NEWPID |
				syscall.CLONE_NEWNET |
				syscall.CLONE_NEWNS,
			UidMappings: []syscall.SysProcIDMap{{
				ContainerID: 0,
				HostID: 1000,
				Size: 1,
			}},
			GidMappings: []syscall.SysProcIDMap{{
				ContainerID: 0,
				HostID: 1000,
				Size: 1,
			}},
			AmbientCaps: []uintptr{},
		}

		// Capture stdout and stderr.
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error creating stdout pipe: %s\n", err)
			os.Exit(1)
		}
		stderr, err := cmd.StderrPipe()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error creating stderr pipe: %s\n", err)
			os.Exit(1)
		}

		// Start the child process.
		if err = cmd.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "cannot start command %s\n", cmd.String())
			os.Exit(1)
		}

		// Read the output from stdout and stderr.
		go func() {
			output, err := read(stdout)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error reading stdout: %s\n", err)
			}
			if output != "" {
				fmt.Fprintf(os.Stdout, output)
			}
		}()
		go func() {
			output, err := read(stderr)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error reading stderr: %s\n", err)
			}
			if output != "" {
				fmt.Fprintf(os.Stderr, "%s\n", output)
			}
		}()

		// Wait for the child process.
		if err = cmd.Wait(); err != nil {
			fmt.Fprintf(os.Stderr, "container %d terminated with error: %v\n", cmd.Process.Pid, err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "terminated.")
	}
}

func read(reader io.Reader) (string, error) {
	var buf bytes.Buffer
	_, err := io.Copy(&buf, reader)

	return buf.String(), err
}


package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/codegangsta/cli"
	"github.com/koding/fuseklient"
)

var ErrNotInMount = errors.New("command not run on mount")

func RunCommandFactory(c *cli.Context) int {
	localPath, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		fmt.Printf("Error running command: '%s'\n", err)
		return 1
	}

	r, err := NewRunCommand()
	if err != nil {
		fmt.Printf("Error running command: '%s'\n", err)
		return 1
	}

	if len(c.Args()) < 1 {
		cli.ShowCommandHelp(c, "run")
		return 1
	}

	res, err := r.Run(localPath, c.Args()[0:])
	if err != nil && err != fuseklient.ErrNotInMount {
		fmt.Printf("Error running command: '%s'\n", err)
		return 1
	}

	if err == fuseklient.ErrNotInMount {
		fmt.Println("Error: 'run' command only works from inside a mount")
		return 1
	}

	// Write to standard out stream.
	// NOTE: This stream can contain values even if exit status is not 0.
	os.Stderr.WriteString(res.Stdout)

	if res.ExitStatus != 0 {
		os.Stderr.WriteString(res.Stderr)
		return res.ExitStatus
	}

	return 0
}

type RunCommand struct {
	// Transport is communication layer between this and local klient.
	// This is used to run the command on the remote machine.
	Transport
}

func NewRunCommand() (*RunCommand, error) {
	klientKite, err := CreateKlientClient(NewKlientOptions())
	if err != nil {
		return nil, err
	}

	if err := klientKite.Dial(); err != nil {
		return nil, err
	}

	return &RunCommand{Transport: klientKite}, nil
}

func (r *RunCommand) Run(localPath string, cmdWithArgs []string) (*ExecRes, error) {
	machine, err := fuseklient.GetMachineMountedForPath(localPath)
	if err != nil {
		return nil, err
	}

	fullCmdPath, err := r.getCmdRemotePath(machine, localPath)
	if err != nil {
		return nil, err
	}

	return r.RunOnMachine(machine, fullCmdPath, cmdWithArgs)
}

func (r *RunCommand) RunOnMachine(machine, fullCmdPath string, cmdWithArgs []string) (*ExecRes, error) {
	req := struct {
		Machine string
		Command string
		Path    string
	}{
		Machine: machine,
		Command: strings.Join(cmdWithArgs, " "),
		Path:    fullCmdPath,
	}
	raw, err := r.Tell("remote.exec", req)
	if err != nil {
		return nil, err
	}

	var res ExecRes
	if err := raw.Unmarshal(&res); err != nil {
		return nil, err
	}

	return &res, nil
}

// getCmdRemotePath return the path on remote machine where the command
// should be run.
func (r *RunCommand) getCmdRemotePath(machine, localPath string) (string, error) {
	relativePath, err := fuseklient.GetRelativeMountPath(localPath)
	if err != nil {
		return "", err
	}

	mounts, err := r.getMounts()
	if err != nil {
		return "", err
	}

	for _, m := range mounts {
		if m.MountName == machine {
			// join path in remote machine
			return filepath.Join(m.RemotePath, relativePath), nil
		}
	}

	return "", ErrNotInMount
}

func (r *RunCommand) getMounts() ([]kiteMounts, error) {
	res, err := r.Tell("remote.mounts")
	if err != nil {
		return nil, err
	}

	var mounts []kiteMounts
	if err := res.Unmarshal(&mounts); err != nil {
		return nil, err
	}

	return mounts, nil
}

func isMachineMatchPartial(m1, m2 string) bool {
	return m1 == m2 || strings.HasPrefix(m1, m2)
}

package command

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

var (
	waitKillTimeout = 10 * time.Second
)

type Cmd struct {
	outStream, errStream io.Writer

	cmd      *exec.Cmd
	command  string
	commands []string
	timeout  time.Duration
}

func NewCommand(outStream, errStream io.Writer, cmd string, timeout time.Duration) *Cmd {
	return &Cmd{
		outStream: outStream,
		errStream: errStream,
		command:   cmd,
		commands:  []string{"sh", "-c", cmd},
		timeout:   timeout,
	}
}

func NewCommands(outStream, errStream io.Writer, cmds []string, timeout time.Duration) *Cmd {
	return &Cmd{
		outStream: outStream,
		errStream: errStream,
		command:   cmds[0],
		commands:  cmds,
		timeout:   timeout,
	}
}

func (c *Cmd) Exec() error {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout+waitKillTimeout)
	defer cancel()
	timeoutCh := time.After(c.timeout)
	cmd := exec.CommandContext(ctx, c.commands[0], c.commands[1:]...)
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	exitCh := make(chan struct{})
	errCh := make(chan error)

	go func(cmd *exec.Cmd) {
		select {
		case <-exitCh:
			return
		case <-timeoutCh:
			err := cmd.Process.Signal(syscall.SIGTERM)
			if err != nil {
				errCh <- err
			}
			return
		}
	}(cmd)

	err := cmd.Start()
	if err != nil {
		fmt.Fprint(c.outStream, stdout.String())
		fmt.Fprint(c.errStream, stderr.String())
		return fmt.Errorf("exec: %s; err: %+v", strings.Join(c.commands, " "), err)
	}

	go func() {
		defer func() {
			close(exitCh)
		}()

		err := cmd.Wait()
		if err != nil {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		if err != nil {
			fmt.Fprint(c.outStream, stdout.String())
			fmt.Fprint(c.errStream, stderr.String())
			return fmt.Errorf("exec: %s; err: %+v", strings.Join(c.commands, " "), err)
		}
	case <-exitCh:
	}

	return nil
}

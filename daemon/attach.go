package daemon // import "github.com/docker/docker/daemon"

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/containerd/containerd/tracing"
	"github.com/containerd/log"
	"github.com/docker/docker/api/types/backend"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/container"
	"github.com/docker/docker/container/stream"
	"github.com/docker/docker/daemon/logger"
	"github.com/docker/docker/errdefs"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/moby/term"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/attribute"
)

// ContainerAttach attaches to logs according to the config passed in. See ContainerAttachConfig.
func (daemon *Daemon) ContainerAttach(ctx context.Context, prefixOrName string, req *backend.ContainerAttachConfig) error {
	keys := []byte{}
	var err error
	if req.DetachKeys != "" {
		keys, err = term.ToBytes(req.DetachKeys)
		if err != nil {
			return errdefs.InvalidParameter(errors.Errorf("Invalid detach keys (%s) provided", req.DetachKeys))
		}
	}

	ctr, err := daemon.GetContainer(prefixOrName)
	if err != nil {
		return err
	}
	if ctr.IsPaused() {
		err := fmt.Errorf("container %s is paused, unpause the container before attach", prefixOrName)
		return errdefs.Conflict(err)
	}
	if ctr.IsRestarting() {
		err := fmt.Errorf("container %s is restarting, wait until the container is running", prefixOrName)
		return errdefs.Conflict(err)
	}

	cfg := stream.AttachConfig{
		UseStdin:   req.UseStdin,
		UseStdout:  req.UseStdout,
		UseStderr:  req.UseStderr,
		TTY:        ctr.Config.Tty,
		CloseStdin: ctr.Config.StdinOnce,
		DetachKeys: keys,
	}
	ctr.StreamConfig.AttachStreams(&cfg)

	multiplexed := !ctr.Config.Tty && req.MuxStreams

	clientCtx, closeNotify := context.WithCancel(context.Background())
	defer closeNotify()
	go func() {
		<-clientCtx.Done()
		// The client has disconnected
		// In this case we need to close the container's output streams so that the goroutines used to copy
		// to the client streams are unblocked and can exit.
		if cfg.CStdout != nil {
			cfg.CStdout.Close()
		}
		if cfg.CStderr != nil {
			cfg.CStderr.Close()
		}
	}()

	inStream, outStream, errStream, err := req.GetStreams(multiplexed, closeNotify)
	if err != nil {
		return err
	}

	defer inStream.Close()

	if multiplexed {
		errStream = stdcopy.NewStdWriter(errStream, stdcopy.Stderr)
		outStream = stdcopy.NewStdWriter(outStream, stdcopy.Stdout)
	}

	if cfg.UseStdin {
		cfg.Stdin = inStream
	}
	if cfg.UseStdout {
		cfg.Stdout = outStream
	}
	if cfg.UseStderr {
		cfg.Stderr = errStream
	}

	if err := daemon.containerAttach(ctx, ctr, &cfg, req.Logs, req.Stream); err != nil {
		fmt.Fprintf(outStream, "Error attaching: %s\n", err)
	}
	return nil
}

// ContainerAttachRaw attaches the provided streams to the container's stdio
func (daemon *Daemon) ContainerAttachRaw(prefixOrName string, stdin io.ReadCloser, stdout, stderr io.Writer, doStream bool, attached chan struct{}) error {
	ctr, err := daemon.GetContainer(prefixOrName)
	if err != nil {
		return err
	}
	cfg := stream.AttachConfig{
		UseStdin:   stdin != nil,
		UseStdout:  stdout != nil,
		UseStderr:  stderr != nil,
		TTY:        ctr.Config.Tty,
		CloseStdin: ctr.Config.StdinOnce,
	}
	ctr.StreamConfig.AttachStreams(&cfg)
	close(attached)
	if cfg.UseStdin {
		cfg.Stdin = stdin
	}
	if cfg.UseStdout {
		cfg.Stdout = stdout
	}
	if cfg.UseStderr {
		cfg.Stderr = stderr
	}

	return daemon.containerAttach(context.TODO(), ctr, &cfg, false, doStream)
}

func (daemon *Daemon) containerAttach(ctx context.Context, c *container.Container, cfg *stream.AttachConfig, logs, doStream bool) error {
	if logs {
		logDriver, logCreated, err := daemon.getLogger(c)
		if err != nil {
			return err
		}
		if logCreated {
			defer func() {
				if err = logDriver.Close(); err != nil {
					log.G(context.TODO()).Errorf("Error closing logger: %v", err)
				}
			}()
		}
		cLog, ok := logDriver.(logger.LogReader)
		if !ok {
			return logger.ErrReadLogsNotSupported{}
		}
		logs := cLog.ReadLogs(logger.ReadConfig{Tail: -1})
		defer logs.ConsumerGone()

	LogLoop:
		for {
			select {
			case msg, ok := <-logs.Msg:
				if !ok {
					break LogLoop
				}
				if msg.Source == "stdout" && cfg.Stdout != nil {
					cfg.Stdout.Write(msg.Line)
				}
				if msg.Source == "stderr" && cfg.Stderr != nil {
					cfg.Stderr.Write(msg.Line)
				}
			case err := <-logs.Err:
				log.G(context.TODO()).Errorf("Error streaming logs: %v", err)
				break LogLoop
			}
		}
	}

	daemon.LogContainerEvent(c, events.ActionAttach)

	if !doStream {
		return nil
	}

	if cfg.Stdin != nil {
		r, w := io.Pipe()
		go func(stdin io.ReadCloser) {
			defer w.Close()
			defer log.G(context.TODO()).Debug("Closing buffered stdin pipe")
			io.Copy(w, stdin)
		}(cfg.Stdin)
		cfg.Stdin = r
	}

	if !c.Config.OpenStdin {
		cfg.Stdin = nil
	}

	if c.Config.StdinOnce && !c.Config.Tty {
		// Wait for the container to stop before returning.
		waitChan := c.Wait(ctx, container.WaitConditionNotRunning)
		defer func() {
			<-waitChan // Ignore returned exit code.
		}()
	}

	// We don't want to keep an indefinitely long tracing span, instead we create
	// A new span at an interval until the function returns and end the original one.
	// The final span should have any error that may have returned from the stream
	// copier.
	// The context handling here gets a little weird due to the use of `c.AttachContext()`.
	// We won't get any of the debugging messages from `CopyStreams` in our span.
	// Maye be worth improving this in the future.
	// For now this is fixing a very real problem with how spans are handled for
	// attaches and the debugging in the span was already a problem before.
	// Note that the span heirarchy is preserved here across each new span.

	done := make(chan error, 1)
	defer close(done)

	go func() {
		origCtx := ctx
		ctx, span := tracing.StartSpan(ctx, "daemon.container.attach.stream")
		span.SetAttributes(attribute.String("container", c.ID))

		tracing.SpanFromContext(origCtx).End()

		timer := time.NewTimer(time.Minute)
		defer func() {
			if !timer.Stop() {
				<-timer.C
			}
			span.End()
		}()

		for {
			select {
			case err := <-done:
				span.SetStatus(err)
				return
			case <-timer.C:
				last := span
				ctx, span = tracing.StartSpan(ctx, "daemon.container.attach.stream")
				span.SetAttributes(attribute.String("container", c.ID))
				last.SetAttributes(attribute.Bool("more", true))
				last.End()
				timer.Reset(time.Minute)
			}
		}
	}()

	actx := c.AttachContext()
	err := <-c.StreamConfig.CopyStreams(actx, cfg)
	if err != nil {
		var ierr term.EscapeError
		if errors.Is(err, context.Canceled) || errors.As(err, &ierr) {
			daemon.LogContainerEvent(c, events.ActionDetach)
		} else {
			done <- err
			log.G(actx).Errorf("attach failed with error: %v", err)
		}
	}

	return nil
}

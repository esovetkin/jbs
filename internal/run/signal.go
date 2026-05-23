package run

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

var signalExit = os.Exit

func withSignals(parent context.Context, beforeHardExit func()) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	sigs := make(chan os.Signal, 2)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	go func() {
		_, ok := <-sigs
		if !ok {
			return
		}
		cancel()
		_, ok = <-sigs
		if ok {
			if beforeHardExit != nil {
				beforeHardExit()
			}
			signalExit(130)
		}
	}()
	return ctx, func() {
		signal.Stop(sigs)
		close(sigs)
		cancel()
	}
}

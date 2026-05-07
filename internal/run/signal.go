package run

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

func withSignals(parent context.Context) (context.Context, context.CancelFunc) {
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
			os.Exit(130)
		}
	}()
	return ctx, func() {
		signal.Stop(sigs)
		close(sigs)
		cancel()
	}
}

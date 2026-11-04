// content-archiver consumes encrypted Kafka fragments and archives them outside
// the gateway's request and accounting paths.
package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/contentarchive"
)

func main() {
	if err := run(); err != nil {
		// Configuration and driver errors may contain connection credentials.
		// Deliberately emit no wrapped error or configuration values.
		common.SysError(common.LogText("Content archive worker stopped before acknowledging pending records; check configuration and dependency health"))
		os.Exit(1)
	}
}

func run() error {
	cfg, err := contentarchive.ConfigFromEnv()
	if err != nil {
		return err
	}
	if !cfg.Enabled() {
		common.SysLog(common.LogText("Content archive worker is disabled; configure CONTENT_ARCHIVE_BROKERS to start it"))
		return nil
	}
	index, err := contentarchive.OpenIndex(cfg.IndexDSN)
	if err != nil {
		return err
	}
	defer index.Close()
	objects, err := contentarchive.NewS3Store(cfg)
	if err != nil {
		return err
	}
	worker, err := contentarchive.NewArchiver(cfg, index, objects)
	if err != nil {
		return err
	}
	defer worker.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	common.SysLog(common.LogText("Content archive worker started"))
	if err := worker.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}

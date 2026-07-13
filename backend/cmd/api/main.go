package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	apiapp "caiyun/internal/app/api"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := apiapp.Run(ctx, os.Args[1:]); err != nil {
		if err == flag.ErrHelp {
			return
		}
		log.Printf("启动失败: %v", err)
		os.Exit(1)
	}
}

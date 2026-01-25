package main

import (
	"context"
	"log"

	"github.com/mistakeknot/autarch/internal/pollard/cli"
	"github.com/mistakeknot/autarch/internal/pollard/intermute"
)

func main() {
	if stop, err := intermute.Start(context.Background()); err != nil {
		log.Printf("intermute registration failed: %v", err)
	} else if stop != nil {
		defer stop()
	}
	cli.Execute()
}

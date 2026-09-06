package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/mistakeknot/autarch/internal/reviewagent"
	"github.com/mistakeknot/autarch/pkg/review"
	"github.com/spf13/cobra"
)

func acknowledgeReviewBuild() {
	if len(os.Args) > 1 && os.Args[1] == "review-controller" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := review.AcknowledgeRetestInvocation(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "Retest invocation was not recorded:", err)
	}
}

func reviewControllerCmd() *cobra.Command {
	return &cobra.Command{Use: "review-controller", Hidden: true, RunE: func(cmd *cobra.Command, args []string) error {
		store, err := review.Open(review.DefaultDir())
		if err != nil {
			return err
		}
		server, err := review.Listen(review.DefaultSocket(), store)
		if err != nil {
			return err
		}
		defer server.Close()
		engine := reviewagent.New(store)
		go engine.Run(context.Background())
		go store.RunRetention(context.Background())
		exe, _ := os.Executable()
		clavainBin := filepath.Join(filepath.Dir(exe), "clavain-cli")
		if _, err := os.Stat(clavainBin); err != nil {
			clavainBin = ""
		}
		go reviewagent.RunExecution(context.Background(), store, clavainBin)
		server.OnQuery = func(r review.Request) review.Response {
			if strings.HasPrefix(r.Method, "auth.") {
				response := engine.Auth(r)
				pending := response.Auth != nil && response.Auth.Operation != nil && response.Auth.Operation.Status == "pending"
				if (r.Method == "auth.login" || r.Method == "auth.providers" && pending) && response.Error == "" {
					app := filepath.Join(filepath.Dir(exe), "AutarchCapture.app")
					if err := exec.Command("open", "-a", app).Run(); err != nil {
						response.Error = "Open AutarchCapture to view the provider sign-in instructions"
					}
				}
				return response
			}
			if r.Method == "trace" {
				return reviewagent.Trace(store, r)
			}
			return review.LaunchRetest(store, r)
		}
		server.OnRequest = func(r review.Request) {
			if r.Method == "runtime.cancel" {
				engine.Handle(r)
			}
			if r.Method == "capture.command" && (r.Text == "open" || r.Text == "voice" || r.Text == "play") {
				exe, err := os.Executable()
				if err != nil {
					fmt.Fprintln(os.Stderr, err)
					return
				}
				app := filepath.Join(filepath.Dir(exe), "AutarchCapture.app")
				if _, err = os.Stat(app); err != nil {
					fmt.Fprintln(os.Stderr, "Capture companion missing:", app)
					return
				}
				if err = exec.Command("open", "-a", app).Run(); err != nil {
					fmt.Fprintln(os.Stderr, "Open companion:", err)
				}
			}
		}
		return server.Serve()
	}}
}

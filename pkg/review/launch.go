package review

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func quoteShell(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }

// The launched binary itself acknowledges its hash, rather than recording an
// invocation merely because Terminal accepted an open request.
func LaunchRetest(store *Store, r Request) Response {
	fail := func(err error) Response { return Response{Version: Version, Error: err.Error()} }
	e, ok := store.Snapshot().Executions[r.Target]
	if !ok || e.Project != r.Project || e.Status != "ready_for_retest" || e.Build != r.Text {
		return fail(fmt.Errorf("retest selection no longer matches prepared build"))
	}
	data, err := os.ReadFile(e.Binary)
	if err != nil {
		return fail(err)
	}
	sum := sha256.Sum256(data)
	if !strings.HasSuffix(e.Build, fmt.Sprintf(":sha256:%x", sum)) {
		return fail(fmt.Errorf("prepared binary changed; rebuild before retest"))
	}
	dir := filepath.Join(store.Dir(), "launches")
	if err = os.MkdirAll(dir, 0700); err != nil {
		return fail(err)
	}
	script := filepath.Join(dir, NewID()+".command")
	text := "#!/bin/sh\ncd " + quoteShell(e.Project) + " || exit 1\nAUTARCH_RETEST_EXECUTION=" + quoteShell(e.ID) + " AUTARCH_RETEST_BUILD=" + quoteShell(e.Build) + " exec " + quoteShell(e.Binary) + "\n"
	if err = os.WriteFile(script, []byte(text), 0700); err != nil {
		return fail(err)
	}
	if err = exec.Command("open", "-a", "Terminal", script).Run(); err != nil {
		return fail(err)
	}
	return Response{Version: Version, ID: r.Target}
}

func AcknowledgeRetestInvocation(ctx context.Context) error {
	id, build := os.Getenv("AUTARCH_RETEST_EXECUTION"), os.Getenv("AUTARCH_RETEST_BUILD")
	if id == "" || build == "" {
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(exe)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	if !strings.HasSuffix(build, fmt.Sprintf(":sha256:%x", sum)) {
		return fmt.Errorf("invoked binary differs from prepared retest")
	}
	project, err := os.Getwd()
	if err != nil {
		return err
	}
	_, err = (Client{}).Call(ctx, Request{Method: "execution.invoked", Project: project, Target: id, Text: build})
	return err
}

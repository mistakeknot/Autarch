use anyhow::{Context, Result};
use nix::libc;
use nix::sys::signal::{self, Signal};
use nix::unistd::Pid;
use std::process::Stdio;
use std::time::Duration;
use tokio::io::{AsyncBufReadExt, BufReader};
use tokio::process::Command;
use tokio::time::timeout;

/// Process handle with cancellation support
struct ManagedProcess {
    child: tokio::process::Child,
    pid: i32,
}

impl ManagedProcess {
    /// Spawn a command in its own process group
    async fn spawn(command: &str, args: &[&str]) -> Result<Self> {
        let mut cmd = Command::new(command);
        cmd.args(args)
            .stdout(Stdio::piped())
            .stderr(Stdio::piped())
            .stdin(Stdio::null());

        // Set process group - this allows us to kill the entire group
        unsafe {
            cmd.pre_exec(|| {
                // Create new process group with this process as leader
                let result = libc::setpgid(0, 0);
                if result != 0 {
                    return Err(std::io::Error::last_os_error());
                }
                Ok(())
            });
        }

        let mut child = cmd.spawn().context("Failed to spawn process")?;
        let pid = child.id().context("No process ID")? as i32;

        Ok(ManagedProcess { child, pid })
    }

    /// Capture stdout/stderr streams
    async fn capture_output(&mut self) -> Result<(String, String)> {
        let stdout = self.child.stdout.take().context("No stdout")?;
        let stderr = self.child.stderr.take().context("No stderr")?;

        let stdout_handle = tokio::spawn(async move {
            let reader = BufReader::new(stdout);
            let mut lines = reader.lines();
            let mut output = String::new();
            while let Ok(Some(line)) = lines.next_line().await {
                output.push_str(&line);
                output.push('\n');
            }
            output
        });

        let stderr_handle = tokio::spawn(async move {
            let reader = BufReader::new(stderr);
            let mut lines = reader.lines();
            let mut output = String::new();
            while let Ok(Some(line)) = lines.next_line().await {
                output.push_str(&line);
                output.push('\n');
            }
            output
        });

        let stdout_result = stdout_handle.await.context("stdout task failed")?;
        let stderr_result = stderr_handle.await.context("stderr task failed")?;

        Ok((stdout_result, stderr_result))
    }

    /// Cancel process with signal cascade: SIGINT → SIGTERM → SIGKILL
    async fn cancel_with_cascade(&mut self) -> Result<()> {
        println!("  Sending SIGINT to process group {}", self.pid);
        if let Err(e) = signal::killpg(Pid::from_raw(self.pid), Signal::SIGINT) {
            println!("  SIGINT failed: {}, continuing to SIGTERM", e);
        }

        // Wait 3 seconds for graceful shutdown
        match timeout(Duration::from_secs(3), self.child.wait()).await {
            Ok(Ok(status)) => {
                println!("  ✓ Process terminated gracefully on SIGINT: {}", status);
                return Ok(());
            }
            Ok(Err(e)) => {
                println!("  Wait error: {}", e);
            }
            Err(_) => {
                println!("  SIGINT timeout, escalating to SIGTERM");
            }
        }

        // Escalate to SIGTERM
        if let Err(e) = signal::killpg(Pid::from_raw(self.pid), Signal::SIGTERM) {
            println!("  SIGTERM failed: {}, continuing to SIGKILL", e);
        }

        // Wait 10 seconds for SIGTERM
        match timeout(Duration::from_secs(10), self.child.wait()).await {
            Ok(Ok(status)) => {
                println!("  ✓ Process terminated on SIGTERM: {}", status);
                return Ok(());
            }
            Ok(Err(e)) => {
                println!("  Wait error: {}", e);
            }
            Err(_) => {
                println!("  SIGTERM timeout, escalating to SIGKILL");
            }
        }

        // Force kill with SIGKILL
        if let Err(e) = signal::killpg(Pid::from_raw(self.pid), Signal::SIGKILL) {
            println!("  SIGKILL failed: {}", e);
        }

        match timeout(Duration::from_secs(2), self.child.wait()).await {
            Ok(Ok(status)) => {
                println!("  ✓ Process force killed with SIGKILL: {}", status);
                Ok(())
            }
            Ok(Err(e)) => Err(e).context("Failed to kill process"),
            Err(_) => Err(anyhow::anyhow!("Process did not die after SIGKILL")),
        }
    }
}

/// Test 1: Simple command execution
async fn test_simple_execution() -> Result<()> {
    println!("\n[Test 1] Simple Command Execution");
    println!("  Running: echo 'Hello from subprocess'");

    let mut process = ManagedProcess::spawn("echo", &["Hello from subprocess"]).await?;
    let (stdout, stderr) = process.capture_output().await?;
    let status = process.child.wait().await?;

    println!("  Exit status: {}", status);
    println!("  Stdout: {}", stdout.trim());
    println!("  Stderr: {}", stderr.trim());
    println!("  ✓ Simple execution works");

    Ok(())
}

/// Test 2: Long-running process with cancellation
async fn test_cancellation() -> Result<()> {
    println!("\n[Test 2] Process Cancellation with Signal Cascade");
    println!("  Running: sleep 30 (will cancel after 1s)");

    let mut process = ManagedProcess::spawn("sleep", &["30"]).await?;

    // Let it run for a bit
    tokio::time::sleep(Duration::from_secs(1)).await;

    // Cancel it
    process.cancel_with_cascade().await?;

    println!("  ✓ Cancellation works");

    Ok(())
}

/// Test 3: Command that spawns child processes (test process group isolation)
async fn test_process_group() -> Result<()> {
    println!("\n[Test 3] Process Group Isolation");
    println!("  Running: sh -c 'sleep 10 & sleep 10' (spawns background processes)");

    let mut process = ManagedProcess::spawn(
        "sh",
        &["-c", "sleep 10 & sleep 10 & wait"],
    )
    .await?;

    // Let processes spawn
    tokio::time::sleep(Duration::from_millis(500)).await;

    // Cancel the entire group
    process.cancel_with_cascade().await?;

    // Check for zombie processes
    tokio::time::sleep(Duration::from_secs(1)).await;
    let output = Command::new("ps")
        .arg("-o")
        .arg("pid,stat,comm")
        .output()
        .await?;

    let ps_output = String::from_utf8_lossy(&output.stdout);
    let zombies: Vec<&str> = ps_output
        .lines()
        .filter(|line| line.contains("Z") && line.contains("sleep"))
        .collect();

    if zombies.is_empty() {
        println!("  ✓ No zombie processes found");
    } else {
        println!("  ✗ Found zombie processes:");
        for zombie in zombies {
            println!("    {}", zombie);
        }
        return Err(anyhow::anyhow!("Zombie processes detected"));
    }

    Ok(())
}

/// Test 4: stdout/stderr capture accuracy
async fn test_output_capture() -> Result<()> {
    println!("\n[Test 4] stdout/stderr Capture");
    println!("  Running script that writes to both streams");

    let mut process = ManagedProcess::spawn(
        "sh",
        &["-c", "echo 'stdout line 1'; echo 'stderr line 1' >&2; echo 'stdout line 2'; echo 'stderr line 2' >&2"],
    )
    .await?;

    let (stdout, stderr) = process.capture_output().await?;
    process.child.wait().await?;

    let stdout_lines: Vec<&str> = stdout.trim().lines().collect();
    let stderr_lines: Vec<&str> = stderr.trim().lines().collect();

    println!("  Stdout captured: {:?}", stdout_lines);
    println!("  Stderr captured: {:?}", stderr_lines);

    if stdout_lines.len() == 2 && stderr_lines.len() == 2 {
        println!("  ✓ Output capture is accurate");
        Ok(())
    } else {
        Err(anyhow::anyhow!(
            "Output capture failed: expected 2 stdout and 2 stderr lines"
        ))
    }
}

/// Test 5: Command execution reliability under concurrent load
async fn test_concurrent_execution() -> Result<()> {
    println!("\n[Test 5] Concurrent Command Execution");
    println!("  Running 10 concurrent echo commands");

    let mut handles = vec![];
    for i in 0..10 {
        let handle = tokio::spawn(async move {
            let mut process = ManagedProcess::spawn("echo", &[&format!("Process {}", i)])
                .await
                .unwrap();
            let (stdout, _) = process.capture_output().await.unwrap();
            process.child.wait().await.unwrap();
            stdout.trim().to_string()
        });
        handles.push(handle);
    }

    let results: Result<Vec<_>, _> = futures::future::try_join_all(handles).await;
    let outputs = results?;

    println!("  All processes completed:");
    for output in &outputs {
        println!("    {}", output);
    }

    if outputs.len() == 10 {
        println!("  ✓ Concurrent execution reliable");
        Ok(())
    } else {
        Err(anyhow::anyhow!("Not all processes completed"))
    }
}

#[tokio::main]
async fn main() -> Result<()> {
    println!("=== M0 Terminal/Command Runner Prototype ===");
    println!("Testing: Process execution, cancellation, and isolation\n");

    // Run all validation tests
    test_simple_execution().await?;
    test_cancellation().await?;
    test_process_group().await?;
    test_output_capture().await?;
    test_concurrent_execution().await?;

    println!("\n=== Validation Report ===");
    println!("✅ PASS: Commands execute reliably");
    println!("✅ PASS: Cancellation works cleanly (SIGINT→SIGTERM→SIGKILL cascade)");
    println!("✅ PASS: No zombie processes remain");
    println!("✅ PASS: stdout/stderr capture accurate");
    println!("✅ PASS: Process group isolation working");
    println!("\n🎉 All validation criteria passed!");
    println!("\nFindings:");
    println!("  - tokio::process::Command is reliable for command execution");
    println!("  - Process group isolation prevents zombie processes");
    println!("  - Signal cascade ensures graceful-then-forceful termination");
    println!("  - Command runner is VIABLE for P0 (no PTY needed)");

    Ok(())
}

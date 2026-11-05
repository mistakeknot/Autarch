# Live systemd tests (build tag `homerunlive`)

`live_test.go` exercises the real `CLISystemd`, `ExecRunner`, `FSCgroup` and `RootLogs` against a guest's
own system systemd, as root. It is review evidence, never a CI gate.

Run it in a throwaway VM (no KVM needed, TCG is enough; user-mode networking or none):

1. Cross-build: `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go test -c -tags homerunlive -o live.test ./internal/homerun`
2. Build the NoCloud seed: `livevm/build-seed.sh live.test seed.iso 'TestLive'`
3. Boot an Ubuntu cloud image (verify it against its GPG-signed SHA256SUMS first) with the seed as a
   second drive, a qcow2 overlay, and one 9p share named `out`; the guest runs `guest.sh`, writes
   `test.log`, `obs.txt` and `exit.txt` to the share and powers off.

Observations are printed as `OBS <test> <key> = <value>` lines.

## Recorded run (Ubuntu 24.04.5, kernel 6.8.0, systemd 255.4-1ubuntu8.18, cgroup v2)

Image `noble-server-cloudimg-amd64.img`, sha256 `04e8f2461a16a44e78a370284471f29616cdd9baf1893df25807b71f985b6c94`.
All `TestLive*` pass, about 270 s wall time per boot under TCG with 8 vCPUs and 3 GiB.

Contradictions found and fixed in the launcher (each failed live first):

- `InaccessiblePaths` entries that do not exist made every unit fail with 226/NAMESPACE; they now carry the `-` prefix.
- `ProtectSystem=strict` left `/home` writable: added `ProtectHome=read-only`.
- `NoExecPaths=/` left the private `/tmp` and the `ReadWritePaths` directories executable: they are now listed.

Corrected assumptions: `StandardOutput=file:` truncates a precreated log (owner and mode are kept).
Residual risk: see `checkTrustedChain` (service-owned path inside the writes root can be swapped before systemd resolves it).

One run of `TestLiveMainProcessTimeout` recovered UNCERTAIN ("finished after it was recorded"); it did not
recur in three further runs, and is unexplained (guest clock suspected).

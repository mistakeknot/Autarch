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
- `InaccessiblePaths=-/run/docker.sock` (and the like) skips a path that is absent at launch, so a socket created mid-run was reachable. `TemporaryFileSystem=/run:ro,...` now masks the whole runtime directory (`/var/run` is a symlink to it); `systemctl show` confirms it. `TestLiveRuntimeSocketsCreatedAfterLaunch` creates five socket paths (docker, containerd, podman, dbus; `/var/run/docker.sock` is the same socket as `/run/docker.sock`) from outside while the unit runs, checks each accepts a connection from outside the unit (control), then requires every probe inside to be denied. It failed first without the property (all REACHED, 40 `/run` entries) and passes with it (1 entry, `/run` unwritable).
- `NoExecPaths=/` left the private `/tmp` and the `ReadWritePaths` directories executable: they are now listed.

Corrected assumptions: `StandardOutput=file:` truncates a precreated log (owner and mode are kept).
Residual risk: see `checkTrustedChain` (service-owned path inside the writes root can be swapped before systemd resolves it).

One run of `TestLiveMainProcessTimeout` recovered UNCERTAIN ("finished after it was recorded"); it did not
recur in three further runs, and is unexplained (guest clock suspected).

## Key loader run (`internal/homerun/review/key_live_test.go`)

Build with `go test -c -tags homerunlive -o live.test ./internal/homerun/review`, seed with run pattern
`TestLiveKey`, boot as above (user-mode networking off is enough). The tests run as root in the guest and
generate their keys there; nothing leaves the guest.

Recorded run (Ubuntu 24.04.5, kernel 6.8.0-147, systemd 255.4-1ubuntu8.18, same image sha256 as above): all
12 `TestLiveKey*` pass. `LoadKey` accepts a root-owned 0600/0400 key under root-owned directories (and a
root-owned sticky 1777 ancestor), and refuses: a key owned by another user, any mode with group or other
bits, an ancestor owned by another user (sticky or not), group- or world-writable non-sticky ancestors, a
symlink as the final component or as an ancestor, a FIFO at the key path or as an ancestor (no hang), a
character device and non-hex content. A 15 s race that swaps an ancestor for a symlink to a decoy tree loaded
the decoy key zero times (about 10,000 good loads, 17,000 refusals). No defect was found in `key.go`.

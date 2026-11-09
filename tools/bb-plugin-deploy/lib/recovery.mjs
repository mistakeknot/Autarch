// The printed last-resort recovery block (plan section on invariants). It needs nothing but bb, bash, flock, stat and
// git: one locked unit that takes the destination lock BEFORE the disable and holds it through the install, the enable,
// the verification and the atomic installed.sha write. Printed with real values on every deploy and rollback report.

const q = (s) => `'${String(s).replace(/'/g, `'\\''`)}'`;

/**
 * @param {{serverUrl:string, pluginId:string, anchoredBuild:string, pluginDirRel:string, stateDir:string, rollbackLeaves?:"enabled"|"disabled"}} a
 */
export function recoveryBlock({ serverUrl, pluginId, anchoredBuild, pluginDirRel, stateDir, rollbackLeaves = "enabled" }) {
  const enable = rollbackLeaves === "enabled";
  const id = q(pluginId);
  return [
    "bash -s <<'RECOVER'",
    "set -euo pipefail",
    `export BB_SERVER_URL=${q(serverUrl)}`,
    `B=${q(anchoredBuild)}; ST=${q(stateDir)}; L="$ST/lock"; P=${id}; D=${q(pluginDirRel)}`,
    'if [ -d "$ST" ]; then',
    "  # reuse an inherited lock descriptor: BPD_LOCK_FD if set, else fd 9; otherwise open our own",
    "  F=${BPD_LOCK_FD:-9}",
    "  case \"$F\" in ''|*[!0-9]*) F=9;; esac",
    '  if [ -e "/proc/self/fd/$F" ] && [ "$(stat -L -c %d:%i "/proc/self/fd/$F")" = "$(stat -c %d:%i "$L" 2>/dev/null)" ]; then :; else exec {F}>>"$L"; fi',
    "  W=${BPD_LOCK_WAIT_SEC:-2100}",
    "  case \"$W\" in ''|*[!0-9]*) W=2100;; esac",
    '  [ "$W" -le 2100 ] || W=2100',
    '  flock -w "$W" "$F" || { echo "lock busy after 35 min; see the skip variant"; exit 75; }',
    "else",
    '  echo "WARNING: no state dir $ST; recovering without the lock or the record write"',
    "fi",
    'bb plugin disable "$P"',
    'bb plugin install --yes "$B/$D"',
    ...(enable ? ['bb plugin enable "$P"'] : []),
    "# verify the source is the anchored build" + (enable ? " and that the plugin is running" : ""),
    "SRC=''",
    'while IFS= read -r line; do case "$line" in *"resolved: path:"*) SRC=${line#*resolved: path:};; esac; done < <(bb plugin source "$P")',
    '[ "$SRC" = "$B/$D" ] || { echo "VERIFY FAILED: source is $SRC; record not written"; exit 76; }',
    ...(enable
      ? [
          "UP=0",
          "for _ in $(seq 1 60); do",
          '  while IFS= read -r line; do case "$line" in "$P"@*" running"*) UP=1;; esac; done < <(bb plugin list)',
          '  [ "$UP" = 1 ] && break',
          "  sleep 1",
          "done",
          '[ "$UP" = 1 ] || { echo "VERIFY FAILED: $P is not running; record not written"; exit 76; }',
        ]
      : []),
    'if [ -d "$ST" ]; then',
    '  git -C "$B" rev-parse HEAD > "$ST/installed.sha.tmp.$$" && mv -f "$ST/installed.sha.tmp.$$" "$ST/installed.sha"',
    "fi",
    "RECOVER",
  ].join("\n");
}

/** The bare variant for mk only: no lock, so a runner tick can interleave. The risk is printed with it. */
export function bareRecovery({ serverUrl, pluginId, anchoredBuild, pluginDirRel, rollbackLeaves = "enabled" }) {
  return [
    "# RISK: this variant takes no lock, so a runner or tool tick can install something between these commands.",
    `export BB_SERVER_URL=${q(serverUrl)}`,
    `bb plugin disable ${q(pluginId)}`,
    `bb plugin install --yes ${q(`${anchoredBuild}/${pluginDirRel}`)}`,
    ...(rollbackLeaves === "enabled" ? [`bb plugin enable ${q(pluginId)}`] : []),
  ].join("\n");
}

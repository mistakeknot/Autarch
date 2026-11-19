import { chmodSync, writeFileSync } from "node:fs";
import { join } from "node:path";

// Every rig running the real helper must fail closed before reaching host services.
export function homeUiSafety(root: string) {
  const systemctl = join(root, "forbidden-systemctl");
  writeFileSync(systemctl, '#!/bin/sh\necho "TEST: forbidden systemctl hook reached" >&2\nexit 97\n');
  chmodSync(systemctl, 0o755);
  return {
    HOME_UI_SYSTEMCTL: systemctl,
    HOME_UI_PROC_ROOT: join(root, "fixture-proc"),
    AUTARCH_CARD_STATE_DIR: join(root, "fixture-filer"),
    XDG_STATE_HOME: join(root, "fixture-state"),
  };
}

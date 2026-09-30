export interface BuildFile { commit: string; plugin_dir: string; source_sha256: string; autarch_path: string; autarch_sha256: string }
export function buildIdentity(o: { repo: string; out: string; scratch?: string; skipNpmCi?: boolean }): BuildFile;

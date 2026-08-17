//! Zed extension: launch `card-lsp.py` for markdown buffers.
//!
//! This file is a launcher and nothing more. Every rule lives in
//! `card-check.py`; every position and code action lives in `card-lsp.py`.
//! Zed's extension API is WebAssembly with no filesystem or process access
//! beyond what the host grants, which makes it a poor place to put logic and a
//! fine place to put a `which`.
//!
//! It deliberately does NOT ship or download the server. Zed's own guidance is
//! that extensions "must not ship the language server as part of the
//! extension", and here that agrees with the dotfiles invariant: `card-lsp.py`
//! is deployed by `install-macos.sh` / `install-server.sh` next to the checker
//! it imports. A second copy vendored into the extension would be a second
//! rule implementation arriving by the back door -- the exact thing the whole
//! design refuses -- and it would be the copy that goes stale, because nothing
//! runs its tests.

use zed_extension_api::{self as zed, Command, LanguageServerId, Result, Worktree};

struct ProductCardExtension;

/// Where `install-macos.sh` and `install-server.sh` link the server.
const RELATIVE_PATH: &str = ".local/bin/card-lsp.py";
const BINARY: &str = "card-lsp.py";

impl zed::Extension for ProductCardExtension {
    fn new() -> Self {
        Self
    }

    fn language_server_command(
        &mut self,
        _id: &LanguageServerId,
        worktree: &Worktree,
    ) -> Result<Command> {
        // `which` and nothing else.
        //
        // The obvious-looking fallback -- stat ~/.local/bin/card-lsp.py when
        // PATH misses -- compiles cleanly against wasm32-wasip2 and then fails
        // at runtime, because WASI grants no preopens outside the worktree and
        // `std::env::var("HOME")` reads an environment Zed does not populate.
        // It would have looked like working code and behaved like a silent
        // dead end, which is worse than not having it.
        //
        // It is also unnecessary. Zed resolves `which` against the worktree's
        // SHELL environment, so it already answers the case the fallback was
        // written for: Zed launched from Finder inherits launchd's PATH rather
        // than the login shell's, and a plain PATH probe would miss a
        // stow-managed install that the shell can see perfectly well.
        let env = worktree.shell_env();
        if let Some(path) = worktree.which(BINARY) {
            return Ok(Command {
                command: path,
                args: vec![],
                env,
            });
        }

        // Name the expected location even though we could not check it. "Language
        // server failed to start" with no path in it sends you to the wrong half
        // of the system, and this server's absence is already quiet enough: a card
        // with no server looks exactly like a card with no problems.
        Err(format!(
            "{BINARY} is not on the shell PATH. It should be at ~/{RELATIVE_PATH}; \
             deploy it with dotfiles install-macos.sh (or install-server.sh)."
        ))
    }
}

zed::register_extension!(ProductCardExtension);

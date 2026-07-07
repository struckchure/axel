//! Axel Zed extension.
//!
//! Provides the language server for `.asl` schemas and `.aql` queries by
//! launching the system-installed `axel` CLI as `axel lsp`. Syntax highlighting
//! is handled separately by the tree-sitter grammars declared in extension.toml.

use zed_extension_api::{
    self as zed, settings::LspSettings, Command, LanguageServerId, Result, Worktree,
};

struct AxelExtension;

impl AxelExtension {
    /// Resolve the `axel` binary: an explicit `binary.path` in the LSP settings
    /// takes priority, otherwise it is looked up on the worktree's PATH.
    fn axel_binary(&self, id: &LanguageServerId, worktree: &Worktree) -> Result<String> {
        // Allow an override via Zed settings:
        //   "lsp": { "axel": { "binary": { "path": "/path/to/axel" } } }
        if let Ok(settings) = LspSettings::for_worktree(id.as_ref(), worktree) {
            if let Some(binary) = settings.binary {
                if let Some(path) = binary.path {
                    return Ok(path);
                }
            }
        }

        worktree.which("axel").ok_or_else(|| {
            "Axel: `axel` was not found on your PATH. Install the axel CLI (see \
             https://github.com/struckchure/axel) and make sure `axel` is on your PATH, \
             or set `lsp.axel.binary.path` in your Zed settings."
                .to_string()
        })
    }
}

impl zed::Extension for AxelExtension {
    fn new() -> Self {
        Self
    }

    fn language_server_command(
        &mut self,
        id: &LanguageServerId,
        worktree: &Worktree,
    ) -> Result<Command> {
        Ok(Command {
            command: self.axel_binary(id, worktree)?,
            args: vec!["lsp".to_string()],
            env: worktree.shell_env(),
        })
    }
}

zed::register_extension!(AxelExtension);

use zed_extension_api::{self as zed, LanguageServerId, Result};

/// SemanticShapeExtension starts semanticshape-lsp from the PATH - the language
/// server is a go binary of the walker repository
struct SemanticShapeExtension;

impl zed::Extension for SemanticShapeExtension {
    fn new() -> Self {
        SemanticShapeExtension
    }

    fn language_server_command(
        &mut self,
        _language_server_id: &LanguageServerId,
        worktree: &zed::Worktree,
    ) -> Result<zed::Command> {
        let command = worktree.which("semanticshape-lsp").ok_or_else(|| {
            "semanticshape-lsp is not on the PATH - install it from the walker repository with: go -C go install ./cmd/semanticshape-lsp".to_string()
        })?;
        Ok(zed::Command {
            command,
            args: vec![],
            env: worktree.shell_env(),
        })
    }
}

zed::register_extension!(SemanticShapeExtension);

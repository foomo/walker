use zed_extension_api::{self as zed, LanguageServerId, Result};

/// HtmlSchemaExtension starts htmlschema-lsp from the PATH - the language
/// server is a go binary of the walker repository
struct HtmlSchemaExtension;

impl zed::Extension for HtmlSchemaExtension {
    fn new() -> Self {
        HtmlSchemaExtension
    }

    fn language_server_command(
        &mut self,
        _language_server_id: &LanguageServerId,
        worktree: &zed::Worktree,
    ) -> Result<zed::Command> {
        let command = worktree.which("htmlschema-lsp").ok_or_else(|| {
            "htmlschema-lsp is not on the PATH - install it from the walker repository with: go install ./cmd/htmlschema-lsp".to_string()
        })?;
        Ok(zed::Command {
            command,
            args: vec![],
            env: worktree.shell_env(),
        })
    }
}

zed::register_extension!(HtmlSchemaExtension);

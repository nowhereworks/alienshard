# Security

## Public Release Guardrails

- Never commit or publish secrets, credentials, tokens, private environment values, personal data, local wiki contents, generated artifacts, logs, local binaries, or editor-local settings.
- Before committing, tagging, releasing, or publishing this repository, inspect staged, unstaged, untracked, and ignored files for non-public or sensitive data.
- Keep `.env.example` and documentation examples sanitized; use placeholders instead of real values.
- Do not include local `__wiki/` contents in commits or release artifacts.

## Reporting

If you find a security issue, report it privately through GitHub Security Advisories when available. If private reporting is unavailable, open a public issue without exploit details or sensitive data.

#!/usr/bin/env bash
# Sets up the Git pre-commit hook for the backend (Go formatter + linter).
# Run once from the repo root: bash precommit-setup.sh

set -euo pipefail

REPO_ROOT="$(git rev-parse --show-toplevel)"
HOOK_FILE="$REPO_ROOT/.git/hooks/pre-commit"


if ! command -v gofmt &>/dev/null; then
  echo "error: gofmt not found — install Go first" >&2
  exit 1
fi

if ! command -v golangci-lint &>/dev/null; then
  echo "golangci-lint not found — installing latest version..."
  curl -sSfL https://golangci-lint.run/install.sh | sh -s -- -b $(go env GOPATH)/bin v2.11.4
fi


cat > "$HOOK_FILE" <<'EOF'
#!/usr/bin/env bash
# Pre-commit: format check + lint for the Go backend (server/).
set -euo pipefail

REPO_ROOT="$(git rev-parse --show-toplevel)"
SERVER_DIR="$REPO_ROOT/server"

# Only run when Go files are staged.
STAGED=$(git diff --cached --name-only --diff-filter=ACM | grep '\.go$' || true)
if [[ -z "$STAGED" ]]; then
  exit 0
fi

echo "gofmt check..."
UNFORMATTED=$(gofmt -l $STAGED)
if [[ -n "$UNFORMATTED" ]]; then
  echo "error: the following files are not gofmt-formatted:" >&2
  echo "$UNFORMATTED" >&2
  echo "  run: gofmt -w $UNFORMATTED" >&2
  exit 1
fi

echo "golangci-lint..."
(cd "$SERVER_DIR" && golangci-lint run ./...)

echo "[OK] pre-commit checks passed"
EOF

chmod +x "$HOOK_FILE"
echo "Pre-commit hook installed at $HOOK_FILE"

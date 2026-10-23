#!/bin/bash
# PreToolUse (Edit|Write): block edits to files that must not be hand-edited.
# Exit 2 blocks the tool call; stderr is shown to Claude.
file=$(jq -r '.tool_input.file_path // empty')
[ -z "$file" ] && exit 0
rel="${file#"$CLAUDE_PROJECT_DIR"/}"
base=$(basename "$rel")

case "$base" in
  *.example) ;;
  .env|.env.*)
    echo "Blocked: $rel holds real secrets. Edit the matching .env*.example instead and tell the user which keys to set." >&2
    exit 2 ;;
esac

case "$rel" in
  frontend/pnpm-lock.yaml|backend/go.sum)
    echo "Blocked: $rel is generated. Use pnpm add/remove or go get / go mod tidy." >&2
    exit 2 ;;
  backend/internal/database/db/*.go)
    echo "Blocked: $rel is sqlc output. Edit internal/database/queries/*.sql or migrations, then run: cd backend && make sqlc" >&2
    exit 2 ;;
  frontend/src/generated/api.d.ts)
    echo "Blocked: $rel is generated. Edit backend/docs/openapi.yaml, then run: cd frontend && pnpm api:generate" >&2
    exit 2 ;;
esac
exit 0

@AGENTS.md

## Claude Code

Project setup checked into the repo (works the same in every app cloned from this boilerplate):

- **Skills**: `.claude/skills/*` are symlinks to `.agents/skills/*`, so Claude and other agents share one copy. Add a skill under `.agents/skills/<name>/SKILL.md`, then link it: `ln -s ../../.agents/skills/<name> .claude/skills/<name>`.
- **Hooks** (`.claude/settings.json`):
  - before an edit, `.claude/hooks/guard-edit.sh` blocks real `.env*` files, lock files / `go.sum`, sqlc output (`backend/internal/database/db`) and `frontend/src/generated/api.d.ts`; regenerate those instead.
  - after an edit, `.claude/hooks/format-edit.sh` runs `gofmt` on Go files and Prettier on frontend files.
- **MCP servers** (`.mcp.json`, approve once per machine): `shadcn` (component registry, runs in `frontend/`), `context7` (current library docs), `playwright` (browser checks).
- **Subagent**: `boilerplate-reviewer` checks a diff against the hard rules above before a PR.
- **Before saying done**: follow the `verify-changes` skill. Schema work: `db-migration` skill.
- **Shipping**: push a feature branch; CI's `auto-pr` job opens and merges a PR when backend + frontend pass. `[no-automerge]` in the head commit message keeps it open.

# Local artifact preservation

The following files are local engineering artifacts and must remain available in the worktree without being committed:

- `.history/**`
- `docs/review-2026-10-15*`
- `.pi/plans/**`

Preserve their contents when rebasing, rewriting history, or changing project files. Do not stage or commit them. Do not add these paths to `.gitignore`; their uncommitted status is intentional and must remain visible. Before history rewrites, stash or otherwise back up the artifacts and restore them afterward.

Do not include unrelated dirty or untracked work in cleanup commits.

# Repository Instructions

## Git And Pull Requests

- Use Conventional Commits 1.0.0 for all commit messages and pull request titles: <https://www.conventionalcommits.org/ja/v1.0.0/>
- Keep commit and pull request titles concise, with an appropriate type such as `feat`, `fix`, `docs`, `refactor`, `test`, `chore`, or `ci`.
- Before merging a pull request, check that there are no unresolved conversations because the repository ruleset enables "Require conversation resolution before merging".
- When a pull request has unresolved review conversations, resolve them on the AI side: address each comment (reply and/or push a fix), then mark the conversation as resolved using the `resolveReviewThread` GraphQL mutation via `gh api graphql`, so the pull request can satisfy the "Require conversation resolution before merging" ruleset.

## Repository Documentation

- If a directory you are working in contains a `README.md`, always read it before making changes in that directory.

## cmt Execution

- When asked to run `cmt apply` (or similar `cmt` commands), always scope execution to specific targets unless explicitly instructed to apply everything. Do not run `cmt` against all targets without an explicit instruction to do so.

---
name: forge-pr-review
description: How to publish a code review onto a GitHub / GitLab / Forgejo-Gitea pull/merge request as inline comments with one-click ```suggestion blocks. Read this before the publish_review step posts anything.
---

# Publishing a review onto a forge PR

You are turning a finished, merged finding set into ONE pull/merge-request
review: an inline comment per finding, anchored to `file:line` in the
PR's diff, with a one-click `suggestion` block when the finding carries a
concrete `replacement`. You only POST comments — never edit, fix, or
commit the workspace.

## 1. Detect the forge from the PR URL

Parse the URL host and path:

- `github.com` (or GitHub Enterprise host) → **GitHub**, use `gh`.
  `https://github.com/<owner>/<repo>/pull/<number>`
- host contains `gitlab` → **GitLab**, use `glab`.
  `https://gitlab.com/<group>/<project>/-/merge_requests/<iid>`
- otherwise assume **Forgejo / Gitea** (self-hosted) → REST API.
  `https://<host>/<owner>/<repo>/pulls/<index>`

### Unattended auth: the `forge_token` file secret

This run may be unattended (an org webhook launch) with no
pre-authenticated forge CLI on the host. If the mounted secret file
`/run/iterion/secrets/forge_token` EXISTS, authenticate the matching CLI
with it before posting — pass the path/value to the CLI, never read the
token into a prompt or echo it:
- GitHub: `gh auth login --with-token < /run/iterion/secrets/forge_token`
- GitLab: `glab auth login --hostname <host-from-pr_url> --stdin < /run/iterion/secrets/forge_token`
- Forgejo/Gitea: `export FORGEJO_TOKEN="$(cat /run/iterion/secrets/forge_token)"`

When the file is absent (manual/local runs), assume host auth.

Confirm the matching CLI is authenticated BEFORE building anything:
- GitHub: `gh auth status` (exit 0 = ok).
- GitLab: `glab auth status`.
- Forgejo/Gitea: a token in `$FORGEJO_TOKEN` / `$GITEA_TOKEN`, or
  `tea login list` showing the host.

Honour `pr_review_mode`: `summary` posts ONE rolled-up note (the safe
default for unattended webhook runs — no diff-position mapping);
`inline` posts one anchored comment per finding (section 2).

If the forge is unrecognised or the CLI is not authenticated, publish
NOTHING: return `published=false` with a precise `skipped_reason`
(e.g. `"gh not authenticated; run gh auth login"`). Do not pretend.

## 2. Anchoring rule (all forges)

An inline comment can only attach to a line that is part of the PR's
diff — i.e. an added or context line on the NEW side. Findings whose
`line` is not in the diff CANNOT take an inline comment: collect those
and list them in the review's summary body instead of dropping them.

`line` / `line_end` from a finding are 1-based line numbers in the new
file. A single-line anchor has `line == line_end`.

## 3. The `suggestion` block

When a finding has a non-empty `replacement`, the comment body must end
with a fenced suggestion block so the author can apply it in one click.
The fence keyword differs slightly per forge (below). The body shape:

```
**[<severity> · <category><· cross-confirmed if reviewers==both>]** <title>

<detail>

```suggestion
<replacement, verbatim — the exact new content for line..line_end>
```​
```

The suggestion REPLACES the anchored `line..line_end` range — its content
is the complete new text for those lines. A single-line anchor
(`line == line_end`) replaced by multi-line content is normal: most
one-click fixes are one line in, several out. Each forge carries the
replaced range in its own fence syntax (§4–§6) — derive it from the
finding's `line`/`line_end`, never guess.

When `replacement` is empty, post a plain comment: the bold header line,
`detail`, then `Suggested fix: <suggestion>` (the one-line sketch). No
fenced block.

## 4. GitHub (`gh`)

Build ONE review with all inline comments in a single API call. Write a
JSON payload to a temp file and submit it (avoids shell-quoting hell):

```sh
cat > /tmp/revi-review.json <<'JSON'
{
  "event": "COMMENT",
  "body": "<summary: totals, severity counts, cross-confirmed count, any unanchorable findings>",
  "comments": [
    { "path": "src/x.js", "line": 142, "side": "RIGHT", "body": "**[high · security]** ...\n\n```suggestion\nconst safe = sanitize(input);\n```" },
    { "path": "src/y.js", "start_line": 10, "line": 14, "start_side": "RIGHT", "side": "RIGHT", "body": "**[medium · correctness]** ..." }
  ]
}
JSON
gh api --method POST "repos/<owner>/<repo>/pulls/<number>/reviews" --input /tmp/revi-review.json
```

- Single-line comment: `line` + `side: "RIGHT"`.
- Multi-line: `start_line` + `line` (+ `start_side`/`side` = `"RIGHT"`);
  the suggestion block replaces `start_line..line`.
- `event: "COMMENT"` — Revi advises, it does NOT approve or
  request-changes (never gate the merge).
- If the call returns 422 for a comment whose line is not in the diff,
  remove that comment from the array, move its finding to the summary
  body, and retry. Better: pre-filter to lines you confirmed are in
  `gh api repos/<o>/<r>/pulls/<n>/files` patches.

VERIFY (mandatory): re-fetch and COUNT what the API stored —
```sh
gh api "repos/<owner>/<repo>/pulls/<number>/comments" --jq 'length'
gh api "repos/<owner>/<repo>/pulls/<number>/reviews" --jq '.[-1].html_url'
```
Report that count as `comments_posted` and the review URL as
`review_url`. Cite these calls in your `summary`.

## 5. GitLab (`glab`)

Inline comments are MR discussion threads carrying a `position`. Fetch the
diff refs ONCE (`:id` = URL-encoded `group/project`, or the numeric id):

```sh
glab api "projects/:id/merge_requests/<iid>/versions" --jq '.[0]'
```
Read `base_commit_sha`, `head_commit_sha`, `start_commit_sha` → BASE, HEAD,
START. Then post ONE discussion per finding with a NESTED `position` object.

**Do NOT use `glab api -f position[...]`** — glab serialises `position[base_sha]`
as a *literal flat* JSON key, never a nested object, so GitLab drops the
position and you silently get a plain note. Send real nested JSON via curl +
the forge token (host, URL-encoded `group/project`, and iid all come from the
`pr_url`):

```sh
TOKEN="$(cat /run/iterion/secrets/forge_token)"
jq -nc --arg body "$BODY" --arg bs "$BASE" --arg hs "$HEAD" --arg ss "$START" \
       --arg path "fetch.go" --argjson line 15 \
  '{body:$body, position:{position_type:"text", base_sha:$bs, head_sha:$hs,
    start_sha:$ss, new_path:$path, old_path:$path, new_line:$line}}' \
| curl -sS -H "PRIVATE-TOKEN: $TOKEN" -H 'Content-Type: application/json' \
    -X POST "https://<host>/api/v4/projects/<group%2Fproject>/merge_requests/<iid>/discussions" -d @-
```

Position rules (wrong → 400 or a dropped anchor):
- ALWAYS set BOTH `new_path` AND `old_path` to the SAME path — **even for a
  brand-new file**. Omitting `old_path` is the #1 reason an added line won't
  anchor.
- ADDED line (new file, or a `+` line): set ONLY `new_line` (no `old_line`).
- UNCHANGED context line: set BOTH `new_line` and `old_line` (same value when
  nothing above it shifted).

Loop over EVERY finding in ONE bash script; don't stop on the first error. If a
POST fails (400/422 — line not in the diff, or a bad position), do NOT
downgrade it to a plain note: collect it and list it under "could not be
anchored inline" in the summary. Capture each returned discussion id, and
VERIFY with the position-aware count below.

GitLab suggestion fence is `suggestion:-K+N`: it replaces `K` lines ABOVE
the anchored line plus `N` below. Anchor the discussion at the finding's
`line_end` (the position's `new_line`) and set `K = line_end - line`,
`N = 0` — so a single-line finding is `suggestion:-0+0`, a 3-line finding
(anchored at its last line) is `suggestion:-2+0`. The block body is the
full `replacement` (one line in, several out is fine):

```
```suggestion:-0+0
<replacement>
```​
```

After the per-finding discussions, post ONE summary note (totals, severity
counts, cross-confirmed count, and any unanchorable findings):
```sh
glab api --method POST "projects/:id/merge_requests/<iid>/notes" -f body="$SUMMARY"
```

VERIFY (mandatory): count the discussions that actually carry a diff position
— those are the inline comments GitLab stored — and capture the MR URL:
```sh
glab api "projects/:id/merge_requests/<iid>/discussions" --paginate \
  --jq '[.[].notes[] | select(.position != null)] | length'
```
Report that number as `comments_posted` (never an optimistic self-estimate);
`review_url` is the MR URL.

## 6. Forgejo / Gitea (REST API)

One review with comments, via the API (`tea` has no inline-review
command). Token from `$FORGEJO_TOKEN`/`$GITEA_TOKEN`:

```sh
curl -sS -X POST \
  -H "Authorization: token $FORGEJO_TOKEN" -H "Content-Type: application/json" \
  "https://<host>/api/v1/repos/<owner>/<repo>/pulls/<index>/reviews" \
  -d '{"event":"COMMENT","body":"<summary>","comments":[
       {"path":"src/x.js","new_position":142,"body":"**[high · security]** ...\n\n```suggestion\n<replacement>\n```"}
     ]}'
```
- `new_position` = line on the new side; use `old_position` for a
  removed line. Gita/Forgejo renders the same ```suggestion fence as
  GitHub.
- `event: "COMMENT"`.

VERIFY: `GET .../pulls/<index>/reviews` (and its `/comments`) and count.

## 7. Output contract

Return `publish_output`:
- `published`: true only if the forge accepted the review.
- `forge`: `"github" | "gitlab" | "forgejo" | "unknown"`.
- `review_url`: the posted review/thread URL (from the verify step).
- `comments_posted`: inline comments the forge STORED (from the re-fetch
  count — never an optimistic self-estimate).
- `suggestions_posted`: of those, how many carried a suggestion block.
- `skipped_reason`: non-empty when nothing was posted.
- `summary`: what you posted + the exact verify command(s) you ran and
  their result. A success claim without a verify call is a façade.

# jobqueue (iterion fixture — review-pr-mix)

A small worker-pool / job-queue used as a realistic codebase for
`iterion`'s `vibe_review_alternating` live test. The code contains a
deliberate mix of clean modules and ones with production-blocking
issues — the bot's reviewers are expected to surface the latter
without re-flagging the former.

**The issues are intentional. Do not "fix" this fixture casually.**
The point is that the reviewers find them on a live run; over-
sanitising the source removes signal from future runs.

## Layout

| Path | Responsibility |
|------|----------------|
| `main.go` | Entry point — spawns workers, enqueues jobs. |
| `queue/` | FIFO job queue backed by a buffered channel. |
| `worker/` | Pulls jobs and dispatches to a handler. |
| `auth/` | Bearer-token validation for internal tooling. |
| `storage/` | Blob-on-disk store for job payloads. |
| `config/` | Env-var-driven runtime configuration. |

## Build

```bash
go build ./...
go test ./...
```

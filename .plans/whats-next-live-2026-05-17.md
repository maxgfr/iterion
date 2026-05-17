# Board capabilities end-to-end — findings (2026-05-17)

Session de test ciblant `.plans/board-capabilities-next-steps.md`,
**Task A** (stdio smoke) puis amorce de **Task C** (whats-next +
conductor). La Task C n'a pas été poursuivie jusqu'au bout — pivot
sur le design d'une vue "Pilote" pour whats-next first-class.

## Ce qui a été fait

- **Task A — stdio smoke : PASS.** Bot minimal
  [bots/smoke/board_smoke.bot](../bots/smoke/board_smoke.bot)
  (≈30 lignes), 1 agent `claude_code` + `capabilities: [board.read,
  board.create, board.move]`. Run de 26s, 834 tokens, $0.06. Le
  subprocess `claude` voit bien `mcp__iterion_board__*` (7 MCP
  servers init, 64 tools), exécute 2 create_issue + 1
  transition_issue, et le board final affiche 2 issues dont 1
  transitionnée en `ready`.

- **Bug d'isolation trouvé + corrigé.** Avant le fix, le smoke
  test polluait le `.iterion/conductor/` du repo iterion alors que
  `--store-dir /tmp/iterion-board-smoke` était explicite. Cause :
  `model.WithStoreDir` était défini mais jamais appelé →
  `task.StoreDir` toujours vide → `__mcp-board` fallback sur
  `<cwd>/.iterion/conductor`. Fix appliqué et vérifié — re-run du
  smoke avec `--store-dir` honoré, isolation effective, host
  conductor non touché. Détails ci-dessous.

- **Phase 2 (Task C) — partiellement câblée, non exécutée.**
  Config conductor écrite (`/tmp/whats-next-conduct.yaml`,
  `assignee_workflows:` complet + `after_create` hook git worktree),
  editor + conductor up sur `:4891` (l'editor contient un conductor
  manager intégré, pas besoin du `iterion conduct` standalone).
  Le run whats-next n'a pas été lancé — pivot avant.

## Bug fixé — détails

**Symptôme.** `./iterion run <bot-with-board-caps> --store-dir <X>`
écrit le board dans `./.iterion/conductor/`, pas dans `<X>/conductor/`.

**Cause.** Chaîne :
1. CLI passe `storeDir` à `runview.BuildExecutor` (ExecutorSpec.StoreDir).
2. `BuildExecutor` ne forwarde **pas** vers
   `model.WithStoreDir(spec.StoreDir)`.
3. `ClawExecutor.storeDir` reste `""` → `task.StoreDir = ""`.
4. Le délégué `claude_code` ne pose pas `ITERION_STORE_DIR` dans
   l'env du subprocess `__mcp-board`.
5. `mcp_board.go::openBoardStoreFromEnv` fallback :
   `<cwd>/.iterion/conductor`.

**Fix.**
[pkg/runview/executor.go](../pkg/runview/executor.go) — calcule
`conductorStoreDir = filepath.Join(spec.StoreDir, "conductor")` et
passe via `model.WithStoreDir`. Le sub-path `/conductor` est
explicite parce que le contrat `task.StoreDir` (cf. godoc
[pkg/backend/delegate/delegate.go:115](../pkg/backend/delegate/delegate.go#L115))
est "conductor store root", pas "run-level store root".

Touche **tous** les call-sites de `BuildExecutor` :
[pkg/cli/run.go](../pkg/cli/run.go),
[pkg/cli/resume.go](../pkg/cli/resume.go),
[pkg/conductor/engine_runner.go](../pkg/conductor/engine_runner.go),
[pkg/runner/loop.go](../pkg/runner/loop.go),
[pkg/runview/service.go](../pkg/runview/service.go).

Tests `pkg/runview/...`, `pkg/backend/model/...`,
`pkg/backend/delegate/...` verts.

## Findings ouverts (non bloquants, à trier)

### F1 — Handoff plan stale sur le routing par assignee

`.plans/board-capabilities-next-steps.md` Task C "Known gaps to flag
back" mentionne *"il n'existe pas de router qui mappe
assignee → workflow"*. C'est **obsolète** : `RoutingRunner` et
`AssigneeWorkflows` existent déjà
([pkg/conductor/routing_runner.go](../pkg/conductor/routing_runner.go),
[pkg/conductor/config.go:29](../pkg/conductor/config.go#L29)),
documentés [docs/conductor.md:219-263](../docs/conductor.md#L219-L263).
Le handoff a été rédigé avant que ce code soit mergé. **Action** :
soit on supprime/corrige la section dans le handoff plan, soit on
laisse le handoff comme un instantané historique et on s'en remet
aux docs. Recommandation : corriger le handoff (faible coût).

### F2 — Mismatch `fields.bot_args` ↔ conductor template engine

Le prompt
[examples/whats-next/main.bot:477-484](../examples/whats-next/main.bot#L477-L484)
demande à emit_action d'encoder les args en chaîne CSV sous
`fields.bot_args = "--var,feature_prompt=..."` *et* annonce *"(The
conductor router will split on comma and emit --var flags...)"*. Ce
splitter **n'existe pas** : le template engine
([pkg/conductor/template.go](../pkg/conductor/template.go)) résout
`{{issue.fields.bot_args}}` comme une string entière, sans
split-comma. Conséquence : un bot dispatché reçoit la chaîne brute
"--var,feature_prompt=..." en valeur de var, jamais des vars
distinctes. **Workaround utilisé dans notre config** :
`feature_prompt: "{{issue.title}}\n\n{{issue.body}}"` — le body
est plus riche de toute façon. **Choix design à trancher** :
- Ajouter le split-comma dans le template engine (peu coûteux,
  préserve la promesse du prompt).
- Modifier emit_action_system pour stamper des `fields.<name>`
  typés directement (plus propre, casse moins de choses).
- Documenter qu'on ne supporte que les champs typés et déprécier
  la phrase mensongère dans le prompt.

### F3 — Conductor workspace exige un hook git pour les bots `worktree: auto`

`workspace.root` par défaut crée des sous-dirs vides
(`<store-dir>/conductor/workspaces/<issue-id>`). Les bots
dispatchés (vibe_feature_dev etc.) utilisent `worktree: auto` qui
**exige** un git context. Sans `after_create` hook clonant ou
worktreeant un repo, le dispatch échoue. **Recommandation** :
documenter ce prérequis dans
[docs/conductor.md](../docs/conductor.md) ou shipper un hook par
défaut quand l'opérateur ne fournit rien (avec un avertissement
explicite).

### F4 — `iterion conduct` vs `iterion editor` — un seul process suffit

`iterion editor` embarque déjà un conductor manager idle
([pkg/cli/editor.go:120-137](../pkg/cli/editor.go#L120-L137))
configurable + démarrable via `PUT/POST /api/v1/conductor/*`. Le
process standalone `iterion conduct` est utile pour des
déploiements headless mais en dev local il y a redondance, et le
SPA servi par `iterion conduct` n'expose pas le launcher de run
(d'où le `JSON.parse` qu'on a vu — l'UI conductor appelle un
endpoint que le serveur conduct ne fournit pas). **Action** :
améliorer la doc / les outputs des deux commandes pour clarifier
qui sert quoi.

### F5 — Conductor SPA et endpoint manquant — bug ou UX ?

Sur le SPA servi par `iterion conduct` (:7892), naviguer dans
l'UI provoque `JSON.parse: unexpected character at line 1 column
1` parce que le SPA appelle `/api/runs` (ou similaire) que le
serveur conduct **ne monte pas**
([pkg/cli/conduct.go:109-122](../pkg/cli/conduct.go#L109-L122)).
**À investiguer** : soit l'UI doit dégrader gracefully quand un
endpoint manque (afficher "feature unavailable" au lieu de planter
le rendering), soit `iterion conduct` doit monter le même set
d'endpoints que `iterion editor`. F4 + F5 sont liés.

### F6 — Modif debug locale dans `pkg/backend/model/claw_backend.go`

`generateTextWithToolsAndSchema` a été enrichi localement avec un
dump diagnostique de la première passe (text/messages/usage) pour
investiguer un mode d'échec "empty response". Conservé hors-commit
sur décision opérateur. À reviewer plus tard : soit l'inclure
proprement (avec un flag debug), soit nettoyer après que le mode
d'échec aura été élucidé.

## État local de la session

- Branche : `main`.
- Modifs non commitées :
  - `pkg/runview/executor.go` — **fix isolation** (à committer).
  - `pkg/backend/model/claw_backend.go` — debug logging
    (conservé hors-commit, cf. F6).
- Nouveau fichier non commité : `bots/smoke/board_smoke.bot`.
- Services tournants : conductor (idle, dans l'editor) +
  editor sur `:4891`. À stopper avant pivot.
- Répertoires temporaires : `/tmp/iterion-board-smoke`,
  `/tmp/whats-next-store`, `/tmp/whats-next-workspaces`,
  `/tmp/whats-next-conduct.yaml`. À nettoyer après commit.

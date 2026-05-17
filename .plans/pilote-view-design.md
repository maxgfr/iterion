# Pilote — design de la vue first-class pour whats-next

## Context

Aujourd'hui le bot `examples/whats-next/main.bot` (orchestrateur
"qu'est-ce qu'on fait ensuite ?") est lancé comme n'importe quel
workflow — `iterion run` en CLI ou LaunchView dans le SPA. Son
intérêt est pourtant transverse : c'est le point d'entrée naturel
pour décider quoi pousser sur le board / dispatcher via conductor.
On veut donc en faire un **first-class bot** avec une UI dédiée
("Pilote"), point d'entrée depuis le Home du SPA.

Décisions cadrées (session 2026-05-17) :
- **Nom** : Pilote (route `/pilote`, label SPA "Pilote").
- **First-class** : hardcodé dans le SPA (const TS, refactorable).
- **Interaction** : hybride — chat-bubbles pour les nœuds humains
  (`ask_priorities`, `human_review`), bandeau progress pour les
  nœuds agentiques (`explore`, `propose_roadmap`, `revise_roadmap`,
  `emit_action`).
- **Home** : carte pleine largeur en haut, au-dessus du grid
  RecentFiles + Runs actuel.
- **Fin de session** : reste sur le fil avec récap + liens
  `/board` et `/conductor`.
- **Reprise** : auto-attach à la session whats-next active la plus
  récente si présente.

## Architecture

Pas de nouveau endpoint backend pour v0. On consomme les APIs
existantes :
- `POST /api/runs` — lancement (workflow path + vars).
- `GET /api/runs?filter` — lister les runs (pour le auto-attach).
- WS run events — déjà utilisé par `RunView`.
- `POST /api/runs/{id}/resume` — soumettre réponse human.

Le mapping nœud → rendu est porté côté TS dans une const :

```ts
// editor/src/lib/pilote/firstClassBots.ts
export const FIRST_CLASS_BOTS = {
  whatsNext: {
    id: "whats-next",
    label: "What's Next",
    description: "Décide quoi faire ensuite sur ce repo.",
    workflowPath: "examples/whats-next/main.bot",
    nodeMap: {
      explore:         { kind: "banner", label: "Surveying repository…", summaryField: "summary" },
      ask_priorities:  { kind: "human",  prompt: "What matters right now?" },
      propose_roadmap: { kind: "banner", label: "Drafting roadmap…", followCardKind: "roadmap" },
      human_review:    { kind: "human",  prompt: "Review the proposed roadmap.",
                         actions: ["approve", "request_revision"] },
      revise_roadmap:  { kind: "banner", label: "Revising roadmap…", followCardKind: "roadmap" },
      carry_roadmap:   { kind: "silent" },   // compute node — pas de rendu
      emit_action:     { kind: "banner", label: "Creating kanban issues…",
                         followCardKind: "issuesSummary" },
    },
  },
} as const;
```

Couplage : si on renomme un nœud whats-next, ce fichier casse. Le
risque est borné (un seul bot, un seul fichier de mapping).

## File-level breakdown

### Nouveaux fichiers

| Fichier | Rôle |
|---|---|
| `editor/src/lib/pilote/firstClassBots.ts` | Registre const (mapping nœud→rendu, workflow path). |
| `editor/src/lib/pilote/useWhatsNextSession.ts` | Hook : auto-attach + launch + WS subscribe + state machine du fil. |
| `editor/src/components/Home/PiloteCard.tsx` | Carte home (titre, sous-titre, CTA "Démarrer une session"). |
| `editor/src/components/Pilote/PiloteView.tsx` | Shell route, gère les 2 modes (no-session / active-session). |
| `editor/src/components/Pilote/SessionLauncher.tsx` | UI "no-session" : bouton + auto-fill workspace_dir + auto-attach hint. |
| `editor/src/components/Pilote/ChatTranscript.tsx` | Liste virtualisée de messages (banner / human / roadmap / issues). |
| `editor/src/components/Pilote/NodeBanner.tsx` | Bandeau progress (spinner + label) pour nœud agentique en cours. |
| `editor/src/components/Pilote/RoadmapCard.tsx` | Rendu structuré du `roadmap` schema (sections long/short/next + recommended_bots). |
| `editor/src/components/Pilote/HumanChatTurn.tsx` | Bubble assistant + input user (réutilise `HumanInteractionField` autant que possible). |
| `editor/src/components/Pilote/IssuesSummaryCard.tsx` | Liste des issues créées par emit_action + liens /board /conductor. |

### Modifications

| Fichier | Changement |
|---|---|
| `editor/src/App.tsx` | Ajouter `<Route path="/pilote">` avant la route catch-all `/`. |
| `editor/src/components/Home/HomeView.tsx` | Insérer `<PiloteCard />` pleine largeur au-dessus du grid 2-col. |
| `editor/src/components/shared/AppHeader.tsx` (à vérifier) | Ajouter un onglet ou bouton "Pilote" si l'AppHeader contient une nav. |

## Build order (étapes incrémentales)

### Étape 1 — UI seule, mock data
**Objectif** : valider le look + la navigation avant tout wiring.

- Créer `firstClassBots.ts` (juste le registre).
- Créer `PiloteCard.tsx` + `HomeView` adapter.
- Créer `PiloteView.tsx` avec un état mock en dur :
  1 bandeau "Surveying…" terminé (devient summary card),
  1 bubble assistant ask_priorities,
  1 bubble user (réponse mock),
  1 bandeau "Drafting roadmap…" en cours,
  1 carte roadmap mock,
  1 bubble human_review en attente.
- Créer `NodeBanner.tsx`, `RoadmapCard.tsx`, `HumanChatTurn.tsx`,
  `ChatTranscript.tsx` minimaux.
- Wire `/pilote` dans `App.tsx`.

**Critère d'acceptation Étape 1** : route accessible, home card
visible, vue Pilote affiche le mock sans erreur console. Pas de
backend touché.

### Étape 2 — Wiring run launch + observation
**Objectif** : lancer un vrai run et voir les events arriver.

- Créer `useWhatsNextSession.ts` :
  - `launch(workspaceDir)` → POST /api/runs avec workflow whats-next.
  - `attach(runId)` → souscription WS.
  - State machine : empty → launching → running → paused (human) → finished.
- `SessionLauncher` : champ workspace_dir (pré-rempli avec
  `server_info.work_dir`), bouton "Démarrer".
- `ChatTranscript` consomme les events du hook au lieu du mock :
  - `node_started` → push un banner en cours.
  - `node_finished` → finalise le banner + push la carte de suivi.

**Critère d'acceptation Étape 2** : on lance whats-next depuis
Pilote, on voit le banner "Surveying…" s'animer puis fermer avec
le summary. La phase humaine `ask_priorities` apparaît en bubble
**non interactive** (l'envoi sera l'Étape 3).

### Étape 3 — Soumission des réponses humaines
**Objectif** : compléter le cycle interactif.

- `HumanChatTurn` réutilise `HumanInteractionField` (textarea
  contrôlée + bouton Send). Submit → POST resume avec la réponse.
- Tester le tour complet `ask_priorities` → `propose_roadmap` →
  carte roadmap.
- Mode `human_review` : ajouter les 2 actions "Approuver" / "Demander
  une révision" (la révision ouvre la textarea, l'approbation envoie
  `{ approved: true }`).

**Critère d'acceptation Étape 3** : on peut approuver et atteindre
`emit_action`.

### Étape 4 — Boucle review + rendu emit_action
**Objectif** : supporter `approval_loop(10)`.

- Quand `revise_roadmap` produit une nouvelle roadmap, l'afficher
  comme **nouvelle carte** dans le fil (pas remplacement).
- `IssuesSummaryCard` : liste des issues créées (depuis le
  `emit_action` output), liens /board, /conductor.

**Critère d'acceptation Étape 4** : session complète E2E sur le
repo iterion lui-même, board peuplé en fin.

### Étape 5 — Polish + reprise + tests
**Objectif** : robustesse session.

- localStorage : current `whatsNextRunId` (clé par projet).
- `useWhatsNextSession` au mount → tente auto-attach :
  - Si un run whats-next dans état actif existe : attach.
  - Sinon : afficher l'écran SessionLauncher avec un lien
    "Reprendre la dernière session (terminée)" si applicable.
- Tests unitaires sur le mapping events → messages.
- Eventuellement : indicateur "session en cours" dans `AppHeader`
  pour pouvoir revenir au Pilote depuis n'importe où.

**Critère d'acceptation Étape 5** : reload de page = reprise propre,
pas de doublons de messages, pas de perte d'état.

## Points d'attention identifiés

1. **Le schéma `roadmap`** : il faut le lire précisément
   (`examples/whats-next/main.bot`) pour le `RoadmapCard`. Les
   champs attendus : `long_term[]`, `short_term[]`, `next_action`,
   `recommended_bots[]`, plus `rationale`. À vérifier.
2. **Le `human_review` schema** (`review_output`) : `{ approved: bool,
   feedback: string }` — déterminer si le SPA doit envoyer toujours
   `feedback` (même chaîne vide) ou seulement à la révision.
3. **Le path de lancement** : `examples/whats-next/main.bot` est
   relatif au repo root. Le SPA doit le résoudre via
   `server_info.work_dir` (déjà exposé).
4. **Filtre runs auto-attach** : il faut un moyen côté API de
   filtrer `runs?workflow=examples/whats-next/main.bot&status=running`.
   Si l'endpoint /api/runs ne supporte pas le filtre, on filtre
   côté SPA après fetch.
5. **AppHeader** : on doit vérifier si la nav supérieure existe et
   peut accueillir un onglet "Pilote" ; sinon on s'en passe et on
   met l'accès via la home card uniquement (suffisant en v0).

## Hors scope v0

- Multi-bot first-class (Pilote ne sert que whats-next).
- Configuration côté SPA du `assignee_workflows:` du conductor
  (utile mais oblique).
- Plan-mode dans la chat (chat à propos d'un plan en cours).
- Templates de priorités ("ajouter X comme priorité récurrente").
- Resume après crash serveur (utilise localStorage + auto-attach,
  ce qui couvre déjà la plupart des cas).

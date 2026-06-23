---
name: rgaa-audit
description: Méthodologie d'audit d'accessibilité RGAA 4.1.2 (106 critères, 13 thèmes, base WCAG 2.1 AA) — workflow d'audit, scoring C/NC/NA, grille de priorités 🔴/🟠/🟡 et format du rapport de conformité. Lire cette skill avant toute revue ou correction d'accessibilité, et avant de rédiger un rapport d'audit RGAA.
---

> Source : méthodologie réécrite depuis etalab-ia/skills (licence MIT) — référentiel officiel https://accessibilite.numerique.gouv.fr/.

# RGAA 4.1.2 — méthode d'audit de conformité

Le RGAA (Référentiel Général d'Amélioration de l'Accessibilité) est le
référentiel d'accessibilité numérique français : **106 critères** répartis en
**13 thématiques**, basé sur **WCAG 2.1 niveau AA**. Cette skill est la grille de
référence ; le détail des critères vit dans les skills `rgaa-criteria-*` :

- `rgaa-criteria-images-couleurs` — thèmes 1-3 (Images, Cadres, Couleurs)
- `rgaa-criteria-multimedia-tableaux-liens-scripts` — thèmes 4-7 (Multimédia, Tableaux, Liens, Scripts)
- `rgaa-criteria-structure-presentation` — thèmes 8-10 (Éléments obligatoires, Structuration, Présentation)
- `rgaa-criteria-formulaires` — thème 11 (Formulaires)
- `rgaa-criteria-navigation-consultation` — thèmes 12-13 (Navigation, Consultation)

Quand le code cible utilise le Système de Design de l'État (DSFR), lire aussi
`rgaa-dsfr` pour exploiter le baseline d'accessibilité des composants officiels.

## Périmètre de cette analyse

Audit **statique** : on évalue le **code source** (HTML, JSX/TSX, Vue, Twig,
CSS). On ne lance pas de navigateur ni de scan DOM runtime. Les critères qui
exigent un rendu (contraste sur image, audiodescription, sous-titres réellement
synchronisés) sont audités sur la base du code (présence des attributs, des
`<track>`, des valeurs de couleur en CSS) et signalés « à vérifier visuellement »
quand le code seul ne tranche pas — ne jamais conclure « conforme » sur un point
non vérifiable statiquement : statuer **NA** justifié ou laisser une note.

## Workflow d'audit

1. **Analyser le code fourni** (HTML, JSX/TSX, Vue, CSS, templates).
2. **Parcourir les 13 thèmes RGAA** en consultant les skills `rgaa-criteria-*`.
3. **Pour chaque critère applicable, attribuer un statut :**
   - **C** — Conforme : critère respecté.
   - **NC** — Non conforme : critère violé. Identifier l'élément précis
     (`file:line` + extrait de code) et le problème.
   - **NA** — Non applicable : critère sans objet pour ce contenu (justifier en
     une ligne).
4. **Classer chaque non-conformité** par priorité (grille ci-dessous).
5. **Produire le rapport** au format ci-dessous (pour un run d'audit) OU appliquer
   la correction la plus petite qui rend conforme (pour un run d'amélioration).

## Grille de priorités

| Priorité | Définition | Exemples |
|----------|------------|----------|
| 🔴 **Bloquant** | Bloque l'accès au contenu ou à la fonctionnalité pour un utilisateur de technologie d'assistance | Image informative sans `alt`, champ sans label, lien vide, piège au clavier, `<div onClick>` non focusable |
| 🟠 **Majeur** | Impact fort mais contournable par certaines AT | Contraste insuffisant, focus non visible, hiérarchie de titres cassée, lien non explicite, message de statut sans `aria-live` |
| 🟡 **Mineur** | Gêne légère sans blocage | `autocomplete` manquant, `<caption>` de tableau absent, nouvelle fenêtre non signalée, image-texte au lieu de texte stylé |

En contexte d'**amélioration** (boucle de fix), une non-conformité 🔴 ou 🟠
concrète et localisée EST un blocker : il faut appliquer la correction, pas la
différer. Réserver le pushback aux points hors-périmètre UI, factuellement faux,
de pur goût, ou déjà traités.

## Format du rapport de conformité (run d'audit)

```markdown
## Audit RGAA 4.1.2 — Rapport de conformité

**Date :** AAAA-MM-JJ
**Périmètre audité :** [description du code / pages / composants]
**Résultat global :** X% conforme
(N critères applicables — C conformes, NC non conformes, NA non applicables)

---

### Tableau de synthèse

| Thème | C | NC | NA |
|-------|---|----|----|
| 1. Images | | | |
| 2. Cadres | | | |
| 3. Couleurs | | | |
| 4. Multimédia | | | |
| 5. Tableaux | | | |
| 6. Liens | | | |
| 7. Scripts | | | |
| 8. Éléments obligatoires | | | |
| 9. Structuration | | | |
| 10. Présentation | | | |
| 11. Formulaires | | | |
| 12. Navigation | | | |
| 13. Consultation | | | |
| **Total** | | | |

---

### Non-conformités détectées

**[NC] {ID} — {Intitulé du critère}**
- **Élément concerné :** `file:line` + extrait `<code>`
- **Problème :** description précise de la violation
- **Correction :** action concrète à mener (le plus petit changement conforme)
- **Priorité :** 🔴 Bloquant / 🟠 Majeur / 🟡 Mineur

---

### Critères conformes

{ID1}, {ID2}, {ID3}... (liste compacte)

### Critères non applicables

- **{ID}** — {justification courte}
```

## Calcul du taux de conformité

`% conforme = C / (C + NC) × 100`, calculé sur les **critères applicables**
seulement (les NA sont exclus du dénominateur). Arrondir à l'entier.

## Export du rapport (run d'audit)

Après avoir produit le rapport :

1. Créer le dossier `audits/` à la racine du périmètre s'il n'existe pas.
2. Écrire le rapport complet dans `audits/rgaa-AAAA-MM-JJ.md` (date du jour, ISO).
3. Si un fichier de même nom existe déjà, suffixer : `rgaa-AAAA-MM-JJ-2.md`.

Ne jamais gonfler le score ni masquer des non-conformités pour « passer » : un
rapport honnête avec des NC documentées vaut mieux qu'une façade conforme. Si le
périmètre est trop large pour un audit exhaustif, le dire explicitement (bannière
de couverture partielle) plutôt que d'échantillonner en silence.

<!-- TODO(skill-dup): this RGAA skill is also shipped in bots/rgaa-audit/skills/rgaa-audit.md — iterion has no skill-sharing primitive yet, keep the copies in sync. -->

import { useEffect, useState } from "react";

const WORDS: readonly string[] = [
  "Bidouillating",
  "Mijoticating",
  "Gribouillating",
  "Pinaillating",
  "Magouillating",
  "Tortillating",
  "Patouillating",
  "Bricolating",
  "Embrouillating",
  "Triturating",
  "Iterionning",
  "Schmilblicking",
  "Mouliningette",
  "Voilàssembling",
  "Rafistolating",
  "Trifouilling",
  "Fignoling",
  "Gambergifying",
  "Bafouillating",
  "Tambouilling",
  "Bouillonnating",
  "Ronronnating",
  "Manigancing",
  "Chamboulificating",
  "Démerdouilling",
  "Chuchotating",
  "Vasouilling",
  "Tintouinifying",
  "Baguettomancing",
  "Saucissonnaging",
  "Cocoricoding",
  "Bidouillonnant",
  "Zinzinifying",
  "Patapouflicating",
  "Cogitruffing",
  "Ratiocinating",
  "Ponderificating",
  "Reflectomancing",
  "Schemarbling",
  "Iterificating",
  "Branchifying",
  "Routerising",
  "Judgifying",
  "Diagrammifying",
  "Excogitating",
  "Heuristicating",
  "Algorithmancing",
  "Conjecturizing",
  "Recursificating",
  "Postulificating",
  "Stratagemizing",
  "Promptomancing",
  "Tokenificating",
  "Schemificating",
  "Heuristomancing",
  "Hypothesifying",
  "Synthesimorphing",
  "Permutologizing",
];

const ROTATE_MS = 2400;
const TYPE_MS = 35;

export function ThinkingFooter({ active }: { active: boolean }) {
  const [idx, setIdx] = useState(() =>
    Math.floor(Math.random() * WORDS.length),
  );
  const [charCount, setCharCount] = useState(0);

  useEffect(() => {
    if (!active) return;
    const id = window.setInterval(() => {
      setIdx((i) => (i + 1) % WORDS.length);
    }, ROTATE_MS);
    return () => window.clearInterval(id);
  }, [active]);

  useEffect(() => {
    if (!active) return;
    setCharCount(0);
    const word = WORDS[idx] ?? "";
    const id = window.setInterval(() => {
      setCharCount((n) => {
        if (n >= word.length) {
          window.clearInterval(id);
          return n;
        }
        return n + 1;
      });
    }, TYPE_MS);
    return () => window.clearInterval(id);
  }, [idx, active]);

  if (!active) return null;

  const word = WORDS[idx] ?? "";
  const shown = word.slice(0, charCount);
  const done = charCount >= word.length;

  return (
    <div
      aria-hidden="true"
      className="font-mono text-[11px] text-info-fg italic px-1 py-0.5 animate-fade-in-opacity"
    >
      <span className="mr-1 inline-block animate-pulse">✻</span>
      {shown}
      {done ? "…" : ""}
    </div>
  );
}

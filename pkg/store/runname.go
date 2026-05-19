package store

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"

	"github.com/google/uuid"
)

// runNamePool1 is the atmospheric-qualifier slot: synth-tech, rock-grit
// and a sprinkle of mischief ("neon", "thunder", "savage", "feral",
// "sassy", "wonky", "zesty", …) that set the mood. Order is part of
// the contract: changing it would invalidate the seed → name mapping
// for all subsequent runs derived from this list.
var runNamePool1 = []string{
	"neon", "prism", "chrome", "plasma", "quantum", "void", "fractal", "photon", "thunder", "hex",
	"pixel", "glitch", "vapor", "crystal", "neural", "sonic", "electric", "kinetic", "magnetic", "atomic",
	"cosmic", "lunar", "solar", "astral", "stellar", "orbital", "radiant", "savage", "cryo", "pyro",
	"holo", "vector", "sassy", "wonky", "zesty", "helix", "vortex", "beam", "flare", "halo",
	"blaze", "shimmer", "mirage", "renegade", "feral", "phantom", "ghost", "opal", "obsidian", "onyx",
	"quartz", "ember", "ash", "dusk", "dawn", "twilight", "midnight", "aurora", "eclipse", "comet",
	"meteor", "nova", "polar", "arctic", "boreal", "synth", "retro", "riot", "fuzz", "blistered",
	"laser", "sonar", "radar", "wrecked", "scorching", "smoldering", "magneto", "blastoff", "molten", "gamma",
}

// runNamePool2 is the motion-or-state slot: stage-rock action ("shred",
// "thrash", "mosh", "riff", "howl", "growl", "yeet", "bonk", "boop", …)
// blended with iterion's franglais verb roots taken from the studio
// ThinkingFooter ("bidouille", "magouille", "schmilblick", "ratiocine",
// "cocorico", "mijote", "demerdouille", "tambouille", "manigance",
// "cogitruffe", …). Order is part of the contract (see runNamePool1).
var runNamePool2 = []string{
	"drift", "pulse", "ripple", "bloom", "surge", "echo", "wave", "shard", "haze", "flux",
	"smash", "slam", "shred", "crash", "thrash", "wail", "howl", "snarl", "roar", "growl",
	"blast", "kick", "riff", "strum", "yeet", "bonk", "boop", "thump", "mosh", "grind",
	"stomp", "vroom", "sparkle", "sizzle", "churn", "bidouille", "magouille", "gribouille", "trifouille", "patouille",
	"fignole", "bricole", "embrouille", "triture", "rafistole", "tambouille", "bouillonne", "mouline", "ronronne", "manigance",
	"chuchote", "vasouille", "baguette", "saucisson", "cocorico", "zinzin", "cogitruffe", "ratiocine", "schmarble", "itere",
	"mijote", "pinaille", "tortille", "ricochet", "voila", "schmilblick", "demerdouille", "chamboule", "fricote", "gambergue",
	"bafouille", "tergiverse", "gambade", "virevolte", "gigote", "bidouillonne", "eparpille", "ronchonne", "gargouille", "tripote",
}

// runNamePool3 is the coined-noun slot: portmanteaus and invented
// compounds blending synth-cosmos with garage-rock iconography
// ("ampthunder", "riotchord", "overdrive", "feedbackwail",
// "voltageclash", "neondemon", "stellarcrash", …) and iterion's
// signature franglais nouns ("ratiocinator", "mouliningette",
// "baguettomancer", "schmilblickerie", "voilassembler",
// "cogitruffaire", "saucissonator", "cocoricoder", …) that give
// each name a distinctive tail. Order is part of the contract
// (see runNamePool1).
var runNamePool3 = []string{
	"foxhowl", "mothbeam", "owlspark", "hexglade", "pyrebloom", "lumicore", "starforge", "voiddriver", "glitchfox", "pixelriot",
	"neondrift", "prismfox", "chromethrash", "sonarsnoot", "beamspire", "ampthunder", "riotchord", "twilightfox", "feedbackwail", "overdrive",
	"distortpedal", "riotpyre", "voltageclash", "retrosparkle", "neondemon", "jadeshred", "quartzhowl", "solarslam", "stellarcrash", "magnetoglyph",
	"bidouillefox", "magouilloid", "gribouillor", "schmilblicker", "mouliningette", "ratiocinator", "ponderificator", "reflectomancer", "schemarbler", "iterifier",
	"baguettomancer", "saucissonator", "cocoricoder", "zinzinator", "manigancier", "demerdouilleur", "cogitruffaire", "ratiocinette", "mouliningor", "bidouillonator",
	"chamboulifier", "schmilblickerie", "voilassembler", "mijotefox", "trifouilloid", "patouilleur", "bricolatron", "embrouillateur", "triturateur", "rafistolier",
	"tambouillier", "bouillonneur", "moulineur", "ronronneur", "manigancere", "baguettifier", "saucissonet", "cocoricomatic", "zinzinifier", "patapouflin",
	"cogitruffex", "ratiocinaire", "ponderifaire", "recursifaire", "conjecturifier", "branchifier", "judgifier", "diagrammifier", "fricoteur", "gambergueur",
}

// GenerateRunName derives a stable, human-friendly run label from an
// opaque seed. Same seed → same name. The output is kebab-case ASCII,
// path- and URL-safe: <atmos>-<motion>-<coined>-<4hex>, e.g.
// "thunder-bidouille-ratiocinator-a3f2".
//
// Three distinct word slots (atmospheric qualifier, motion/state,
// coined portmanteau) make parallel runs visually unmistakable, and
// the 4-char hex suffix (16 bits) keeps the combined namespace at
// 80×80×80×65 536 ≈ 34 billion — well beyond any realistic collision
// horizon.
func GenerateRunName(seed string) string {
	h := sha256.Sum256([]byte(seed))
	w1 := runNamePool1[binary.BigEndian.Uint16(h[0:2])%uint16(len(runNamePool1))]
	w2 := runNamePool2[binary.BigEndian.Uint16(h[2:4])%uint16(len(runNamePool2))]
	w3 := runNamePool3[binary.BigEndian.Uint16(h[4:6])%uint16(len(runNamePool3))]
	suffix := hex.EncodeToString(h[6:8])
	return w1 + "-" + w2 + "-" + w3 + "-" + suffix
}

// GenerateRunID returns a new UUIDv7 run identifier. Lexicographic
// order matches creation order.
func GenerateRunID() string {
	id, err := uuid.NewV7()
	if err != nil {
		// crypto/rand failure: the process is in an unrecoverable
		// state and silently returning the zero UUID would let
		// concurrent callers collide on the same id.
		panic("iterion: failed to mint run id: " + err.Error())
	}
	return id.String()
}

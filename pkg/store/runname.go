package store

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"

	"github.com/google/uuid"
)

// runNamePool1 is the atmospheric-qualifier slot: synth-tech adjectives
// and noun-prefixes ("neon", "prism", "fractal", …) that set the mood.
// Order is part of the contract: changing it would invalidate the
// seed → name mapping for all subsequent runs derived from this list.
var runNamePool1 = []string{
	"neon", "prism", "chrome", "plasma", "quantum", "void", "fractal", "photon", "lumen", "hex",
	"pixel", "glitch", "vapor", "crystal", "neural", "sonic", "electric", "kinetic", "magnetic", "atomic",
	"cosmic", "lunar", "solar", "astral", "stellar", "orbital", "radiant", "luminous", "cryo", "pyro",
	"holo", "vector", "scalar", "binary", "ionic", "helix", "vortex", "beam", "flare", "halo",
	"glow", "shimmer", "mirage", "spectral", "ethereal", "phantom", "ghost", "opal", "obsidian", "onyx",
	"quartz", "ember", "ash", "dusk", "dawn", "twilight", "midnight", "aurora", "eclipse", "comet",
	"meteor", "nova", "polar", "arctic", "boreal", "synth", "retro", "modal", "fiber", "optic",
	"laser", "sonar", "radar", "gleam", "refractive", "nebular", "magneto", "plasmic", "hypno", "gamma",
}

// runNamePool2 is the motion-or-state slot: short verb-nouns ("drift",
// "pulse", "ripple", …) that give the name kinetic energy. Order is
// part of the contract (see runNamePool1).
var runNamePool2 = []string{
	"drift", "pulse", "ripple", "bloom", "surge", "echo", "wave", "shard", "haze", "flux",
	"zephyr", "glide", "flow", "dash", "sweep", "swirl", "twist", "spin", "whirl", "dance",
	"dive", "plunge", "soar", "climb", "leap", "bound", "rush", "race", "sprint", "hop",
	"bounce", "blink", "wink", "flash", "dart", "ray", "trace", "track", "trail", "chase",
	"hunt", "seek", "fetch", "catch", "clasp", "swipe", "sway", "lull", "hush", "breath",
	"sigh", "hum", "chime", "ring", "peal", "clang", "ping", "tap", "tick", "whir",
	"buzz", "hiss", "sizzle", "spark", "glimmer", "twinkle", "sparkle", "glitter", "shine", "glare",
	"blaze", "burst", "fade", "flick", "gust", "draft", "eddy", "plume", "billow", "churn",
}

// runNamePool3 is the coined-noun slot: portmanteaus and invented
// compound words ("foxhowl", "lumicore", "starforge", …) that give
// each name a distinctive tail. Order is part of the contract (see
// runNamePool1).
var runNamePool3 = []string{
	"foxhowl", "mothbeam", "owlspark", "hexglade", "pyrebloom", "lumicore", "starforge", "cryomantle", "novachime", "auroraflux",
	"voiddriver", "glitchfox", "pixelpurr", "neondrift", "prismfox", "chromewhisper", "photonpetal", "lasercrick", "sonarglow", "beamspire",
	"mosslight", "ferncloak", "dawnglyph", "duskvane", "twilightfox", "midnightfern", "glimmerleaf", "shimmerstone", "mirageknot", "ghostpetal",
	"phantomwick", "spectrelune", "etherspark", "vaporlark", "plasmasong", "quantumcurl", "fractalfin", "vectorvane", "scalarsprout", "helixbloom",
	"vortexlark", "orbitcrest", "novacrest", "cometwhisk", "meteorfern", "polarpetal", "arcticquill", "borealhum", "synthsigh", "retrosparkle",
	"cyberbloom", "optigleam", "modulink", "fiberglyph", "opalrune", "onyxwhisper", "jadehum", "quartzhowl", "emberknot", "ashglyph",
	"crystalbloom", "pyrelark", "cryolark", "holowhisk", "neonmoth", "prismink", "beamwick", "halospire", "voidwhisk", "nebulink",
	"lunaspire", "solarsigh", "stellarhum", "astralcrick", "kineticglide", "magnetoglyph", "ionicpetal", "atomicfern", "ravenrune", "sirenchime",
}

// GenerateRunName derives a stable, human-friendly run label from an
// opaque seed. Same seed → same name. The output is kebab-case ASCII,
// path- and URL-safe: <atmos>-<motion>-<coined>-<4hex>, e.g.
// "neon-glitch-foxhowl-a3f2".
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

import { SunIcon, MoonIcon, DesktopIcon } from "@radix-ui/react-icons";

import { useThemeStore, type ThemeMode } from "@/store/theme";

const OPTIONS: { mode: ThemeMode; label: string; description: string; Icon: typeof SunIcon }[] = [
  {
    mode: "system",
    label: "System",
    description: "Match the OS appearance",
    Icon: DesktopIcon,
  },
  { mode: "light", label: "Light", description: "Always light", Icon: SunIcon },
  { mode: "dark", label: "Dark", description: "Always dark", Icon: MoonIcon },
];

export default function AppearanceTab() {
  const mode = useThemeStore((s) => s.mode);
  const setMode = useThemeStore((s) => s.setMode);
  return (
    <div className="flex flex-col gap-3 p-4 text-sm">
      <div>
        <div className="font-medium text-fg-default mb-1">Theme</div>
        <p className="text-xs text-fg-subtle">
          Persists in this browser. "System" follows your OS preference.
        </p>
      </div>
      <div className="grid grid-cols-3 gap-2">
        {OPTIONS.map(({ mode: m, label, description, Icon }) => {
          const active = mode === m;
          return (
            <button
              key={m}
              type="button"
              onClick={() => setMode(m)}
              className={`flex flex-col items-start gap-1 rounded border p-3 text-left transition-colors ${
                active
                  ? "border-accent bg-accent-soft/30 ring-1 ring-accent/40"
                  : "border-border-default bg-surface-1 hover:bg-surface-2"
              }`}
              aria-pressed={active}
            >
              <span className="flex items-center gap-2 text-fg-default font-medium">
                <Icon className="w-4 h-4" />
                {label}
              </span>
              <span className="text-xs text-fg-subtle">{description}</span>
            </button>
          );
        })}
      </div>
    </div>
  );
}

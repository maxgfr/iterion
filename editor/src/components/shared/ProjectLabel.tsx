import { FolderOpen } from "lucide-react";

import { useProjectInfo } from "@/hooks/useProjectInfo";

interface Props {
  // Visual variant tunes typography for the host surface.
  variant?: "toolbar" | "header";
}

/**
 * ProjectLabel renders the currently-selected folder/project name so
 * the user always knows which workspace they're editing. Sourced via
 * useProjectInfo (desktop currentProject → server work_dir). Renders
 * nothing when no project is resolvable (cloud mode).
 *
 * In desktop mode the label is clickable and dispatches a global
 * `iterion:open-project-switcher` CustomEvent that AuthedApp listens
 * for to open the ProjectSwitcher dialog.
 */
export default function ProjectLabel({ variant = "toolbar" }: Props) {
  const { name, dir, source } = useProjectInfo();
  if (!name) return null;

  const clickable = source === "desktop";
  const tooltip = dir
    ? `Project: ${name}\n${dir}${clickable ? "\n\nClick to switch project" : ""}`
    : name;

  const size = variant === "header" ? "text-xs" : "text-xs";
  const iconSize = 13;

  const base =
    "inline-flex items-center gap-1.5 px-2.5 py-1 rounded-md font-medium " +
    size +
    " border border-accent/40 bg-accent-soft text-accent leading-none max-w-[220px]";
  const interactive = clickable
    ? " hover:border-accent hover:bg-accent/20 cursor-pointer transition-colors"
    : " cursor-default";

  const content = (
    <>
      <FolderOpen size={iconSize} className="shrink-0" />
      <span className="truncate">{name}</span>
    </>
  );

  if (clickable) {
    return (
      <button
        type="button"
        className={base + interactive}
        title={tooltip}
        onClick={() =>
          window.dispatchEvent(new CustomEvent("iterion:open-project-switcher"))
        }
      >
        {content}
      </button>
    );
  }
  return (
    <span className={base + interactive} title={tooltip}>
      {content}
    </span>
  );
}

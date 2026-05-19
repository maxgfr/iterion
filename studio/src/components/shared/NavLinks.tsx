import { Link, useLocation } from "wouter";
import {
  HomeIcon,
  Pencil2Icon,
  ListBulletIcon,
  ViewGridIcon,
  RocketIcon,
  PaperPlaneIcon,
} from "@radix-ui/react-icons";

import { useServerInfoStore } from "@/store/serverInfo";

export type Section =
  | "home"
  | "whatsNext"
  | "editor"
  | "runs"
  | "board"
  | "dispatcher";

interface Props {
  collapsed: boolean;
}

interface LinkDef {
  section: Section;
  href: string;
  label: string;
  icon: typeof HomeIcon;
}

const BASE_LINKS: LinkDef[] = [
  { section: "home", href: "/", label: "Home", icon: HomeIcon },
  { section: "whatsNext", href: "/whats-next", label: "What's Next", icon: PaperPlaneIcon },
  { section: "editor", href: "/editor", label: "Editor", icon: Pencil2Icon },
  { section: "runs", href: "/runs", label: "Runs", icon: ListBulletIcon },
];

// deriveSection maps the current path to the highlighted nav entry by
// looking at the first path segment only — so `/runs/:id` highlights
// "runs" without `/boardroom` accidentally highlighting "board".
const SEGMENT_TO_SECTION: Record<string, Section> = {
  "whats-next": "whatsNext",
  editor: "editor",
  runs: "runs",
  board: "board",
  dispatcher: "dispatcher",
};

function deriveSection(pathname: string): Section | undefined {
  if (pathname === "/" || pathname === "") return "home";
  const segment = pathname.split("/")[1] ?? "";
  return SEGMENT_TO_SECTION[segment];
}

// NavLinks renders the primary navigation as a vertical column inside
// the Sidebar. When `collapsed` is true the labels are hidden and each
// link becomes a square icon button with a native tooltip.
export default function NavLinks({ collapsed }: Props) {
  const info = useServerInfoStore((s) => s.info);
  const [location] = useLocation();
  const active = deriveSection(location);

  const links: LinkDef[] = [...BASE_LINKS];
  if (info?.native_tracker_enabled) {
    links.push({ section: "board", href: "/board", label: "Board", icon: ViewGridIcon });
  }
  if (info?.dispatcher_enabled) {
    links.push({ section: "dispatcher", href: "/dispatcher", label: "Dispatcher", icon: RocketIcon });
  }

  return (
    <nav className="flex flex-col gap-0.5" aria-label="Primary navigation">
      {links.map(({ section, href, label, icon: Icon }) => {
        const isActive = active === section;
        const base = "inline-flex items-center text-xs rounded border transition-colors";
        const layout = collapsed
          ? "justify-center h-8 w-10"
          : "gap-2 px-2 py-1.5";
        const state = isActive
          ? "border-accent/40 bg-accent-soft text-fg-default"
          : "border-transparent text-fg-muted hover:bg-surface-2 hover:text-fg-default";
        return (
          <Link
            key={section}
            href={href}
            className={`${base} ${layout} ${state}`}
            aria-current={isActive ? "page" : undefined}
            title={label}
            aria-label={label}
          >
            <Icon className="w-3.5 h-3.5 shrink-0" />
            {!collapsed && <span className="truncate">{label}</span>}
          </Link>
        );
      })}
    </nav>
  );
}

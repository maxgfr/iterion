import { Link, useLocation } from "wouter";
import {
  HomeIcon,
  Pencil2Icon,
  ListBulletIcon,
  ViewGridIcon,
  RocketIcon,
} from "@radix-ui/react-icons";

import { useServerInfoStore } from "@/store/serverInfo";

type Section = "home" | "editor" | "runs" | "board" | "conductor";

interface Props {
  // Override which link is rendered as active. When omitted, the
  // current route decides — useful for views that own a sub-route
  // (e.g. /runs/:id should keep "Runs" highlighted).
  active?: Section;
}

interface LinkDef {
  section: Section;
  href: string;
  label: string;
  icon: typeof HomeIcon;
}

const BASE_LINKS: LinkDef[] = [
  { section: "home", href: "/", label: "Home", icon: HomeIcon },
  { section: "editor", href: "/editor", label: "Editor", icon: Pencil2Icon },
  { section: "runs", href: "/runs", label: "Runs", icon: ListBulletIcon },
];

function sectionFromPath(path: string): Section {
  if (path === "/") return "home";
  if (path.startsWith("/runs") || path.startsWith("/launch")) return "runs";
  if (path.startsWith("/editor")) return "editor";
  if (path.startsWith("/board")) return "board";
  if (path.startsWith("/conductor")) return "conductor";
  return "home";
}

export default function NavLinks({ active }: Props) {
  const [location] = useLocation();
  const current = active ?? sectionFromPath(location);
  const info = useServerInfoStore((s) => s.info);

  const links: LinkDef[] = [...BASE_LINKS];
  if (info?.native_tracker_enabled) {
    links.push({ section: "board", href: "/board", label: "Board", icon: ViewGridIcon });
  }
  if (info?.conductor_enabled) {
    links.push({ section: "conductor", href: "/conductor", label: "Conductor", icon: RocketIcon });
  }

  return (
    <nav className="flex items-center gap-0.5" aria-label="Primary navigation">
      {links.map(({ section, href, label, icon: Icon }) => {
        const isActive = current === section;
        return (
          <Link
            key={section}
            href={href}
            className={`inline-flex items-center gap-1.5 text-xs px-2 py-1 rounded border ${
              isActive
                ? "border-accent/40 bg-accent-soft text-fg-default"
                : "border-transparent text-fg-muted hover:bg-surface-2 hover:text-fg-default"
            }`}
            aria-current={isActive ? "page" : undefined}
          >
            <Icon className="w-3.5 h-3.5" />
            <span>{label}</span>
          </Link>
        );
      })}
    </nav>
  );
}

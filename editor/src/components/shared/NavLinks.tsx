import { Link, useLocation } from "wouter";
import {
  HomeIcon,
  Pencil2Icon,
  ListBulletIcon,
} from "@radix-ui/react-icons";

type Section = "home" | "editor" | "runs";

interface Props {
  // Override which link is rendered as active. When omitted, the
  // current route decides — useful for views that own a sub-route
  // (e.g. /runs/:id should keep "Runs" highlighted).
  active?: Section;
}

const LINKS: Array<{
  section: Section;
  href: string;
  label: string;
  icon: typeof HomeIcon;
}> = [
  { section: "home", href: "/", label: "Home", icon: HomeIcon },
  { section: "editor", href: "/editor", label: "Editor", icon: Pencil2Icon },
  { section: "runs", href: "/runs", label: "Runs", icon: ListBulletIcon },
];

function sectionFromPath(path: string): Section {
  if (path === "/") return "home";
  if (path.startsWith("/runs") || path.startsWith("/launch")) return "runs";
  if (path.startsWith("/editor")) return "editor";
  return "home";
}

export default function NavLinks({ active }: Props) {
  const [location] = useLocation();
  const current = active ?? sectionFromPath(location);

  return (
    <nav className="flex items-center gap-0.5" aria-label="Primary navigation">
      {LINKS.map(({ section, href, label, icon: Icon }) => {
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

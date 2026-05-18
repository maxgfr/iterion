import { Link } from "wouter";
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
  { section: "whatsNext", href: "/whats-next", label: "What's Next", icon: PaperPlaneIcon },
  { section: "editor", href: "/editor", label: "Editor", icon: Pencil2Icon },
  { section: "runs", href: "/runs", label: "Runs", icon: ListBulletIcon },
];

export default function NavLinks({ active }: Props) {
  const info = useServerInfoStore((s) => s.info);

  const links: LinkDef[] = [...BASE_LINKS];
  if (info?.native_tracker_enabled) {
    links.push({ section: "board", href: "/board", label: "Board", icon: ViewGridIcon });
  }
  if (info?.dispatcher_enabled) {
    links.push({ section: "dispatcher", href: "/dispatcher", label: "Dispatcher", icon: RocketIcon });
  }

  return (
    <nav className="flex items-center gap-0.5" aria-label="Primary navigation">
      {links.map(({ section, href, label, icon: Icon }) => {
        const isActive = active === section;
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
            title={label}
            aria-label={label}
          >
            <Icon className="w-3.5 h-3.5" />
            {/* Hide the label text on narrow viewports so the header
             * stays within ~360px without wrapping. The link title +
             * aria-label still announce the destination to screen
             * readers and tooltip on hover. */}
            <span className="hidden sm:inline">{label}</span>
          </Link>
        );
      })}
    </nav>
  );
}

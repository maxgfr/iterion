import type { ReactNode } from "react";
import { ChevronRightIcon } from "@radix-ui/react-icons";

import { IconButton } from "@/components/ui";

export function CollapseButton({
  onCollapse,
}: {
  onCollapse?: () => void;
}): ReactNode {
  if (!onCollapse) return null;
  return (
    <IconButton
      label="Hide details panel"
      size="sm"
      variant="ghost"
      className="absolute top-1.5 right-1.5 z-[var(--z-canvas)]"
      onClick={onCollapse}
    >
      <ChevronRightIcon />
    </IconButton>
  );
}

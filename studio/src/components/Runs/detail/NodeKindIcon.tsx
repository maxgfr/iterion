import type { ReactNode } from "react";

import { NodeIcon } from "@/components/icons/NodeIcon";
import type { NodeKind } from "@/api/types";

export function NodeKindIcon({ kind }: { kind?: string }): ReactNode {
  if (!kind) return null;
  return <NodeIcon kind={kind as NodeKind} size={14} />;
}

import { useEffect } from "react";
import type { ReactNode } from "react";

import { useUIStore } from "@/store/ui";

interface HeaderSlotInput {
  left?: ReactNode;
  right?: ReactNode;
}

// useHeaderSlot pushes contextual content into the persistent
// ContextualHeaderBar that sits above <main> in the AppShell. The bar
// only renders when at least one slot is non-null, so pages without
// header content simply skip this hook and let the bar stay hidden.
//
// Slots are cleared on unmount and replaced (not merged) on re-render
// so pages don't leak stale buttons after navigation.
export function useHeaderSlot({ left = null, right = null }: HeaderSlotInput): void {
  const setHeaderSlots = useUIStore((s) => s.setHeaderSlots);

  useEffect(() => {
    setHeaderSlots({ left, right });
    return () => {
      setHeaderSlots({ left: null, right: null });
    };
  }, [left, right, setHeaderSlots]);
}

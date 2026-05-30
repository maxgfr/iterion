import { useEffect } from "react";

import { useProjectInfo } from "@/hooks/useProjectInfo";
import { useTabsStore } from "@/store/tabs";

/**
 * useProjectScopeSync keeps the tabs store's currentProjectKey in sync
 * with the active project (useProjectInfo().dir). Run/editor tabs are
 * filtered to that key (see selectRunTabs / selectEditorTabs), so
 * switching projects hides the other project's tabs (preserved for
 * switch-back) instead of leaking them into the current project.
 *
 * Mount once at the app root. The dir is null in cloud mode (no folder),
 * which disables scoping — every tab shows.
 */
export function useProjectScopeSync(): void {
  const dir = useProjectInfo().dir;
  useEffect(() => {
    useTabsStore.getState().setCurrentProjectKey(dir);
  }, [dir]);
}

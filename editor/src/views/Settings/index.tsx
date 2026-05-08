import { Dialog, Tabs } from "@/components/ui";

import ApiKeysTab from "./ApiKeysTab";
import ProjectsTab from "./ProjectsTab";
import UpdatesTab from "./UpdatesTab";
import AboutTab from "./AboutTab";

interface Props {
  open: boolean;
  onClose: () => void;
  tab: string;
  onTabChange: (tab: string) => void;
}

const tabItems = [
  { value: "api-keys", label: "API keys" },
  { value: "projects", label: "Projects" },
  { value: "updates", label: "Updates" },
  { value: "about", label: "About" },
];

export default function Settings({ open, onClose, tab, onTabChange }: Props) {
  return (
    <Dialog
      open={open}
      onOpenChange={(o) => !o && onClose()}
      title="Settings"
      widthClass="max-w-3xl"
    >
      <Tabs
        value={tab}
        onValueChange={onTabChange}
        items={tabItems}
        panels={{
          "api-keys": <ApiKeysTab />,
          projects: <ProjectsTab />,
          updates: <UpdatesTab />,
          about: <AboutTab />,
        }}
        className="min-h-[420px]"
      />
    </Dialog>
  );
}

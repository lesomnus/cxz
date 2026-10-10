import { createContext } from "react";
import type { Project } from "../../gen/cxz/project_pb";
import { Connection } from "../shared/api/connection";

export const WorkspaceContext = createContext<
  | {
      c: Connection;
      projects: Project[];
      projectsLoading: boolean;
      settingsFileOpen: boolean;
      setSettingsFileOpen: (open: boolean) => void;
      expandProject: (id: string) => void;
    }
  | undefined
>(undefined);

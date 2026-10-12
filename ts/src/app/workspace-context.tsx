import { createContext } from "react";
import type { Project } from "#gen/cxz/project_pb";
import type { Session } from "#gen/cxz/session_pb";
import { Connection } from "#src/shared/api/connection.ts";

export const WorkspaceContext = createContext<
  | {
      c: Connection;
      projects: Project[];
      sessions: Session[];
      projectsLoading: boolean;
      settingsFileOpen: boolean;
      setSettingsFileOpen: (open: boolean) => void;
      expandProject: (id: string) => void;
    }
  | undefined
>(undefined);

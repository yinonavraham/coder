import { Theme } from "@mui/material/styles"
import dayjs from "dayjs"
import duration from "dayjs/plugin/duration"
import minMax from "dayjs/plugin/minMax"
import utc from "dayjs/plugin/utc"
import semver from "semver"
import * as TypesGen from "../api/typesGenerated"
import i18next from "i18next"
import CircularProgress from "@mui/material/CircularProgress"
import ErrorIcon from "@mui/icons-material/ErrorOutline"
import StopIcon from "@mui/icons-material/StopOutlined"
import PlayIcon from "@mui/icons-material/PlayArrowOutlined"
import QueuedIcon from "@mui/icons-material/HourglassEmpty"

dayjs.extend(duration)
dayjs.extend(utc)
dayjs.extend(minMax)

const DisplayWorkspaceBuildStatusLanguage = {
  succeeded: "Succeeded",
  pending: "Pending",
  running: "Running",
  canceling: "Canceling",
  canceled: "Canceled",
  failed: "Failed",
}

const DisplayAgentVersionLanguage = {
  unknown: "Unknown",
}

export const getDisplayWorkspaceBuildStatus = (
  theme: Theme,
  build: TypesGen.WorkspaceBuild,
) => {
  switch (build.job.status) {
    case "succeeded":
      return {
        type: "success",
        color: theme.palette.success.main,
        status: DisplayWorkspaceBuildStatusLanguage.succeeded,
      } as const
    case "pending":
      return {
        type: "secondary",
        color: theme.palette.text.secondary,
        status: DisplayWorkspaceBuildStatusLanguage.pending,
      } as const
    case "running":
      return {
        type: "info",
        color: theme.palette.primary.main,
        status: DisplayWorkspaceBuildStatusLanguage.running,
      } as const
    case "failed":
      return {
        type: "error",
        color: theme.palette.text.secondary,
        status: DisplayWorkspaceBuildStatusLanguage.failed,
      } as const
    case "canceling":
      return {
        type: "warning",
        color: theme.palette.warning.light,
        status: DisplayWorkspaceBuildStatusLanguage.canceling,
      } as const
    case "canceled":
      return {
        type: "secondary",
        color: theme.palette.text.secondary,
        status: DisplayWorkspaceBuildStatusLanguage.canceled,
      } as const
  }
}

export const getDisplayWorkspaceBuildInitiatedBy = (
  build: TypesGen.WorkspaceBuild,
): string => {
  switch (build.reason) {
    case "initiator":
      return build.initiator_name
    case "autostart":
    case "autostop":
      return "Coder"
  }
}

const getWorkspaceBuildDurationInSeconds = (
  build: TypesGen.WorkspaceBuild,
): number | undefined => {
  const isCompleted = build.job.started_at && build.job.completed_at

  if (!isCompleted) {
    return
  }

  const startedAt = dayjs(build.job.started_at)
  const completedAt = dayjs(build.job.completed_at)
  return completedAt.diff(startedAt, "seconds")
}

export const displayWorkspaceBuildDuration = (
  build: TypesGen.WorkspaceBuild,
  inProgressLabel = "In progress",
): string => {
  const duration = getWorkspaceBuildDurationInSeconds(build)
  return duration ? `${duration} seconds` : inProgressLabel
}

export const getDisplayVersionStatus = (
  agentVersion: string,
  serverVersion: string,
): { displayVersion: string; outdated: boolean } => {
  if (!semver.valid(serverVersion) || !semver.valid(agentVersion)) {
    return {
      displayVersion: agentVersion || DisplayAgentVersionLanguage.unknown,
      outdated: false,
    }
  } else if (semver.lt(agentVersion, serverVersion)) {
    return {
      displayVersion: agentVersion,
      outdated: true,
    }
  } else {
    return {
      displayVersion: agentVersion,
      outdated: false,
    }
  }
}

export const isWorkspaceOn = (workspace: TypesGen.Workspace): boolean => {
  const transition = workspace.latest_build.transition
  const status = workspace.latest_build.job.status
  return transition === "start" && status === "succeeded"
}

export const defaultWorkspaceExtension = (
  __startDate?: dayjs.Dayjs,
): TypesGen.PutExtendWorkspaceRequest => {
  const now = __startDate ? dayjs(__startDate) : dayjs()
  const fourHoursFromNow = now.add(4, "hours").utc()

  return {
    deadline: fourHoursFromNow.format(),
  }
}

// You can see the favicon designs here: https://www.figma.com/file/YIGBkXUcnRGz2ZKNmLaJQf/Coder-v2-Design?node-id=560%3A620

type FaviconType =
  | "favicon"
  | "favicon-success"
  | "favicon-error"
  | "favicon-warning"
  | "favicon-running"

export const getFaviconByStatus = (
  build: TypesGen.WorkspaceBuild,
): FaviconType => {
  switch (build.status) {
    case undefined:
      return "favicon"
    case "running":
      return "favicon-success"
    case "starting":
      return "favicon-running"
    case "stopping":
      return "favicon-running"
    case "stopped":
      return "favicon"
    case "deleting":
      return "favicon"
    case "deleted":
      return "favicon"
    case "canceling":
      return "favicon-warning"
    case "canceled":
      return "favicon"
    case "failed":
      return "favicon-error"
    case "pending":
      return "favicon"
  }
}

export const getDisplayWorkspaceTemplateName = (
  workspace: TypesGen.Workspace,
): string => {
  return workspace.template_display_name.length > 0
    ? workspace.template_display_name
    : workspace.template_name
}

export const getDisplayWorkspaceStatus = (
  workspaceStatus: TypesGen.WorkspaceStatus,
  provisionerJob?: TypesGen.ProvisionerJob,
) => {
  const { t } = i18next

  switch (workspaceStatus) {
    case undefined:
      return {
        text: t("workspaceStatus.loading", { ns: "common" }),
        icon: <LoadingIcon />,
      } as const
    case "running":
      return {
        type: "success",
        text: t("workspaceStatus.running", { ns: "common" }),
        icon: <PlayIcon />,
      } as const
    case "starting":
      return {
        type: "success",
        text: t("workspaceStatus.starting", { ns: "common" }),
        icon: <LoadingIcon />,
      } as const
    case "stopping":
      return {
        type: "warning",
        text: t("workspaceStatus.stopping", { ns: "common" }),
        icon: <LoadingIcon />,
      } as const
    case "stopped":
      return {
        type: "warning",
        text: t("workspaceStatus.stopped", { ns: "common" }),
        icon: <StopIcon />,
      } as const
    case "deleting":
      return {
        type: "warning",
        text: t("workspaceStatus.deleting", { ns: "common" }),
        icon: <LoadingIcon />,
      } as const
    case "deleted":
      return {
        type: "error",
        text: t("workspaceStatus.deleted", { ns: "common" }),
        icon: <ErrorIcon />,
      } as const
    case "canceling":
      return {
        type: "warning",
        text: t("workspaceStatus.canceling", { ns: "common" }),
        icon: <LoadingIcon />,
      } as const
    case "canceled":
      return {
        type: "warning",
        text: t("workspaceStatus.canceled", { ns: "common" }),
        icon: <ErrorIcon />,
      } as const
    case "failed":
      return {
        type: "error",
        text: t("workspaceStatus.failed", { ns: "common" }),
        icon: <ErrorIcon />,
      } as const
    case "pending":
      return {
        type: "info",
        text: getPendingWorkspaceStatusText(provisionerJob),
        icon: <QueuedIcon />,
      } as const
  }
}

const getPendingWorkspaceStatusText = (
  provisionerJob?: TypesGen.ProvisionerJob,
): string => {
  const { t } = i18next

  if (!provisionerJob || provisionerJob.queue_size === 0) {
    return t("workspaceStatus.pending", { ns: "common" })
  }
  return "Position in queue: " + provisionerJob.queue_position
}

const LoadingIcon = () => {
  return <CircularProgress size={10} style={{ color: "#FFF" }} />
}

export const hasJobError = (workspace: TypesGen.Workspace) => {
  return workspace.latest_build.job.error !== undefined
}

export const paramUsedToCreateWorkspace = (
  param: TypesGen.TemplateVersionParameter,
) => !param.ephemeral

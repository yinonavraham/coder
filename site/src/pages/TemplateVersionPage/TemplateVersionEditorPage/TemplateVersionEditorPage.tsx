import { useMachine } from "@xstate/react"
import { TemplateVersionEditor } from "components/TemplateVersionEditor/TemplateVersionEditor"
import { useOrganizationId } from "hooks/useOrganizationId"
import { usePermissions } from "hooks/usePermissions"
import { FC } from "react"
import { Helmet } from "react-helmet-async"
import { useNavigate, useParams } from "react-router-dom"
import { pageTitle } from "utils/page"
import { templateVersionEditorMachine } from "xServices/templateVersionEditor/templateVersionEditorXService"
import { useTemplateVersionData } from "./data"

type Params = {
  version: string
  template: string
}

export const TemplateVersionEditorPage: FC = () => {
  const { version: versionName, template: templateName } = useParams() as Params
  const navigate = useNavigate()
  const orgId = useOrganizationId()
  const [editorState, sendEvent] = useMachine(templateVersionEditorMachine, {
    context: { orgId },
    actions: {
      onPublish: () => {
        navigate(`/templates/${templateName}`)
      },
    },
  })
  const permissions = usePermissions()
  const { isSuccess, data } = useTemplateVersionData(
    {
      orgId,
      templateName,
      versionName,
    },
    {
      onSuccess(data) {
        sendEvent({ type: "INITIALIZE", tarReader: data.tarReader })
      },
    },
  )

  return (
    <>
      <Helmet>
        <title>{pageTitle(`${templateName} · Template Editor`)}</title>
      </Helmet>

      {isSuccess && (
        <TemplateVersionEditor
          template={data.template}
          deploymentBannerVisible={permissions.viewDeploymentStats}
          templateVersion={editorState.context.version || data.version}
          defaultFileTree={data.fileTree}
          onPreview={(fileTree) => {
            sendEvent({
              type: "CREATE_VERSION",
              fileTree,
              templateId: data.template.id,
            })
          }}
          onCancelPublish={() => {
            sendEvent({
              type: "CANCEL_PUBLISH",
            })
          }}
          onPublish={() => {
            sendEvent({
              type: "PUBLISH",
            })
          }}
          onConfirmPublish={(data) => {
            sendEvent({
              type: "CONFIRM_PUBLISH",
              ...data,
            })
          }}
          isAskingPublishParameters={editorState.matches(
            "askPublishParameters",
          )}
          publishingError={editorState.context.publishingError}
          isPublishing={editorState.matches("publishingVersion")}
          disablePreview={editorState.hasTag("loading")}
          disableUpdate={
            editorState.hasTag("loading") ||
            editorState.context.version?.job.status !== "succeeded"
          }
          resources={editorState.context.resources}
          buildLogs={editorState.context.buildLogs}
          isPromptingMissingVariables={editorState.matches("promptVariables")}
          missingVariables={editorState.context.missingVariables}
          onSubmitMissingVariableValues={(values) => {
            sendEvent({
              type: "SET_MISSING_VARIABLE_VALUES",
              values,
            })
          }}
          onCancelSubmitMissingVariableValues={() => {
            sendEvent({
              type: "CANCEL_MISSING_VARIABLE_VALUES",
            })
          }}
        />
      )}
    </>
  )
}

export default TemplateVersionEditorPage

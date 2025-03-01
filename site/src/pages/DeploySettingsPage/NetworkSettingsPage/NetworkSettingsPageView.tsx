import { DeploymentOption } from "api/types"
import {
  Badges,
  EnabledBadge,
  DisabledBadge,
} from "components/DeploySettingsLayout/Badges"
import { Header } from "components/DeploySettingsLayout/Header"
import OptionsTable from "components/DeploySettingsLayout/OptionsTable"
import { Stack } from "components/Stack/Stack"
import {
  deploymentGroupHasParent,
  useDeploymentOptions,
} from "utils/deployOptions"
import { docs } from "utils/docs"

export type NetworkSettingsPageViewProps = {
  options: DeploymentOption[]
}

export const NetworkSettingsPageView = ({
  options: options,
}: NetworkSettingsPageViewProps): JSX.Element => (
  <Stack direction="column" spacing={6}>
    <div>
      <Header
        title="Network"
        description="Configure your deployment connectivity."
        docsHref={docs("/networking")}
      />
      <OptionsTable
        options={options.filter((o) =>
          deploymentGroupHasParent(o.group, "Networking"),
        )}
      />
    </div>

    <div>
      <Header
        title="Port Forwarding"
        secondary
        description="Port forwarding lets developers securely access processes on their Coder workspace from a local machine."
        docsHref={docs("/networking/port-forwarding")}
      />

      <Badges>
        {useDeploymentOptions(options, "Wildcard Access URL")[0].value !==
        "" ? (
          <EnabledBadge />
        ) : (
          <DisabledBadge />
        )}
      </Badges>
    </div>
  </Stack>
)

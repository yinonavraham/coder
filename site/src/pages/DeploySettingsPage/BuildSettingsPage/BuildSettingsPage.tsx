import { useDeploySettings } from "components/DeploySettingsLayout/DeploySettingsLayout"
import { FC } from "react"
import { Helmet } from "react-helmet-async"
import { pageTitle } from "util/page"
import { BuildSettingsPageView } from "./BuildSettingsPageView"

const GeneralSettingsPage: FC = () => {
  const { deploymentConfig, deploymentDAUs, getDeploymentDAUsError } =
    useDeploySettings()

  return (
    <>
      <Helmet>
        <title>{pageTitle("Builds")}</title>
      </Helmet>
      <BuildSettingsPageView
        deploymentConfig={deploymentConfig}
        deploymentDAUs={deploymentDAUs}
        getDeploymentDAUsError={getDeploymentDAUsError}
      />
    </>
  )
}

export default GeneralSettingsPage

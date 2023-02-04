import { DeploymentConfig, DeploymentDAUsResponse } from "api/typesGenerated"
import { AlertBanner } from "components/AlertBanner/AlertBanner"
import { DAUChart } from "components/DAUChart/DAUChart"
import { Header } from "components/DeploySettingsLayout/Header"
import OptionsTable from "components/DeploySettingsLayout/OptionsTable"
import { Stack } from "components/Stack/Stack"
import Grid from "@material-ui/core/Grid"
import { colors } from "theme/colors"
import Box from "@material-ui/core/Box"
import { makeStyles, Theme } from "@material-ui/core/styles"

export type BuildSettingsPageViewProps = {
  deploymentConfig: Pick<
    DeploymentConfig,
    "provisioner" | "wildcard_access_url"
  >
  deploymentDAUs?: DeploymentDAUsResponse
  getDeploymentDAUsError: unknown
}

const BuildStatus = (props: {
  color: string
  builds: number
  name: string
}) => {
  return (
    <Box bgcolor={props.color} style={{ textAlign: "center", padding: 12 }}>
      <p style={{ fontSize: 32 }}>{props.builds}</p>
      <p style={{ fontSize: 18 }}>{props.name}</p>
    </Box>
  )
}

export const BuildSettingsPageView = ({
  deploymentConfig,
  deploymentDAUs,
  getDeploymentDAUsError,
}: BuildSettingsPageViewProps): JSX.Element => {
  const classes = useStyles()

  return (
    <>
      <Header
        title="Workspace builds"
        description="Information about your build queue and configuration."
        docsHref="hthttps://coder.com/docs/v2/latest/admin/scale"
      />

      <div className={classes.buildGrid}>
        <Grid container spacing={3}>
          <Grid item xs={3}>
            <BuildStatus builds={324} color="success.main" name="Running" />
          </Grid>
          <Grid item xs={3}>
            <BuildStatus builds={12} color={colors.orange[9]} name="Building" />
          </Grid>
          <Grid item xs={3}>
            <BuildStatus builds={3} color="primary.main" name="Pending" />
          </Grid>
          <Grid item xs={3}>
            <BuildStatus builds={9} color={colors.red[8]} name="Failed" />
          </Grid>
        </Grid>
      </div>

      <Stack spacing={4}>
        {Boolean(getDeploymentDAUsError) && (
          <AlertBanner error={getDeploymentDAUsError} severity="error" />
        )}
        {deploymentDAUs && <DAUChart daus={deploymentDAUs} />}
        <OptionsTable
          options={{
            provisoner_daemons: deploymentConfig.provisioner.daemons,
            replicas: {
              flag: "asd",
              name: "Replicas",
              usage:
                "Improves reliability and runs its a unique set of provisioner daemons.",
              value: "4",
            },
          }}
        />
      </Stack>
    </>
  )
}

const useStyles = makeStyles<Theme>((theme) => ({
  buildGrid: () => ({
    marginBottom: theme.spacing(3),
  }),
}))

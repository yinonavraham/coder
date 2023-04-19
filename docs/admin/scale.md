We scale-test Coder with the [same utility](#scaletest-utility) that can be used in your environment for insights into how Coder scales with your infrastructure.

## General concepts

### Definitions

- **coderd**: Coder’s primary service. Learn more about [Coder’s architecture](../about/architecture.md)
- **coderd replicas**: Replicas (often via Kubernetes) for high availability, this is an [enterprise feature](../enterprise.md)
- **workspace build**: Provisioning operation on a workspace (e.g. create/stop/delete/apply)
- **concurrent workspace builds**: Simultaneous workspace builds  across all users
- **concurrent connections**: Any connection to a workspace (e.g. SSH, web terminal, `coder_app`)
- **provisioner daemons**: Coder subcomponent that performs workspace builds. Coder runs one workspace build
  at a time on each provisioner daemon. One coderd replica can host many daemons
- **scaletest**: Our scale-testing utility, built into the `coder` command line.

### Workspace builds

Coder runs workspace builds in a queue. The number of concurrent builds will be limited to the number of provisioner
daemons across all coderd replicas.

```text
2 coderd replicas * 30 provisioner daemons = 60 max concurrent workspace builds
```

Workspace builds are CPU-intensive.  Increasing the number of provisioner daemons will increase the maximum CPU load
attributable to builds.  We recommend you tune this number so that even at maximum concurrency, the coderd replicas
are below 100% CPU utilization, so that builds, which are latency-insensitive, do not starve latency-sensitive tasks
like serving the API or workspace connections.

Database load is directly affected by the total number of concurrent builds across the cluster.

### Workspace connections

When users connect to workspaces, coderd always negotiates the connection when it starts.

After the connection is established, coderd may or may not be on the data path between the user and their workspace. If
connecting via a web browser, coderd is alway on the data path. If the user is not connecting via browser _and_ the
user and workspace are able to reach each other directly, they send data peer-to-peer and not via coderd.

Thus, the following factors affect the load on coderd

1. The rate at which end users connect to workspaces (e.g. connections per second)
2. The rate at which data is sent and received (e.g. bytes per second)
3. The type of connections (e.g. browser-based applications vs `coder ssh` on the CLI)
4. Network devices (NATs, firewalls) that allow or block direct connections from users to workspaces

In general, you can achieve lower resource requirements on Coder by using Desktop IDEs that connect to workspaces over
SSH compared with Browser-based IDEs.

Database load is affected by the rate at which users connect (e.g. connections per second), but not the data rates or
other factors listed above.

### API Requests

When end users use the Coder dashboard, the dashboard application makes API requests to coderd, which in turn consume
resources on coderd, and on the database.  This scales proportionally with the number of active users.

In addition, any autonomous systems that make Coder API calls generate load.  You should increase resources above the
recommendations given below if you plan to have any autonomous systems interact with Coder, and monitor resource
utilization to tune your requirements.

## Infrastructure recommendations

The following table lists our recommended sizing for the Coderd replicas and PostgreSQL database at various numbers
of daily active users. These recommendations are based on model assumptions which we detail below, and are simply a
starting point for planning.  We *always* recommend that you monitor your coderds and database and adjust resources.
Coder use in your organization may grow and usage patterns may change over time.

### How to use this chart

Once you have determined the number of active users you need to support, consider availability requirements for the
number of coderd replicas.  The CPU and memory requirements listed are the _total_ resources across all your replicas
at peak load for the given number of active users.

When planning for high availability, consider the effect of losing replicas during peak use.  For example, if you plan
to have 3 replicas to be able to survive the loss of one, you may choose to divide the listed CPU cores by _2_ for the
size of each  replica, since you will have 2 of 3 nodes active in that scenario.

Generally speaking, coder will perform better with fewer, larger replicas compared with a large number of small
replicas, due to communication overhead between the replicas and the database.

CPU & memory requirements for the database assumes a single active primary.

**NOTE**: These recommendations cover _only_ the Coder control plane and database.  You must also consider the resources
for your users workspaces.  Resources for workspaces vary considerably by organization, but are also straightforward to
calculate.  For example, if you have 100 active users each with an 8 core, 16 GB workspace, assuming no
over-provisioning, you will need 800 cores and 1600 GB across the cluster for workspaces.

### Recommendations

| Active users | Concurrent builds | Concurrent connections | Coderd CPU | Coderd memory | Database CPU | Database memory |
|--------------|-------------------|------------------------|------------|---------------|--------------|-----------------|
| 50           | ??                | ??                     | ?? cores   | ?? GB         | ?? cores     | ?? GB           |
| 100          | ??                | ??                     | ?? cores   | ?? GB         | ?? cores     | ?? GB           |
| 500          | ??                | ??                     | ?? cores   | ?? GB         | ?? cores     | ?? GB           |
| 1000         | ??                | ??                     | ?? cores   | ?? GB         | ?? cores     | ?? GB           |
| 3000         | ??                | ??                     | ?? cores   | ?? GB         | ?? cores     | ?? GB           |

### Our model

Coder utilization is multidimensional and typically varies from hour to hour and week to week.  In this guide we have
chosen to give resource recommendations based on a single parameter: number of daily active users.  This prioritizes
simple recommendations to get administrators and planners started, but obscures much of the nuance.

This section explains the assumptions we make in our recommendations. We don't expect every administrator to read and
understand these assumptions, as you can always start with our recommendations, monitor and adjust, but we detail them
here for reference.

#### Concurrent builds

Workspace builds are typically correlated in their start times: the developers in a typical organization start their
work in the morning and leave in the evening.  Use of auto-start further increases the time correlation around nice
round numbers: 8:00am, 9:00am, etc.  We assume that ?? of daily active users will start their workspaces simultaneously
due to these factors.

#### Concurrent connections

?? concurrent connection starts
?? Bytes/sec per connection

#### Dashboard utilization

?? fraction of users have the dashboard open

#### Workspace infrastructure

We provision workspaces on Google Kubernetes Engine, using our
[Kubernetes template](https://github.com/coder/coder/tree/main/examples/templates/kubernetes).  Other Terraform
provisioners may have different CPU & memory utilization during builds.

## Scale testing utility

Since Coder's performance is highly dependent on the templates and workflows you support, we recommend using our scale testing utility against your own environments.

The following command will run our scale test against your own Coder deployment. You can also specify a template name and any parameter values.

```sh
coder scaletest create-workspaces \
    --count 1000 \
    --template "kubernetes" \
    --concurrency 0 \
    --cleanup-concurrency 0 \
    --parameter "home_disk_size=10" \
    --run-command "sleep 2 && echo hello"

# Run `coder scaletest create-workspaces --help` for all usage
```

> To avoid potential outages and orphaned resources, we recommend running scale tests on a secondary "staging" environment.

The test does the following:

1. create `1000` workspaces
1. establish SSH connection to each workspace
1. run `sleep 3 && echo hello` on each workspace via the web terminal
1. close connections, attempt to delete all workspaces
1. return results (e.g. `998 succeeded, 2 failed to connect`)

Concurrency is configurable. `concurrency 0` means the scaletest test will attempt to create & connect to all workspaces immediately.

## Troubleshooting

If a load test fails or if you are experiencing performance issues during day-to-day use, you can leverage Coder's [prometheus metrics](./prometheus.md) to identify bottlenecks during scale tests. Additionally, you can use your existing cloud monitoring stack to measure load, view server logs, etc.

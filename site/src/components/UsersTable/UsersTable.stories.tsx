import { ComponentMeta, Story } from "@storybook/react"
import {
  MockUser,
  MockUser2,
  MockAssignableSiteRoles,
} from "testHelpers/entities"
import { UsersTable, UsersTableProps } from "./UsersTable"

export default {
  title: "components/UsersTable",
  component: UsersTable,
  args: {
    isNonInitialPage: false,
  },
} as ComponentMeta<typeof UsersTable>

const Template: Story<UsersTableProps> = (args) => <UsersTable {...args} />

export const Example = Template.bind({})
Example.args = {
  users: [MockUser, MockUser2],
  roles: MockAssignableSiteRoles,
  canEditUsers: false,
}

export const Editable = Template.bind({})
Editable.args = {
  users: [MockUser, MockUser2],
  roles: MockAssignableSiteRoles,
  canEditUsers: true,
  canViewActivity: true,
}

export const Empty = Template.bind({})
Empty.args = {
  users: [],
  roles: MockAssignableSiteRoles,
}

export const Loading = Template.bind({})
Loading.args = {
  users: [],
  roles: MockAssignableSiteRoles,
  isLoading: true,
}
Loading.parameters = {
  chromatic: { pauseAnimationAtEnd: true },
}

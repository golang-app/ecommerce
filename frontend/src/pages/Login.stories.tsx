import { ComponentStory, ComponentMeta } from "@storybook/react";
import { LoginPage, LoginProps } from "./Login";

export default {
  title: "Page/Login",
  component: LoginPage,
  parameters: {
    // More on Story layout: https://storybook.js.org/docs/react/configure/story-layout
    layout: "fullscreen",
  },
} as ComponentMeta<typeof LoginPage>;

const Template: ComponentStory<typeof LoginPage> = (args: LoginProps) => (
  <LoginPage {...args} />
);

export const Default = Template.bind({});
Default.args = {};

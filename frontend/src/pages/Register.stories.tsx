import { ComponentStory, ComponentMeta } from "@storybook/react";
import { RegisterPage, RegisterPageProps } from "./Register";

export default {
  title: "Page/Register",
  component: RegisterPage,
  parameters: {
    // More on Story layout: https://storybook.js.org/docs/react/configure/story-layout
    layout: "fullscreen",
  },
} as ComponentMeta<typeof RegisterPage>;

const Template: ComponentStory<typeof RegisterPage> = (args: RegisterPageProps) => (
  <RegisterPage {...args} />
);

export const Default = Template.bind({});
Default.args = {};

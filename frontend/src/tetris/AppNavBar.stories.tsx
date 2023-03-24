import { ComponentStory, ComponentMeta } from "@storybook/react";
import { AppNavBar } from "./AppNavBar";

export default {
  title: "tetris/AppNavBar",
  component: AppNavBar,
  parameters: {
    // More on Story layout: https://storybook.js.org/docs/react/configure/story-layout
    layout: "fullscreen",
  },
} as ComponentMeta<typeof AppNavBar>;

const Template: ComponentStory<typeof AppNavBar> = (args: any) => (
  <AppNavBar {...args} />
);

export const Default = Template.bind({});

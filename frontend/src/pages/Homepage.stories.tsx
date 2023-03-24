import { ComponentStory, ComponentMeta } from "@storybook/react";
import { Homepage, HomepageProps } from "./Homepage";

export default {
  title: "Page/Homepage",
  component: Homepage,
  parameters: {
    // More on Story layout: https://storybook.js.org/docs/react/configure/story-layout
    layout: "fullscreen",
  },
} as ComponentMeta<typeof Homepage>;

const Template: ComponentStory<typeof Homepage> = (args: HomepageProps) => (
  <Homepage {...args} />
);

export const Default = Template.bind({});
Default.args = {
  products: [
    {
      id: "1",
      name: "Product Name",
      price: {
        amount: 100,
        currency: "USD",
      },
      shortDescription: "Product Description",
    },
  ],
};

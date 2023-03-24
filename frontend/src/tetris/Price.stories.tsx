import { ComponentStory, ComponentMeta } from "@storybook/react";
import { Price, PriceView } from "./Price";

export default {
  title: "tetris/PriceView",
  component: PriceView,
  parameters: {
    // More on Story layout: https://storybook.js.org/docs/react/configure/story-layout
    layout: "fullscreen",
  },
} as ComponentMeta<typeof PriceView>;

interface PriceViewProps {
  price: Price;
}

const Template: ComponentStory<any> = (args: PriceViewProps) => (
  <PriceView price={args.price} />
);

export const Default = Template.bind({});
Default.args = {
  price: {
    amount: 100,
    currency: "USD",
  },
};

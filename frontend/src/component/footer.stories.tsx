import { ComponentStory, ComponentMeta } from "@storybook/react";

import { Footer } from "./footer";
import "bootstrap/dist/css/bootstrap.min.css";

export default {
  title: "Layout/Footer",
  component: Footer,
  parameters: {
    layout: "fullscreen",
  },
} as ComponentMeta<typeof Footer>;

const Template: ComponentStory<typeof Footer> = () => <Footer />;

export const DefaultFooter = Template.bind({});
DefaultFooter.args = {};

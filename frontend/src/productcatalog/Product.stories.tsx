import { ComponentStory, ComponentMeta } from "@storybook/react";
import { ProductView, ProductViewProps } from "./product";
import { Container, Grid } from "@mui/material";

export default {
  title: "product catalog/ProductView",
  component: ProductView,
  parameters: {
    // More on Story layout: https://storybook.js.org/docs/react/configure/story-layout
    layout: "fullscreen",
  },
} as ComponentMeta<typeof ProductView>;

const Template: ComponentStory<any> = (args: ProductViewProps) => (
  <Container>
    <Grid container>
      <Grid xs={4}>
        <ProductView product={args.product} />
      </Grid>
      <Grid xs={4}>
        <ProductView product={args.product} />
      </Grid>
      <Grid xs={4}>
        <ProductView product={args.product} />
      </Grid>
    </Grid>
  </Container>
);

export const Default = Template.bind({});
Default.args = {
  product: {
    name: "Product Name",
    price: {
      amount: 100,
      currency: "USD",
    },
    shortDescription: "Product Description",
  },
};

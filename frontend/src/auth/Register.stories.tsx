import { ComponentStory, ComponentMeta } from "@storybook/react";
import { RegisterForm, RegisterFormProps } from "./Register";
import { Container, Grid } from "@mui/material";

export default {
  title: "Auth/SignUp",
  component: RegisterForm,
  parameters: {
    // More on Story layout: https://storybook.js.org/docs/react/configure/story-layout
    layout: "fullscreen",
  },
} as ComponentMeta<typeof RegisterForm>;

const Template: ComponentStory<any> = (args: RegisterFormProps) => (
  <Container>
    <Grid container>
        <RegisterForm  {...args}/>
    </Grid>
  </Container>
);

export const Default = Template.bind({});
Default.args = {};

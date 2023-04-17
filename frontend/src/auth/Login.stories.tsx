import { ComponentStory, ComponentMeta } from "@storybook/react";
import { LoginForm, LoginFormProps } from "./Login";
import { Container, Grid } from "@mui/material";

export default {
  title: "Auth/Login",
  component: LoginForm,
  parameters: {
    // More on Story layout: https://storybook.js.org/docs/react/configure/story-layout
    layout: "fullscreen",
  },
} as ComponentMeta<typeof LoginForm>;

const Template: ComponentStory<any> = (args: LoginFormProps) => (
  <Container>
    <Grid container>
        <LoginForm  {...args}/>
    </Grid>
  </Container>
);

export const Default = Template.bind({});
Default.args = {};
